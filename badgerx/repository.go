package badgerx

import (
	"context"

	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/dgraph-io/badger/v4"
)

// Repository is a thin business-facing wrapper over a typed Badger namespace.
type Repository[K any, V any] struct {
	namespace *Namespace[K, V]
}

// NewRepository creates a repository backed by a typed namespace.
func NewRepository[K any, V any](
	db *badger.DB,
	prefix string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...NamespaceOption,
) *Repository[K, V] {
	return &Repository[K, V]{
		namespace: NewNamespace(db, prefix, keys, values, opts...),
	}
}

// NewRepositoryWithDB creates a repository backed by a DB wrapper.
func NewRepositoryWithDB[K any, V any](
	db *DB,
	prefix string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...NamespaceOption,
) *Repository[K, V] {
	return &Repository[K, V]{
		namespace: NewNamespaceWithDB(db, prefix, keys, values, opts...),
	}
}

// WrapRepository adapts an existing typed namespace to the repository facade.
func WrapRepository[K any, V any](namespace *Namespace[K, V]) *Repository[K, V] {
	return &Repository[K, V]{namespace: namespace}
}

func (r *Repository[K, V]) Namespace() *Namespace[K, V] {
	if r == nil {
		return nil
	}
	return r.namespace
}

func (r *Repository[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	return r.Namespace().Get(ctx, key)
}

func (r *Repository[K, V]) GetRecord(ctx context.Context, key K) (Record[K, V], bool, error) {
	return r.Namespace().GetRecord(ctx, key)
}

func (r *Repository[K, V]) GetMetadata(ctx context.Context, key K) (Metadata, bool, error) {
	return r.Namespace().GetMetadata(ctx, key)
}

func (r *Repository[K, V]) Save(ctx context.Context, key K, value V, opts ...SetOption) error {
	return r.Namespace().Set(ctx, key, value, opts...)
}

func (r *Repository[K, V]) SaveMany(ctx context.Context, entries []Entry[K, V], opts ...SetOption) error {
	return r.Namespace().SetMany(ctx, entries, opts...)
}

func (r *Repository[K, V]) Delete(ctx context.Context, key K) error {
	return r.Namespace().Delete(ctx, key)
}

func (r *Repository[K, V]) DeleteMany(ctx context.Context, keys ...K) error {
	return r.Namespace().DeleteMany(ctx, keys...)
}

func (r *Repository[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	return r.Namespace().Exists(ctx, key)
}

func (r *Repository[K, V]) GetMany(ctx context.Context, keys ...K) ([]Lookup[K, V], error) {
	return r.Namespace().GetMany(ctx, keys...)
}

func (r *Repository[K, V]) List(ctx context.Context, opts ...ListOption[K]) ([]Entry[K, V], error) {
	return r.Namespace().List(ctx, opts...)
}

func (r *Repository[K, V]) Iter(ctx context.Context, opts ...ListOption[K]) (Iterator[K, V], error) {
	return r.Namespace().Iter(ctx, opts...)
}

func (r *Repository[K, V]) Walk(ctx context.Context, fn func(entry Entry[K, V]) error, opts ...ListOption[K]) error {
	return r.Namespace().Walk(ctx, fn, opts...)
}

func (r *Repository[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error {
	return r.Namespace().View(ctx, fn)
}

func (r *Repository[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error {
	return r.Namespace().Update(ctx, fn)
}

func (r *Repository[K, V]) RunValueLogGC(ctx context.Context, discardRatio float64) error {
	return r.Namespace().RunValueLogGC(ctx, discardRatio)
}
