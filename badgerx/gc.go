package badgerx

import (
	"context"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/dgraph-io/badger/v4"
	"github.com/samber/oops"
)

// RunValueLogGC runs Badger value log GC until there is no more immediate work.
func RunValueLogGC(ctx context.Context, db *badger.DB, discardRatio float64) error {
	ctx = normalizeContext(ctx)
	if db == nil {
		return oops.In("storx/badgerx").
			With("op", "run_value_log_gc").
			Wrapf(errors.Join(storx.ErrInvalidValue, storx.ErrClosed), "database is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for {
		err := db.RunValueLogGC(discardRatio)
		switch {
		case err == nil:
		case errors.Is(err, badger.ErrNoRewrite):
			return nil
		case errors.Is(err, badger.ErrDBClosed):
			return oops.In("storx/badgerx").
				With("op", "run_value_log_gc").
				Wrapf(errors.Join(storx.ErrClosed, err), "run badger value log gc")
		default:
			return oops.In("storx/badgerx").
				With("op", "run_value_log_gc", "discard_ratio", discardRatio).
				Wrapf(err, "run badger value log gc")
		}

		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// RunValueLogGC runs DB-level GC using the namespace's underlying database.
func (n *Namespace[K, V]) RunValueLogGC(ctx context.Context, discardRatio float64) error {
	if n == nil {
		return oops.In("storx/badgerx").
			With("op", "run_value_log_gc").
			Wrapf(errors.Join(storx.ErrInvalidValue, storx.ErrClosed), "namespace wrapper is nil")
	}
	if err := n.validate("run_value_log_gc"); err != nil {
		return err
	}
	return Wrap(n.db, WithDBLogger(n.logger()), WithDBObservers(n.observers()...)).
		RunValueLogGC(ctx, discardRatio)
}
