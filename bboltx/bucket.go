package bboltx

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"time"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/internal/bytesx"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/arcgolabs/storx/observer"
	"github.com/samber/oops"
	"go.etcd.io/bbolt"
)

type Bucket[K any, V any] struct {
	db        *bbolt.DB
	name      string
	nameBytes []byte
	keys      keycodec.Codec[K]
	values    codec.Codec[V]
	opts      bucketOptions
}

// NewBucket creates a typed bbolt bucket wrapper.
func NewBucket[K any, V any](
	db *bbolt.DB,
	name string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...BucketOption,
) *Bucket[K, V] {
	return newBucketWithOptions(db, name, keys, values, defaultBucketOptions(), opts...)
}

func newBucketWithOptions[K any, V any](
	db *bbolt.DB,
	name string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	base bucketOptions,
	opts ...BucketOption,
) *Bucket[K, V] {
	options := base
	options.observers = observer.Clone(options.observers)
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	options.logger = normalizeLogger(options.logger)

	return &Bucket[K, V]{
		db:        db,
		name:      name,
		nameBytes: []byte(name),
		keys:      keys,
		values:    values,
		opts:      options,
	}
}

// Raw returns the underlying bbolt DB.
func (b *Bucket[K, V]) Raw() *bbolt.DB {
	if b == nil {
		return nil
	}
	return b.db
}

// Name returns the underlying bucket name.
func (b *Bucket[K, V]) Name() string {
	if b == nil {
		return ""
	}
	return b.name
}

// View runs a read-only transaction against the bucket.
func (b *Bucket[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error {
	ctx, start := b.startOperation(ctx)

	if err := b.validate("view"); err != nil {
		b.finishOperation(ctx, start, "view", err)
		return err
	}
	if fn == nil {
		err := b.wrapError("view", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
		b.finishOperation(ctx, start, "view", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "view", err)
		return err
	}

	err := b.db.View(func(tx *bbolt.Tx) error {
		return fn(b.newViewTx(ctx, tx.Bucket(b.nameBytes)))
	})
	err = b.normalizeEngineError("view", err)
	b.finishOperation(ctx, start, "view", err)
	return err
}

// Update runs a writable transaction against the bucket.
func (b *Bucket[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error {
	ctx, start := b.startOperation(ctx)

	if err := b.validate("update"); err != nil {
		b.finishOperation(ctx, start, "update", err)
		return err
	}
	if fn == nil {
		err := b.wrapError("update", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
		b.finishOperation(ctx, start, "update", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "update", err)
		return err
	}

	err := b.db.Update(func(tx *bbolt.Tx) error {
		update, err := b.newUpdateTx(ctx, tx)
		if err != nil {
			return err
		}
		return fn(update)
	})
	err = b.normalizeEngineError("update", err)
	b.finishOperation(ctx, start, "update", err)
	return err
}

// Get reads a value from the bucket.
func (b *Bucket[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	ctx, start := b.startOperation(ctx)

	var (
		value V
		ok    bool
	)

	if err := b.validate("get"); err != nil {
		b.finishOperation(ctx, start, "get", err)
		return value, false, err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "get", err)
		return value, false, err
	}

	err := b.db.View(func(tx *bbolt.Tx) error {
		view := b.newViewTx(ctx, tx.Bucket(b.nameBytes))
		var err error
		value, ok, err = view.Get(key)
		return err
	})
	err = b.normalizeEngineError("get", err)
	b.finishOperation(ctx, start, "get", err)
	return value, ok, err
}

// Put writes a value to the bucket.
func (b *Bucket[K, V]) Put(ctx context.Context, key K, value V) error {
	ctx, start := b.startOperation(ctx)

	if err := b.validate("put"); err != nil {
		b.finishOperation(ctx, start, "put", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "put", err)
		return err
	}

	err := b.db.Update(func(tx *bbolt.Tx) error {
		update, err := b.newUpdateTx(ctx, tx)
		if err != nil {
			return err
		}
		return update.Put(key, value)
	})
	err = b.normalizeEngineError("put", err)
	b.finishOperation(ctx, start, "put", err)
	return err
}

// Delete removes a value from the bucket.
func (b *Bucket[K, V]) Delete(ctx context.Context, key K) error {
	ctx, start := b.startOperation(ctx)

	if err := b.validate("delete"); err != nil {
		b.finishOperation(ctx, start, "delete", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "delete", err)
		return err
	}

	err := b.db.Update(func(tx *bbolt.Tx) error {
		update, err := b.newUpdateTx(ctx, tx)
		if err != nil {
			return err
		}
		return update.Delete(key)
	})
	err = b.normalizeEngineError("delete", err)
	b.finishOperation(ctx, start, "delete", err)
	return err
}

// Exists reports whether a key exists in the bucket.
func (b *Bucket[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	ctx, start := b.startOperation(ctx)

	if err := b.validate("exists"); err != nil {
		b.finishOperation(ctx, start, "exists", err)
		return false, err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "exists", err)
		return false, err
	}

	var ok bool
	err := b.db.View(func(tx *bbolt.Tx) error {
		view := b.newViewTx(ctx, tx.Bucket(b.nameBytes))
		var err error
		ok, err = view.Exists(key)
		return err
	})
	err = b.normalizeEngineError("exists", err)
	b.finishOperation(ctx, start, "exists", err)
	return ok, err
}

// NextSequence returns the next bucket sequence value.
func (b *Bucket[K, V]) NextSequence(ctx context.Context) (uint64, error) {
	ctx, start := b.startOperation(ctx)

	if err := b.validate("next_sequence"); err != nil {
		b.finishOperation(ctx, start, "next_sequence", err)
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "next_sequence", err)
		return 0, err
	}

	var seq uint64
	err := b.db.Update(func(tx *bbolt.Tx) error {
		update, err := b.newUpdateTx(ctx, tx)
		if err != nil {
			return err
		}
		seq, err = update.NextSequence()
		return err
	})
	err = b.normalizeEngineError("next_sequence", err)
	b.finishOperation(ctx, start, "next_sequence", err)
	return seq, err
}

func (b *Bucket[K, V]) startOperation(ctx context.Context) (context.Context, time.Time) {
	return normalizeContext(ctx), time.Now()
}

func (b *Bucket[K, V]) finishOperation(ctx context.Context, startedAt time.Time, op string, err error) {
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		b.logger().Debug("storx bbolt operation failed", "bucket", b.safeName(), "op", op, "err", err)
	}

	observer.ObserveAll(ctx, b.observers(), observer.Event{
		Engine:     "bbolt",
		Target:     b.safeName(),
		TargetType: "bucket",
		Operation:  op,
		StartedAt:  startedAt,
		Duration:   time.Since(startedAt),
		Err:        err,
	})
}

func (b *Bucket[K, V]) newViewTx(ctx context.Context, bucket *bbolt.Bucket) *viewTx[K, V] {
	return &viewTx[K, V]{
		bucket: bucket,
		owner:  b,
		ctx:    normalizeContext(ctx),
	}
}

func (b *Bucket[K, V]) newUpdateTx(ctx context.Context, tx *bbolt.Tx) (*updateTx[K, V], error) {
	bucket := tx.Bucket(b.nameBytes)
	if bucket == nil {
		if !b.opts.createBucketIfMissing {
			return nil, b.wrapError("update", storx.ErrBucketNotFound, "bucket does not exist")
		}

		var err error
		bucket, err = tx.CreateBucketIfNotExists(b.nameBytes)
		if err != nil {
			return nil, b.wrapError("update", errors.Join(storx.ErrBucketNotFound, err), "create bucket")
		}
	}

	return &updateTx[K, V]{
		viewTx: viewTx[K, V]{
			bucket: bucket,
			owner:  b,
			ctx:    normalizeContext(ctx),
		},
	}, nil
}

func (b *Bucket[K, V]) validate(op string) error {
	switch {
	case b == nil:
		return oops.In("storx/bboltx").
			With("op", op).
			Wrapf(errors.Join(storx.ErrInvalidValue, storx.ErrClosed), "bucket wrapper is nil")
	case b.db == nil:
		return b.wrapError(op, errors.Join(storx.ErrInvalidValue, storx.ErrClosed), "database is nil")
	case b.keys == nil:
		return b.wrapError(op, errors.Join(storx.ErrKeyCodec, storx.ErrInvalidKey), "key codec is nil")
	case b.values == nil:
		return b.wrapError(op, errors.Join(storx.ErrCodec, storx.ErrInvalidValue), "value codec is nil")
	default:
		return nil
	}
}

func (b *Bucket[K, V]) encodeKey(op string, key K) ([]byte, error) {
	data, err := b.keys.EncodeKey(key)
	if err != nil {
		return nil, b.wrapError(op, errors.Join(storx.ErrKeyCodec, err), "encode key", "key_type", typeOf[K]())
	}
	return bytesx.Clone(data), nil
}

func (b *Bucket[K, V]) decodeKey(op string, data []byte) (K, error) {
	key, err := b.keys.DecodeKey(bytesx.Clone(data))
	if err != nil {
		return key, b.wrapError(op, errors.Join(storx.ErrKeyCodec, err), "decode key", "key_type", typeOf[K](), "size", len(data))
	}
	return key, nil
}

func (b *Bucket[K, V]) encodeValue(op string, value V) ([]byte, error) {
	data, err := b.values.Marshal(value)
	if err != nil {
		return nil, b.wrapError(op, errors.Join(storx.ErrCodec, err), "marshal value", "value_type", typeOf[V]())
	}
	return bytesx.Clone(data), nil
}

func (b *Bucket[K, V]) decodeValue(op string, data []byte) (V, error) {
	value, err := b.values.Unmarshal(bytesx.Clone(data))
	if err != nil {
		return value, b.wrapError(op, errors.Join(storx.ErrCodec, err), "unmarshal value", "value_type", typeOf[V](), "size", len(data))
	}
	return value, nil
}

func (b *Bucket[K, V]) normalizeEngineError(op string, err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, bbolt.ErrDatabaseNotOpen), errors.Is(err, bbolt.ErrTxClosed):
		return b.wrapError(op, errors.Join(storx.ErrClosed, err), "bbolt operation failed")
	default:
		return err
	}
}

func (b *Bucket[K, V]) wrapError(op string, err error, format string, attrs ...any) error {
	builder := oops.In("storx/bboltx").With("op", op, "bucket", b.safeName())
	if len(attrs) > 0 {
		builder = builder.With(attrs...)
	}
	return builder.Wrapf(err, "%s", format)
}

func (b *Bucket[K, V]) logger() *slog.Logger {
	if b == nil {
		return slog.Default()
	}
	return normalizeLogger(b.opts.logger)
}

func (b *Bucket[K, V]) observers() []observer.Observer {
	if b == nil {
		return nil
	}
	return b.opts.observers
}

func (b *Bucket[K, V]) safeName() string {
	if b == nil {
		return ""
	}
	return b.name
}

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func typeOf[T any]() string {
	return reflect.TypeOf((*T)(nil)).Elem().String()
}
