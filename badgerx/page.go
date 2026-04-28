package badgerx

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/dgraph-io/badger/v4"
)

// PageResult contains one page of typed entries plus an opaque cursor.
type PageResult[K any, V any] struct {
	Entries    []Entry[K, V]
	NextCursor string
	HasMore    bool
}

// Page returns one page of typed key/value pairs.
func (n *Namespace[K, V]) Page(ctx context.Context, cursor string, opts ...ListOption[K]) (PageResult[K, V], error) {
	options, err := n.resolveListOptions("page", opts)
	if err != nil {
		return PageResult[K, V]{}, err
	}

	cursorKey, err := decodePageCursor(cursor)
	if err != nil {
		return PageResult[K, V]{}, n.wrapError("page", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "invalid page cursor")
	}

	var fullCursor []byte
	if len(cursorKey) > 0 {
		fullCursor = append(append([]byte(nil), n.prefix...), cursorKey...)
		if options.reverse {
			options.end = fullCursor
		} else {
			options.start = fullCursor
		}
	}

	ctx, start := n.startOperation(ctx)
	if err := n.validate("page"); err != nil {
		n.finishOperation(ctx, start, "page", err)
		return PageResult[K, V]{}, err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "page", err)
		return PageResult[K, V]{}, err
	}

	var page PageResult[K, V]
	err = n.db.View(func(txn *badger.Txn) error {
		entries, hasMore, err := n.collectPageEntries(ctx, "page", txn, options, fullCursor)
		if err != nil {
			return err
		}
		page.Entries = entries
		page.HasMore = hasMore
		if hasMore && len(entries) > 0 {
			rawKey, err := n.encodeKey("page", page.Entries[len(page.Entries)-1].Key)
			if err != nil {
				return err
			}
			page.NextCursor = encodePageCursor(rawKey)
		}
		return nil
	})
	err = n.normalizeEngineError("page", err)
	n.finishOperation(ctx, start, "page", err)
	return page, err
}

func (n *Namespace[K, V]) collectPageEntries(
	ctx context.Context,
	op string,
	txn *badger.Txn,
	options resolvedListOptions,
	cursorKey []byte,
) ([]Entry[K, V], bool, error) {
	if hasEmptyRange(options) {
		return nil, false, nil
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
	skippedCursor := len(cursorKey) == 0

	for ; iteratorValid(it, options); it.Next() {
		if err := ctx.Err(); err != nil {
			return nil, false, err
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
		if !skippedCursor && bytes.Equal(rawKey, cursorKey) {
			skippedCursor = true
			continue
		}
		skippedCursor = true

		key, err := n.decodeUserKey(op, item.KeyCopy(nil))
		if err != nil {
			return nil, false, err
		}
		value, err := n.readItemValue(op, item)
		if err != nil {
			return nil, false, err
		}

		entries = append(entries, Entry[K, V]{Key: key, Value: value})
		if options.limit > 0 && len(entries) > options.limit {
			return entries[:options.limit], true, nil
		}
	}

	return entries, false, nil
}

func encodePageCursor(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodePageCursor(cursor string) ([]byte, error) {
	if cursor == "" {
		return nil, nil
	}
	return base64.RawURLEncoding.DecodeString(cursor)
}
