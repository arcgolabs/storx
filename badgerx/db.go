package badgerx

import (
	"context"
	"errors"
	"log/slog"
	"time"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/arcgolabs/storx/observer"
	"github.com/dgraph-io/badger/v4"
	"github.com/samber/oops"
)

// DB is a thin wrapper around *badger.DB that carries logger and observer
// defaults for namespace wrappers.
type DB struct {
	raw       *badger.DB
	logger    *slog.Logger
	observers []observer.Observer
}

// Wrap attaches badgerx defaults to an existing Badger DB.
func Wrap(db *badger.DB, opts ...DBOption) *DB {
	options := dbOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	return &DB{
		raw:       db,
		logger:    normalizeLogger(options.logger),
		observers: observer.Clone(options.observers),
	}
}

// Open opens a Badger DB and wraps it.
func Open(options badger.Options, opts ...DBOption) (*DB, error) {
	db, err := badger.Open(options)
	if err != nil {
		return nil, oops.In("storx/badgerx").
			With("op", "open", "dir", options.Dir).
			Wrapf(errors.Join(storx.ErrClosed, err), "open badger database")
	}
	return Wrap(db, opts...), nil
}

// Raw returns the underlying Badger DB.
func (db *DB) Raw() *badger.DB {
	if db == nil {
		return nil
	}
	return db.raw
}

// Close closes the underlying Badger DB.
func (db *DB) Close() error {
	if db == nil || db.raw == nil {
		return nil
	}
	return db.raw.Close()
}

// NewNamespaceWithDB creates a typed namespace wrapper that inherits DB-level
// logger and observers.
func NewNamespaceWithDB[K any, V any](
	db *DB,
	prefix string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...NamespaceOption,
) *Namespace[K, V] {
	base := defaultNamespaceOptions()
	base.logger = db.loggerValue()
	base.observers = observer.Clone(db.observers)
	return newNamespaceWithOptions(db.Raw(), prefix, keys, values, base, opts...)
}

// RunValueLogGC runs DB-level GC with wrapper logging and observers.
func (db *DB) RunValueLogGC(ctx context.Context, discardRatio float64) error {
	startedAt := time.Now()
	err := RunValueLogGC(ctx, db.Raw(), discardRatio)
	if err != nil {
		db.loggerValue().Debug("storx badger gc failed", "discard_ratio", discardRatio, "err", err)
	} else {
		db.loggerValue().Debug("storx badger gc completed", "discard_ratio", discardRatio)
	}

	observer.ObserveAll(normalizeContext(ctx), observer.Clone(db.observers), observer.Event{
		Engine:     "badger",
		Target:     "",
		TargetType: "database",
		Operation:  "run_value_log_gc",
		StartedAt:  startedAt,
		Duration:   time.Since(startedAt),
		Err:        err,
	})
	return err
}

func (db *DB) loggerValue() *slog.Logger {
	if db == nil {
		return slog.Default()
	}
	return normalizeLogger(db.logger)
}
