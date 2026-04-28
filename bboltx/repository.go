package bboltx

import (
	"context"

	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"go.etcd.io/bbolt"
)

// Repository is a thin business-facing wrapper over a typed bbolt bucket.
type Repository[K any, V any] struct {
	bucket *Bucket[K, V]
}

// NewRepository creates a repository backed by a typed bucket.
func NewRepository[K any, V any](
	db *bbolt.DB,
	name string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...BucketOption,
) *Repository[K, V] {
	return &Repository[K, V]{
		bucket: NewBucket(db, name, keys, values, opts...),
	}
}

// NewRepositoryWithDB creates a repository backed by a DB wrapper.
func NewRepositoryWithDB[K any, V any](
	db *DB,
	name string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	opts ...BucketOption,
) *Repository[K, V] {
	return &Repository[K, V]{
		bucket: NewBucketWithDB(db, name, keys, values, opts...),
	}
}

// WrapRepository adapts an existing typed bucket to the repository facade.
func WrapRepository[K any, V any](bucket *Bucket[K, V]) *Repository[K, V] {
	return &Repository[K, V]{bucket: bucket}
}

func (r *Repository[K, V]) Bucket() *Bucket[K, V] {
	if r == nil {
		return nil
	}
	return r.bucket
}

func (r *Repository[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	return r.Bucket().Get(ctx, key)
}

func (r *Repository[K, V]) Save(ctx context.Context, key K, value V) error {
	return r.Bucket().Put(ctx, key, value)
}

func (r *Repository[K, V]) SaveMany(ctx context.Context, entries ...Entry[K, V]) error {
	return r.Bucket().PutMany(ctx, entries...)
}

func (r *Repository[K, V]) Delete(ctx context.Context, key K) error {
	return r.Bucket().Delete(ctx, key)
}

func (r *Repository[K, V]) DeleteMany(ctx context.Context, keys ...K) error {
	return r.Bucket().DeleteMany(ctx, keys...)
}

func (r *Repository[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	return r.Bucket().Exists(ctx, key)
}

func (r *Repository[K, V]) GetMany(ctx context.Context, keys ...K) ([]Lookup[K, V], error) {
	return r.Bucket().GetMany(ctx, keys...)
}

func (r *Repository[K, V]) List(ctx context.Context, opts ...ListOption[K]) ([]Entry[K, V], error) {
	return r.Bucket().List(ctx, opts...)
}

func (r *Repository[K, V]) Iter(ctx context.Context, opts ...ListOption[K]) (Iterator[K, V], error) {
	return r.Bucket().Iter(ctx, opts...)
}

func (r *Repository[K, V]) Walk(ctx context.Context, fn func(entry Entry[K, V]) error, opts ...ListOption[K]) error {
	return r.Bucket().Walk(ctx, fn, opts...)
}

func (r *Repository[K, V]) View(ctx context.Context, fn func(tx ViewTx[K, V]) error) error {
	return r.Bucket().View(ctx, fn)
}

func (r *Repository[K, V]) Update(ctx context.Context, fn func(tx UpdateTx[K, V]) error) error {
	return r.Bucket().Update(ctx, fn)
}

func (r *Repository[K, V]) NextSequence(ctx context.Context) (uint64, error) {
	return r.Bucket().NextSequence(ctx)
}
