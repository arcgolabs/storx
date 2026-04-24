package bboltx

import (
	"bytes"
	"context"

	"github.com/arcgolabs/storx/internal/bytesx"
	"go.etcd.io/bbolt"
)

func (b *Bucket[K, V]) queryEntries(ctx context.Context, op string, options resolvedListOptions) ([]Entry[K, V], error) {
	ctx, start := b.startOperation(ctx)

	if err := b.validate(op); err != nil {
		b.finishOperation(ctx, start, op, err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		b.finishOperation(ctx, start, op, err)
		return nil, err
	}

	var entries []Entry[K, V]
	err := b.db.View(func(tx *bbolt.Tx) error {
		view := b.newViewTx(ctx, tx.Bucket(b.nameBytes))
		var err error
		entries, err = b.collectEntries(ctx, op, view, options)
		return err
	})
	err = b.normalizeEngineError(op, err)
	b.finishOperation(ctx, start, op, err)
	return entries, err
}

func (b *Bucket[K, V]) collectEntries(
	ctx context.Context,
	op string,
	view *viewTx[K, V],
	options resolvedListOptions,
) ([]Entry[K, V], error) {
	bucket, err := view.readBucket(op)
	if err != nil || bucket == nil {
		return nil, err
	}
	if hasEmptyRange(options) {
		return nil, nil
	}

	cursor := bucket.Cursor()
	rawKey, rawValue := seekListCursor(cursor, options)
	entries := make([]Entry[K, V], 0, entryCapacity(options.limit))

	for rawKey != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		stop, match := matchListKey(rawKey, options)
		if stop {
			break
		}
		if !match || rawValue == nil {
			rawKey, rawValue = stepListCursor(cursor, options.reverse)
			continue
		}

		key, err := b.decodeKey(op, rawKey)
		if err != nil {
			return nil, err
		}
		value, err := b.decodeValue(op, rawValue)
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

		rawKey, rawValue = stepListCursor(cursor, options.reverse)
	}

	return entries, nil
}

func seekListCursor(cursor *bbolt.Cursor, options resolvedListOptions) ([]byte, []byte) {
	if options.reverse {
		return seekListCursorReverse(cursor, options)
	}

	seek := options.start
	if len(options.prefix) > 0 && (seek == nil || bytes.Compare(seek, options.prefix) < 0) {
		seek = options.prefix
	}
	if seek != nil {
		return cursor.Seek(seek)
	}
	return cursor.First()
}

func seekListCursorReverse(cursor *bbolt.Cursor, options resolvedListOptions) ([]byte, []byte) {
	seek := options.end
	exclusive := false

	if len(options.prefix) > 0 {
		prefixEnd := bytesx.PrefixSuccessor(options.prefix)
		if prefixEnd != nil && (seek == nil || bytes.Compare(seek, prefixEnd) >= 0) {
			seek = prefixEnd
			exclusive = true
		}
	}

	if seek == nil {
		return cursor.Last()
	}

	rawKey, rawValue := cursor.Seek(seek)
	switch {
	case rawKey == nil:
		return cursor.Last()
	case exclusive, bytes.Compare(rawKey, seek) > 0:
		return cursor.Prev()
	default:
		return rawKey, rawValue
	}
}

func stepListCursor(cursor *bbolt.Cursor, reverse bool) ([]byte, []byte) {
	if reverse {
		return cursor.Prev()
	}
	return cursor.Next()
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
