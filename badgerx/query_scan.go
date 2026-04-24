package badgerx

import (
	"bytes"
	"context"

	"github.com/arcgolabs/storx/internal/bytesx"
	"github.com/dgraph-io/badger/v4"
)

func (n *Namespace[K, V]) queryEntries(ctx context.Context, op string, options resolvedListOptions) ([]Entry[K, V], error) {
	ctx, start := n.startOperation(ctx)

	if err := n.validate(op); err != nil {
		n.finishOperation(ctx, start, op, err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, op, err)
		return nil, err
	}

	var entries []Entry[K, V]
	err := n.db.View(func(txn *badger.Txn) error {
		var err error
		entries, err = n.collectEntries(ctx, op, txn, options)
		return err
	})
	err = n.normalizeEngineError(op, err)
	n.finishOperation(ctx, start, op, err)
	return entries, err
}

func (n *Namespace[K, V]) collectEntries(
	ctx context.Context,
	op string,
	txn *badger.Txn,
	options resolvedListOptions,
) ([]Entry[K, V], error) {
	if hasEmptyRange(options) {
		return nil, nil
	}

	iteratorOptions := badger.DefaultIteratorOptions
	iteratorOptions.PrefetchValues = n.opts.copyValue
	iteratorOptions.Reverse = options.reverse
	if len(options.prefix) > 0 {
		iteratorOptions.Prefix = options.prefix
	}

	it := txn.NewIterator(iteratorOptions)
	defer it.Close()

	seekIterator(it, options)

	entries := make([]Entry[K, V], 0, entryCapacity(options.limit))
	for ; iteratorValid(it, options); it.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		item := it.Item()
		rawKey := item.Key()
		stop, match := matchListKey(rawKey, options)
		if stop {
			break
		}
		if !match {
			continue
		}

		key, err := n.decodeUserKey(op, item.KeyCopy(nil))
		if err != nil {
			return nil, err
		}
		value, err := n.readItemValue(op, item)
		if err != nil {
			return nil, err
		}

		entries = append(entries, Entry[K, V]{
			Key:   key,
			Value: value,
		})
		if reachedLimit(len(entries), options.limit) {
			break
		}
	}

	return entries, nil
}

func seekIterator(it *badger.Iterator, options resolvedListOptions) {
	if options.reverse {
		seekIteratorReverse(it, options)
		return
	}

	seek := options.start
	if len(options.prefix) > 0 && (seek == nil || bytes.Compare(seek, options.prefix) < 0) {
		seek = options.prefix
	}
	if seek != nil {
		it.Seek(seek)
		return
	}
	it.Rewind()
}

func seekIteratorReverse(it *badger.Iterator, options resolvedListOptions) {
	seek := options.end
	exclusive := false

	if len(options.prefix) > 0 {
		prefixEnd := bytesx.PrefixSuccessor(options.prefix)
		if prefixEnd != nil && (seek == nil || bytes.Compare(seek, prefixEnd) >= 0) {
			seek = prefixEnd
			exclusive = true
		}
	}

	switch {
	case seek == nil:
		it.Rewind()
	case exclusive:
		it.Seek(seek)
		if it.Valid() && bytes.Equal(it.Item().Key(), seek) {
			it.Next()
		}
	default:
		it.Seek(seek)
	}
}

func iteratorValid(it *badger.Iterator, options resolvedListOptions) bool {
	if len(options.prefix) > 0 {
		return it.ValidForPrefix(options.prefix)
	}
	return it.Valid()
}

func hasEmptyRange(options resolvedListOptions) bool {
	return options.start != nil && options.end != nil && bytes.Compare(options.start, options.end) > 0
}

func matchListKey(rawKey []byte, options resolvedListOptions) (bool, bool) {
	if len(options.prefix) > 0 && !bytes.HasPrefix(rawKey, options.prefix) {
		return true, false
	}

	if options.start != nil {
		startCompare := bytes.Compare(rawKey, options.start)
		if options.reverse {
			if startCompare < 0 {
				return true, false
			}
		} else if startCompare < 0 {
			return false, false
		}
	}

	if options.end != nil {
		endCompare := bytes.Compare(rawKey, options.end)
		if options.reverse {
			if endCompare > 0 {
				return false, false
			}
		} else if endCompare > 0 {
			return true, false
		}
	}

	return false, true
}

func reachedLimit(length, limit int) bool {
	return limit > 0 && length >= limit
}

func entryCapacity(limit int) int {
	if limit <= 0 {
		return 0
	}
	return limit
}
