package badgerx

import (
	"context"

	"github.com/dgraph-io/badger/v4"
)

type badgerSchemaBootstrapper interface {
	bootstrapIndex(ctx context.Context, txn *badger.Txn) error
}

// Bootstrap validates the schema and prepares declared indexes.
func (s ModelSchema[K, V]) Bootstrap(ctx context.Context, db *badger.DB) error {
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.Update(func(txn *badger.Txn) error {
		for _, definition := range s.Indexes {
			if definition == nil {
				continue
			}
			index := definition.openWithRaw(db, s.Keys)
			bootstrapper, ok := index.(badgerSchemaBootstrapper)
			if !ok {
				continue
			}
			if err := bootstrapper.bootstrapIndex(ctx, txn); err != nil {
				return err
			}
		}
		return nil
	})
}

// BootstrapWithDB validates the schema and prepares declared indexes.
func (s ModelSchema[K, V]) BootstrapWithDB(ctx context.Context, db *DB) error {
	if db == nil {
		return nil
	}
	return s.Bootstrap(ctx, db.Raw())
}
