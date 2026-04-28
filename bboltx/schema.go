package bboltx

import (
	"context"

	"go.etcd.io/bbolt"
)

type bboltSchemaBootstrapper interface {
	bootstrapIndex(ctx context.Context, tx *bbolt.Tx) error
}

// Bootstrap ensures the primary bucket and declared index buckets exist.
func (s ModelSchema[K, V]) Bootstrap(ctx context.Context, db *bbolt.DB) error {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(s.Name)); err != nil {
			return err
		}
		for _, definition := range s.Indexes {
			if definition == nil {
				continue
			}
			index := definition.openWithRaw(db, s.Keys)
			bootstrapper, ok := index.(bboltSchemaBootstrapper)
			if !ok {
				continue
			}
			if err := bootstrapper.bootstrapIndex(ctx, tx); err != nil {
				return err
			}
		}
		return nil
	})
}

// BootstrapWithDB ensures the primary bucket and declared index buckets exist.
func (s ModelSchema[K, V]) BootstrapWithDB(ctx context.Context, db *DB) error {
	if db == nil {
		return nil
	}
	return s.Bootstrap(ctx, db.Raw())
}
