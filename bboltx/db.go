package bboltx

import (
	"errors"
	"log/slog"
	"os"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/arcgolabs/storx/observer"
	"github.com/samber/oops"
	"go.etcd.io/bbolt"
)

// DB is a thin wrapper around *bbolt.DB that carries logger and observer
// defaults for bucket wrappers.
type DB struct {
	raw       *bbolt.DB
	logger    *slog.Logger
	observers []observer.Observer
}

// Wrap attaches bboltx defaults to an existing bbolt DB.
func Wrap(db *bbolt.DB, opts ...DBOption) *DB {
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

// Open opens a bbolt database and wraps it.
func Open(path string, mode os.FileMode, options *bbolt.Options, opts ...DBOption) (*DB, error) {
	db, err := bbolt.Open(path, mode, options)
	if err != nil {
		return nil, oops.In("storx/bboltx").
			With("op", "open", "path", path).
			Wrapf(errors.Join(storx.ErrClosed, err), "open bbolt database")
	}
	return Wrap(db, opts...), nil
}

// Raw returns the underlying bbolt DB.
func (db *DB) Raw() *bbolt.DB {
	if db == nil {
		return nil
	}
	return db.raw
}

// Close closes the underlying bbolt DB.
func (db *DB) Close() error {
	if db == nil || db.raw == nil {
		return nil
	}
	return db.raw.Close()
}

// NewBucketWithDB creates a typed bucket wrapper that inherits DB-level logger
// and observers.
func NewBucketWithDB[K any, V any](
	db *DB,
	name string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...BucketOption,
) *Bucket[K, V] {
	base := defaultBucketOptions()
	base.logger = db.loggerValue()
	base.observers = observer.Clone(db.observers)
	return newBucketWithOptions(db.Raw(), name, keys, values, base, opts...)
}

func (db *DB) loggerValue() *slog.Logger {
	if db == nil {
		return slog.Default()
	}
	return normalizeLogger(db.logger)
}
