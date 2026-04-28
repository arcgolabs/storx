package badgerx

import (
	"context"
	"encoding/binary"
	"errors"
	"sort"

	storx "github.com/arcgolabs/storx"
	"github.com/dgraph-io/badger/v4"
)

const migrationNamespacePrefix = "__storx_migrations"

// Migration describes one Badger schema migration step.
type Migration struct {
	Version uint64
	Up      func(ctx context.Context, txn *badger.Txn) error
}

// MigrationPlan describes the pending migrations without applying them.
type MigrationPlan struct {
	CurrentVersion uint64
	TargetVersion  uint64
	Pending        []uint64
}

// Migrator tracks and applies ordered Badger schema migrations.
type Migrator struct {
	db   *badger.DB
	name string
}

func NewMigrator(db *badger.DB, name string) *Migrator {
	return &Migrator{db: db, name: name}
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
	err := m.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(normalizePrefix(migrationNamespacePrefix, "/") + m.name))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			return err
		}
		return item.Value(func(data []byte) error {
			if len(data) != 8 {
				return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
			}
			version = binary.BigEndian.Uint64(data)
			return nil
		})
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
	ordered, err := normalizeMigrations(migrations)
	if err != nil {
		return err
	}

	metaKey := []byte(normalizePrefix(migrationNamespacePrefix, "/") + m.name)
	return m.db.Update(func(txn *badger.Txn) error {
		var current uint64
		item, err := txn.Get(metaKey)
		switch {
		case err == nil:
			if err := item.Value(func(data []byte) error {
				if len(data) != 8 {
					return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
				}
				current = binary.BigEndian.Uint64(data)
				return nil
			}); err != nil {
				return err
			}
		case !errors.Is(err, badger.ErrKeyNotFound):
			return err
		}

		for _, migration := range ordered {
			if migration.Version <= current {
				continue
			}
			if err := migration.Up(ctx, txn); err != nil {
				return err
			}
			current = migration.Version
			var version [8]byte
			binary.BigEndian.PutUint64(version[:], current)
			if err := txn.Set(metaKey, version[:]); err != nil {
				return err
			}
		}
		return nil
	})
}

// Plan returns the pending migration versions without applying them.
func (m *Migrator) Plan(ctx context.Context, migrations ...Migration) (MigrationPlan, error) {
	if m == nil || m.db == nil {
		return MigrationPlan{}, errors.Join(storx.ErrInvalidValue, storx.ErrClosed)
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return MigrationPlan{}, err
	}
	ordered, err := normalizeMigrations(migrations)
	if err != nil {
		return MigrationPlan{}, err
	}

	current, err := m.CurrentVersion(ctx)
	if err != nil {
		return MigrationPlan{}, err
	}
	plan := MigrationPlan{
		CurrentVersion: current,
		Pending:        make([]uint64, 0, len(ordered)),
	}
	for _, migration := range ordered {
		if migration.Version <= current {
			continue
		}
		plan.Pending = append(plan.Pending, migration.Version)
		plan.TargetVersion = migration.Version
	}
	if len(plan.Pending) == 0 {
		plan.TargetVersion = current
	}
	return plan, nil
}

// DryRun is an alias of Plan for migration callers.
func (m *Migrator) DryRun(ctx context.Context, migrations ...Migration) (MigrationPlan, error) {
	return m.Plan(ctx, migrations...)
}

func normalizeMigrations(migrations []Migration) ([]Migration, error) {
	ordered := append([]Migration(nil), migrations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Version < ordered[j].Version
	})
	var previous uint64
	for index, migration := range ordered {
		if migration.Version == 0 || migration.Up == nil {
			return nil, errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
		}
		if index > 0 && migration.Version == previous {
			return nil, errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
		}
		previous = migration.Version
	}
	return ordered, nil
}
