package badgerx

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"time"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/internal/bytesx"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/arcgolabs/storx/observer"
	"github.com/dgraph-io/badger/v4"
	"github.com/samber/oops"
)

type Namespace[K any, V any] struct {
	db            *badger.DB
	prefix        []byte
	displayPrefix string
	keys          keycodec.Codec[K]
	values        codec.Codec[V]
	opts          namespaceOptions
}

// NewNamespace creates a typed namespace wrapper backed by a Badger key prefix.
func NewNamespace[K any, V any](
	db *badger.DB,
	prefix string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...NamespaceOption,
) *Namespace[K, V] {
	return newNamespaceWithOptions(db, prefix, keys, values, defaultNamespaceOptions(), opts...)
}

func newNamespaceWithOptions[K any, V any](
	db *badger.DB,
	prefix string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	base namespaceOptions,
	opts ...NamespaceOption,
) *Namespace[K, V] {
	options := base
	options.observers = observer.Clone(options.observers)
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	prefix = normalizePrefix(prefix, options.prefixSeparator)
	options.logger = normalizeLogger(options.logger)

	return &Namespace[K, V]{
		db:            db,
		prefix:        []byte(prefix),
		displayPrefix: prefix,
		keys:          keys,
		values:        values,
		opts:          options,
	}
}

// Raw returns the underlying Badger DB.
func (n *Namespace[K, V]) Raw() *badger.DB {
	if n == nil {
		return nil
	}
	return n.db
}

// Prefix returns the normalized namespace prefix.
func (n *Namespace[K, V]) Prefix() string {
	if n == nil {
		return ""
	}
	return n.displayPrefix
}

// Get reads a value from the namespace.
func (n *Namespace[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	ctx, start := n.startOperation(ctx)

	var (
		value V
		ok    bool
	)

	if err := n.validate("get"); err != nil {
		n.finishOperation(ctx, start, "get", err)
		return value, false, err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "get", err)
		return value, false, err
	}

	err := n.db.View(func(txn *badger.Txn) error {
		view := &viewTx[K, V]{
			namespace: n,
			txn:       txn,
			ctx:       ctx,
		}
		var err error
		value, ok, err = view.Get(key)
		return err
	})
	err = n.normalizeEngineError("get", err)
	n.finishOperation(ctx, start, "get", err)
	return value, ok, err
}

// Set writes a value to the namespace.
func (n *Namespace[K, V]) Set(ctx context.Context, key K, value V, opts ...SetOption) error {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("set"); err != nil {
		n.finishOperation(ctx, start, "set", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "set", err)
		return err
	}

	err := n.db.Update(func(txn *badger.Txn) error {
		update := &updateTx[K, V]{
			viewTx: viewTx[K, V]{
				namespace: n,
				txn:       txn,
				ctx:       ctx,
			},
		}
		return update.Set(key, value, opts...)
	})
	err = n.normalizeEngineError("set", err)
	n.finishOperation(ctx, start, "set", err)
	return err
}

// Delete removes a value from the namespace.
func (n *Namespace[K, V]) Delete(ctx context.Context, key K) error {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("delete"); err != nil {
		n.finishOperation(ctx, start, "delete", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "delete", err)
		return err
	}

	err := n.db.Update(func(txn *badger.Txn) error {
		update := &updateTx[K, V]{
			viewTx: viewTx[K, V]{
				namespace: n,
				txn:       txn,
				ctx:       ctx,
			},
		}
		return update.Delete(key)
	})
	err = n.normalizeEngineError("delete", err)
	n.finishOperation(ctx, start, "delete", err)
	return err
}

// Exists reports whether a key exists in the namespace.
func (n *Namespace[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("exists"); err != nil {
		n.finishOperation(ctx, start, "exists", err)
		return false, err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "exists", err)
		return false, err
	}

	var ok bool
	err := n.db.View(func(txn *badger.Txn) error {
		view := &viewTx[K, V]{
			namespace: n,
			txn:       txn,
			ctx:       ctx,
		}
		var err error
		ok, err = view.Exists(key)
		return err
	})
	err = n.normalizeEngineError("exists", err)
	n.finishOperation(ctx, start, "exists", err)
	return ok, err
}

// View runs a read-only transaction against the namespace.
func (n *Namespace[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("view"); err != nil {
		n.finishOperation(ctx, start, "view", err)
		return err
	}
	if fn == nil {
		err := n.wrapError("view", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
		n.finishOperation(ctx, start, "view", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "view", err)
		return err
	}

	err := n.db.View(func(txn *badger.Txn) error {
		return fn(&viewTx[K, V]{
			namespace: n,
			txn:       txn,
			ctx:       ctx,
		})
	})
	err = n.normalizeEngineError("view", err)
	n.finishOperation(ctx, start, "view", err)
	return err
}

// Update runs a writable transaction against the namespace.
func (n *Namespace[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("update"); err != nil {
		n.finishOperation(ctx, start, "update", err)
		return err
	}
	if fn == nil {
		err := n.wrapError("update", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
		n.finishOperation(ctx, start, "update", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "update", err)
		return err
	}

	err := n.db.Update(func(txn *badger.Txn) error {
		return fn(&updateTx[K, V]{
			viewTx: viewTx[K, V]{
				namespace: n,
				txn:       txn,
				ctx:       ctx,
			},
		})
	})
	err = n.normalizeEngineError("update", err)
	n.finishOperation(ctx, start, "update", err)
	return err
}

func (n *Namespace[K, V]) startOperation(ctx context.Context) (context.Context, time.Time) {
	return normalizeContext(ctx), time.Now()
}

func (n *Namespace[K, V]) finishOperation(ctx context.Context, startedAt time.Time, op string, err error) {
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		n.logger().Debug("storx badger operation failed", "namespace", n.safePrefix(), "op", op, "err", err)
	}

	observer.ObserveAll(ctx, n.observers(), observer.Event{
		Engine:     "badger",
		Target:     n.safePrefix(),
		TargetType: "namespace",
		Operation:  op,
		StartedAt:  startedAt,
		Duration:   time.Since(startedAt),
		Err:        err,
	})
}

func (n *Namespace[K, V]) validate(op string) error {
	switch {
	case n == nil:
		return oops.In("storx/badgerx").
			With("op", op).
			Wrapf(errors.Join(storx.ErrInvalidValue, storx.ErrClosed), "namespace wrapper is nil")
	case n.db == nil:
		return n.wrapError(op, errors.Join(storx.ErrInvalidValue, storx.ErrClosed), "database is nil")
	case n.keys == nil:
		return n.wrapError(op, errors.Join(storx.ErrKeyCodec, storx.ErrInvalidKey), "key codec is nil")
	case n.values == nil:
		return n.wrapError(op, errors.Join(storx.ErrCodec, storx.ErrInvalidValue), "value codec is nil")
	default:
		return nil
	}
}

func (n *Namespace[K, V]) encodeKey(op string, key K) ([]byte, error) {
	data, err := n.keys.EncodeKey(key)
	if err != nil {
		return nil, n.wrapError(op, errors.Join(storx.ErrKeyCodec, err), "encode key", "key_type", typeOf[K]())
	}
	return bytesx.Clone(data), nil
}

func (n *Namespace[K, V]) encodeValue(op string, value V) ([]byte, error) {
	data, err := n.values.Marshal(value)
	if err != nil {
		return nil, n.wrapError(op, errors.Join(storx.ErrCodec, err), "marshal value", "value_type", typeOf[V]())
	}
	return bytesx.Clone(data), nil
}

func (n *Namespace[K, V]) decodeValue(op string, data []byte) (V, error) {
	payload := data
	if n.opts.copyValue {
		payload = bytesx.Clone(data)
	}

	value, err := n.values.Unmarshal(payload)
	if err != nil {
		return value, n.wrapError(op, errors.Join(storx.ErrCodec, err), "unmarshal value", "value_type", typeOf[V](), "size", len(data))
	}
	return value, nil
}

func (n *Namespace[K, V]) fullKey(op string, key K) ([]byte, error) {
	encodedKey, err := n.encodeKey(op, key)
	if err != nil {
		return nil, err
	}

	fullKey := make([]byte, 0, len(n.prefix)+len(encodedKey))
	fullKey = append(fullKey, n.prefix...)
	fullKey = append(fullKey, encodedKey...)
	return fullKey, nil
}

func (n *Namespace[K, V]) fullPrefix(prefix []byte) []byte {
	full := make([]byte, 0, len(n.prefix)+len(prefix))
	full = append(full, n.prefix...)
	full = append(full, prefix...)
	return full
}

func (n *Namespace[K, V]) decodeUserKey(op string, fullKey []byte) (K, error) {
	var zero K
	if !bytes.HasPrefix(fullKey, n.prefix) {
		return zero, n.wrapError(op, errors.Join(storx.ErrKeyCodec, storx.ErrInvalidKey), "key is outside namespace", "full_key_size", len(fullKey))
	}

	key, err := n.keys.DecodeKey(bytesx.Clone(fullKey[len(n.prefix):]))
	if err != nil {
		return zero, n.wrapError(op, errors.Join(storx.ErrKeyCodec, err), "decode namespace key", "key_type", typeOf[K]())
	}
	return key, nil
}

func (n *Namespace[K, V]) readItemValue(op string, item *badger.Item) (V, error) {
	var zero V

	if n.opts.copyValue {
		data, err := item.ValueCopy(nil)
		if err != nil {
			return zero, n.wrapError(op, errors.Join(storx.ErrCodec, err), "copy item value")
		}
		return n.decodeValue(op, data)
	}

	var value V
	err := item.Value(func(data []byte) error {
		var err error
		value, err = n.values.Unmarshal(data)
		return err
	})
	if err != nil {
		return zero, n.wrapError(op, errors.Join(storx.ErrCodec, err), "read item value")
	}
	return value, nil
}

func (n *Namespace[K, V]) normalizeEngineError(op string, err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, badger.ErrDBClosed):
		return n.wrapError(op, errors.Join(storx.ErrClosed, err), "badger operation failed")
	default:
		return err
	}
}

func (n *Namespace[K, V]) wrapError(op string, err error, format string, attrs ...any) error {
	builder := oops.In("storx/badgerx").With("op", op, "namespace", n.safePrefix())
	if len(attrs) > 0 {
		builder = builder.With(attrs...)
	}
	return builder.Wrapf(err, "%s", format)
}

func (n *Namespace[K, V]) logger() *slog.Logger {
	if n == nil {
		return slog.Default()
	}
	return normalizeLogger(n.opts.logger)
}

func (n *Namespace[K, V]) observers() []observer.Observer {
	if n == nil {
		return nil
	}
	return n.opts.observers
}

func (n *Namespace[K, V]) safePrefix() string {
	if n == nil {
		return ""
	}
	return n.displayPrefix
}

func normalizePrefix(prefix, separator string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	if separator == "" || strings.HasSuffix(prefix, separator) {
		return prefix
	}
	return prefix + separator
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
