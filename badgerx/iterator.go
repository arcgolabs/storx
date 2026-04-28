package badgerx

import (
	"context"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/dgraph-io/badger/v4"
)

// Iterator streams typed namespace entries without materializing them all in
// memory.
type Iterator[K any, V any] interface {
	Next() (Entry[K, V], bool, error)
	Close() error
}

type iterator[K any, V any] struct {
	ctx     context.Context
	owner   *Namespace[K, V]
	txn     *badger.Txn
	iter    *badger.Iterator
	options resolvedListOptions
	started bool
	closed  bool
}

// Iter opens a streaming read iterator over the namespace.
func (n *Namespace[K, V]) Iter(ctx context.Context, opts ...ListOption[K]) (Iterator[K, V], error) {
	if err := n.validate("iter"); err != nil {
		return nil, err
	}

	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	options, err := n.resolveListOptions("iter", opts)
	if err != nil {
		return nil, err
	}

	txn := n.db.NewTransaction(false)
	iteratorOptions := badger.DefaultIteratorOptions
	iteratorOptions.PrefetchValues = n.opts.copyValue
	iteratorOptions.Reverse = options.reverse
	if len(options.prefix) > 0 {
		iteratorOptions.Prefix = options.prefix
	}

	it := txn.NewIterator(iteratorOptions)
	if !hasEmptyRange(options) {
		seekIterator(it, options)
	}

	return &iterator[K, V]{
		ctx:     ctx,
		owner:   n,
		txn:     txn,
		iter:    it,
		options: options,
	}, nil
}

// Walk streams matching entries to fn.
func (n *Namespace[K, V]) Walk(ctx context.Context, fn func(entry Entry[K, V]) error, opts ...ListOption[K]) error {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("walk"); err != nil {
		n.finishOperation(ctx, start, "walk", err)
		return err
	}
	if fn == nil {
		err := n.wrapError("walk", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
		n.finishOperation(ctx, start, "walk", err)
		return err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "walk", err)
		return err
	}

	iter, err := n.Iter(ctx, opts...)
	if err != nil {
		n.finishOperation(ctx, start, "walk", err)
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

	n.finishOperation(ctx, start, "walk", err)
	return err
}

func (it *iterator[K, V]) Next() (Entry[K, V], bool, error) {
	var zero Entry[K, V]

	if it == nil || it.closed || it.iter == nil {
		return zero, false, nil
	}
	if err := it.ctx.Err(); err != nil {
		_ = it.Close()
		return zero, false, err
	}
	if hasEmptyRange(it.options) {
		_ = it.Close()
		return zero, false, nil
	}

	for {
		if !it.positioned() {
			_ = it.Close()
			return zero, false, nil
		}

		item := it.iter.Item()
		rawKey := item.Key()
		stop, match := matchListKey(rawKey, it.options)
		if stop {
			_ = it.Close()
			return zero, false, nil
		}
		if !match {
			it.iter.Next()
			continue
		}

		key, err := it.owner.decodeUserKey("iter", item.KeyCopy(nil))
		if err != nil {
			_ = it.Close()
			return zero, false, err
		}
		value, err := it.owner.readItemValue("iter", item)
		if err != nil {
			_ = it.Close()
			return zero, false, err
		}

		it.iter.Next()
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
	if it.iter != nil {
		it.iter.Close()
	}
	if it.txn != nil {
		it.txn.Discard()
	}
	return nil
}

func (it *iterator[K, V]) positioned() bool {
	if !it.started {
		it.started = true
		return iteratorValid(it.iter, it.options)
	}
	return iteratorValid(it.iter, it.options)
}
