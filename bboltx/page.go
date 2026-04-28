package bboltx

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"

	storx "github.com/arcgolabs/storx"
	"go.etcd.io/bbolt"
)

// PageResult contains one page of typed entries plus an opaque cursor.
type PageResult[K any, V any] struct {
	Entries    []Entry[K, V]
	NextCursor string
	HasMore    bool
}

// Page returns one page of typed key/value pairs.
func (b *Bucket[K, V]) Page(ctx context.Context, cursor string, opts ...ListOption[K]) (PageResult[K, V], error) {
	options, err := b.resolveListOptions("page", opts)
	if err != nil {
		return PageResult[K, V]{}, err
	}

	cursorKey, err := decodePageCursor(cursor)
	if err != nil {
		return PageResult[K, V]{}, b.wrapError("page", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "invalid page cursor")
	}
	if len(cursorKey) > 0 {
		if options.reverse {
			options.end = cursorKey
		} else {
			options.start = cursorKey
		}
	}

	ctx, start := b.startOperation(ctx)
	if err := b.validate("page"); err != nil {
		b.finishOperation(ctx, start, "page", err)
		return PageResult[K, V]{}, err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, "page", err)
		return PageResult[K, V]{}, err
	}

	var page PageResult[K, V]
	err = b.db.View(func(tx *bbolt.Tx) error {
		view := b.newViewTx(ctx, tx.Bucket(b.nameBytes))
		entries, hasMore, err := b.collectPageEntries(ctx, "page", view, options, cursorKey)
		if err != nil {
			return err
		}
		page.Entries = entries
		page.HasMore = hasMore
		if hasMore && len(entries) > 0 {
			rawKey, err := b.encodeKey("page", entries[len(entries)-1].Key)
			if err != nil {
				return err
			}
			page.NextCursor = encodePageCursor(rawKey)
		}
		return nil
	})
	err = b.normalizeEngineError("page", err)
	b.finishOperation(ctx, start, "page", err)
	return page, err
}

func (b *Bucket[K, V]) collectPageEntries(
	ctx context.Context,
	op string,
	view *viewTx[K, V],
	options resolvedListOptions,
	cursorKey []byte,
) ([]Entry[K, V], bool, error) {
	bucket, err := view.readBucket(op)
	if err != nil || bucket == nil {
		return nil, false, err
	}
	if hasEmptyRange(options) {
		return nil, false, nil
	}

	cursor := bucket.Cursor()
	rawKey, rawValue := seekListCursor(cursor, options)
	entries := make([]Entry[K, V], 0, entryCapacity(options.limit))
	skippedCursor := len(cursorKey) == 0

	for rawKey != nil {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}

		stop, match := matchListKey(rawKey, options)
		if stop {
			break
		}
		if !match || rawValue == nil {
			rawKey, rawValue = stepListCursor(cursor, options.reverse)
			continue
		}
		if !skippedCursor && bytes.Equal(rawKey, cursorKey) {
			skippedCursor = true
			rawKey, rawValue = stepListCursor(cursor, options.reverse)
			continue
		}
		skippedCursor = true

		key, err := b.decodeKey(op, rawKey)
		if err != nil {
			return nil, false, err
		}
		value, err := b.decodeValue(op, rawValue)
		if err != nil {
			return nil, false, err
		}

		entries = append(entries, Entry[K, V]{Key: key, Value: value})
		if options.limit > 0 && len(entries) > options.limit {
			return entries[:options.limit], true, nil
		}

		rawKey, rawValue = stepListCursor(cursor, options.reverse)
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
