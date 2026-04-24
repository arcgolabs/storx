package bboltx

import (
	"bytes"
	"context"
	"errors"

	storx "github.com/arcgolabs/storx"
	"go.etcd.io/bbolt"
)

type ViewTx[K any, V any] interface {
	Get(key K) (V, bool, error)
	Exists(key K) (bool, error)
	ForEach(fn func(key K, value V) error) error
	ScanPrefix(prefix []byte, fn func(key K, value V) error) error
	Cursor() Cursor[K, V]
}

type UpdateTx[K any, V any] interface {
	ViewTx[K, V]

	Put(key K, value V) error
	Delete(key K) error
	NextSequence() (uint64, error)
}

type viewTx[K any, V any] struct {
	bucket *bbolt.Bucket
	owner  *Bucket[K, V]
	ctx    context.Context
}

type updateTx[K any, V any] struct {
	viewTx[K, V]
}

func (tx *viewTx[K, V]) Get(key K) (V, bool, error) {
	var zero V

	if err := tx.ctx.Err(); err != nil {
		return zero, false, err
	}

	bucket, err := tx.readBucket("get")
	if err != nil || bucket == nil {
		return zero, false, err
	}

	encodedKey, err := tx.owner.encodeKey("get", key)
	if err != nil {
		return zero, false, err
	}

	rawValue := bucket.Get(encodedKey)
	if rawValue == nil {
		return zero, false, nil
	}

	value, err := tx.owner.decodeValue("get", rawValue)
	if err != nil {
		return zero, false, err
	}

	return value, true, nil
}

func (tx *viewTx[K, V]) Exists(key K) (bool, error) {
	if err := tx.ctx.Err(); err != nil {
		return false, err
	}

	bucket, err := tx.readBucket("exists")
	if err != nil || bucket == nil {
		return false, err
	}

	encodedKey, err := tx.owner.encodeKey("exists", key)
	if err != nil {
		return false, err
	}

	return bucket.Get(encodedKey) != nil, nil
}

func (tx *viewTx[K, V]) ForEach(fn func(key K, value V) error) error {
	if fn == nil {
		return tx.owner.wrapError("for_each", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
	}

	bucket, err := tx.readBucket("for_each")
	if err != nil || bucket == nil {
		return err
	}

	return bucket.ForEach(func(rawKey, rawValue []byte) error {
		if err := tx.ctx.Err(); err != nil {
			return err
		}
		if rawValue == nil {
			return nil
		}

		key, err := tx.owner.decodeKey("for_each", rawKey)
		if err != nil {
			return err
		}

		value, err := tx.owner.decodeValue("for_each", rawValue)
		if err != nil {
			return err
		}

		return fn(key, value)
	})
}

func (tx *viewTx[K, V]) ScanPrefix(prefix []byte, fn func(key K, value V) error) error {
	if fn == nil {
		return tx.owner.wrapError("scan_prefix", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
	}

	bucket, err := tx.readBucket("scan_prefix")
	if err != nil || bucket == nil {
		return err
	}

	cursor := bucket.Cursor()
	rawKey, rawValue := cursor.Seek(prefix)
	for rawKey != nil {
		if err := tx.ctx.Err(); err != nil {
			return err
		}
		if !bytes.HasPrefix(rawKey, prefix) {
			return nil
		}
		if rawValue == nil {
			rawKey, rawValue = cursor.Next()
			continue
		}

		key, err := tx.owner.decodeKey("scan_prefix", rawKey)
		if err != nil {
			return err
		}
		value, err := tx.owner.decodeValue("scan_prefix", rawValue)
		if err != nil {
			return err
		}
		if err := fn(key, value); err != nil {
			return err
		}

		rawKey, rawValue = cursor.Next()
	}

	return nil
}

func (tx *viewTx[K, V]) Cursor() Cursor[K, V] {
	bucket, err := tx.readBucket("cursor")
	if err != nil {
		return &cursor[K, V]{ctx: tx.ctx, owner: tx.owner, err: err}
	}
	if bucket == nil {
		return &cursor[K, V]{ctx: tx.ctx, owner: tx.owner}
	}
	return &cursor[K, V]{ctx: tx.ctx, owner: tx.owner, cursor: bucket.Cursor()}
}

func (tx *viewTx[K, V]) readBucket(op string) (*bbolt.Bucket, error) {
	if tx.bucket != nil {
		return tx.bucket, nil
	}
	if tx.owner.opts.readOnlyMissingAsEmpty {
		return nil, nil
	}
	return nil, tx.owner.wrapError(op, storx.ErrBucketNotFound, "bucket does not exist")
}

func (tx *updateTx[K, V]) Put(key K, value V) error {
	if err := tx.ctx.Err(); err != nil {
		return err
	}

	encodedKey, err := tx.owner.encodeKey("put", key)
	if err != nil {
		return err
	}
	encodedValue, err := tx.owner.encodeValue("put", value)
	if err != nil {
		return err
	}

	return tx.bucket.Put(encodedKey, encodedValue)
}

func (tx *updateTx[K, V]) Delete(key K) error {
	if err := tx.ctx.Err(); err != nil {
		return err
	}

	encodedKey, err := tx.owner.encodeKey("delete", key)
	if err != nil {
		return err
	}
	return tx.bucket.Delete(encodedKey)
}

func (tx *updateTx[K, V]) NextSequence() (uint64, error) {
	if err := tx.ctx.Err(); err != nil {
		return 0, err
	}
	return tx.bucket.NextSequence()
}
