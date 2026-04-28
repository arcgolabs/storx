package bboltx

import (
	"context"
	"encoding/binary"
	"errors"
	"sort"

	storx "github.com/arcgolabs/storx"
	"go.etcd.io/bbolt"
)

var defaultMigrationBucket = []byte("__storx_migrations")

// Migration describes one bbolt schema migration step.
type Migration struct {
	Version uint64
	Up      func(ctx context.Context, tx *bbolt.Tx) error
}

// Migrator tracks and applies ordered bbolt schema migrations.
type Migrator struct {
	db         *bbolt.DB
	name       string
	metaBucket []byte
}

func NewMigrator(db *bbolt.DB, name string) *Migrator {
	return &Migrator{
		db:         db,
		name:       name,
		metaBucket: append([]byte(nil), defaultMigrationBucket...),
	}
}

func (m *Migrator) CurrentVersion(ctx context.Context) (uint64, error) {
	if m == nil || m.db == nil {
		return 0, errors.Join(storx.ErrInvalidValue, storx.ErrClosed)
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	var version uint64
	err := m.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(m.metaBucket)
		if bucket == nil {
			return nil
		}
		data := bucket.Get([]byte(m.name))
		if len(data) == 0 {
			return nil
		}
		if len(data) != 8 {
			return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
		}
		version = binary.BigEndian.Uint64(data)
		return nil
	})
	return version, err
}

func (m *Migrator) Migrate(ctx context.Context, migrations ...Migration) error {
	if m == nil || m.db == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrClosed)
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateMigrations(migrations); err != nil {
		return err
	}

	return m.db.Update(func(tx *bbolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(m.metaBucket)
		if err != nil {
			return err
		}

		var current uint64
		if data := bucket.Get([]byte(m.name)); len(data) > 0 {
			if len(data) != 8 {
				return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
			}
			current = binary.BigEndian.Uint64(data)
		}

		for _, migration := range migrations {
			if migration.Version <= current {
				continue
			}
			if err := migration.Up(ctx, tx); err != nil {
				return err
			}
			current = migration.Version
			var version [8]byte
			binary.BigEndian.PutUint64(version[:], current)
			if err := bucket.Put([]byte(m.name), version[:]); err != nil {
				return err
			}
		}
		return nil
	})
}

func validateMigrations(migrations []Migration) error {
	ordered := append([]Migration(nil), migrations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Version < ordered[j].Version
	})
	var previous uint64
	for index, migration := range ordered {
		if migration.Version == 0 || migration.Up == nil {
			return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
		}
		if index > 0 && migration.Version == previous {
			return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
		}
		previous = migration.Version
	}
	copy(migrations, ordered)
	return nil
}
