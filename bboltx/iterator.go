package bboltx

import (
	"context"
	"errors"

	storx "github.com/arcgolabs/storx"
	"go.etcd.io/bbolt"
)

// Iterator streams typed bucket entries without materializing them all in
// memory.
type Iterator[K any, V any] interface {
	Next() (Entry[K, V], bool, error)
	Close() error
}

type iterator[K any, V any] struct {
	ctx     context.Context
	owner   *Bucket[K, V]
	tx      *bbolt.Tx
	cursor  *bbolt.Cursor
	options resolvedListOptions
	rawKey  []byte
	rawVal  []byte
	started bool
	closed  bool
}

// Iter opens a streaming read iterator over the bucket.
func (b *Bucket[K, V]) Iter(ctx context.Context, opts ...ListOption[K]) (Iterator[K, V], error) {
	if err := b.validate("iter"); err != nil {
		return nil, err
	}

	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options, err := b.resolveListOptions("iter", opts)
	if err != nil {
		return nil, err
	}

	tx, err := b.db.Begin(false)
	if err != nil {
		return nil, b.normalizeEngineError("iter", err)
	}

	var (
		cursor *bbolt.Cursor
		rawKey []byte
		rawVal []byte
	)

	bucket := tx.Bucket(b.nameBytes)
	switch {
	case bucket == nil && b.opts.readOnlyMissingAsEmpty:
	case bucket == nil:
		_ = tx.Rollback()
		return nil, b.wrapError("iter", storx.ErrBucketNotFound, "bucket does not exist")
	case hasEmptyRange(options):
	default:
		cursor = bucket.Cursor()
		rawKey, rawVal = seekListCursor(cursor, options)
	}

	return &iterator[K, V]{
		ctx:     ctx,
		owner:   b,
		tx:      tx,
		cursor:  cursor,
		options: options,
		rawKey:  rawKey,
		rawVal:  rawVal,
	}, nil
}

// Walk streams matching entries to fn.
func (b *Bucket[K, V]) Walk(ctx context.Context, fn func(entry Entry[K, V]) error, opts ...ListOption[K]) error {
	ctx, start := b.startOperation(ctx)

	if err := b.validate("walk"); err != nil {
		b.finishOperation(ctx, start, "walk", err)
		return err
	}
	if fn == nil {
		err := b.wrapError("walk", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
		b.finishOperation(ctx, start, "walk", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "walk", err)
		return err
	}

	iter, err := b.Iter(ctx, opts...)
	if err != nil {
		b.finishOperation(ctx, start, "walk", err)
		return err
	}
	defer func() {
		closeErr := iter.Close()
		if err == nil {
			err = closeErr
		}
	}()

	for {
		entry, ok, nextErr := iter.Next()
		if nextErr != nil {
			err = nextErr
			break
		}
		if !ok {
			break
		}
		if callErr := fn(entry); callErr != nil {
			err = callErr
			break
		}
	}

	b.finishOperation(ctx, start, "walk", err)
	return err
}

func (it *iterator[K, V]) Next() (Entry[K, V], bool, error) {
	var zero Entry[K, V]

	if it == nil || it.closed || it.cursor == nil {
		return zero, false, nil
	}
	if err := it.ctx.Err(); err != nil {
		_ = it.Close()
		return zero, false, err
	}

	for {
		rawKey, rawVal := it.current()
		if rawKey == nil {
			_ = it.Close()
			return zero, false, nil
		}

		stop, match := matchListKey(rawKey, it.options)
		if stop {
			_ = it.Close()
			return zero, false, nil
		}
		if !match || rawVal == nil {
			it.advance()
			continue
		}

		key, err := it.owner.decodeKey("iter", rawKey)
		if err != nil {
			_ = it.Close()
			return zero, false, err
		}
		value, err := it.owner.decodeValue("iter", rawVal)
		if err != nil {
			_ = it.Close()
			return zero, false, err
		}

		it.advance()
		return Entry[K, V]{
			Key:   key,
			Value: value,
		}, true, nil
	}
}

func (it *iterator[K, V]) Close() error {
	if it == nil || it.closed {
		return nil
	}
	it.closed = true
	if it.tx == nil {
		return nil
	}
	return it.tx.Rollback()
}

func (it *iterator[K, V]) current() ([]byte, []byte) {
	if !it.started {
		it.started = true
		return it.rawKey, it.rawVal
	}
	return it.rawKey, it.rawVal
}

func (it *iterator[K, V]) advance() {
	if it.cursor == nil {
		it.rawKey, it.rawVal = nil, nil
		return
	}
	it.rawKey, it.rawVal = stepListCursor(it.cursor, it.options.reverse)
}
