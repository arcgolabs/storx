package badgerx

import (
	"context"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/internal/bytesx"
	"github.com/dgraph-io/badger/v4"
)

// Entry is a typed key/value pair returned by convenience query APIs.
type Entry[K any, V any] struct {
	Key   K
	Value V
}

// Lookup is a typed get-many result that preserves the requested key and found
// status.
type Lookup[K any, V any] struct {
	Key   K
	Value V
	Found bool
}

// ListOptions controls typed list-style scans.
type ListOptions[K any] struct {
	Prefix  []byte
	Start   *K
	End     *K
	Limit   int
	Reverse bool
}

// ListOption mutates typed list options.
type ListOption[K any] func(*ListOptions[K])

// WithPrefix restricts list operations to keys with the provided encoded
// prefix.
func WithPrefix[K any](prefix []byte) ListOption[K] {
	return func(opts *ListOptions[K]) {
		opts.Prefix = bytesx.Clone(prefix)
	}
}

// WithStart sets an inclusive lower bound for ordered list operations.
func WithStart[K any](start K) ListOption[K] {
	return func(opts *ListOptions[K]) {
		opts.Start = &start
	}
}

// WithEnd sets an inclusive upper bound for ordered list operations.
func WithEnd[K any](end K) ListOption[K] {
	return func(opts *ListOptions[K]) {
		opts.End = &end
	}
}

// WithLimit caps the number of results returned by list operations.
func WithLimit[K any](limit int) ListOption[K] {
	return func(opts *ListOptions[K]) {
		opts.Limit = limit
	}
}

// WithReverse scans results in descending key order.
func WithReverse[K any](enabled bool) ListOption[K] {
	return func(opts *ListOptions[K]) {
		opts.Reverse = enabled
	}
}

type resolvedListOptions struct {
	prefix  []byte
	start   []byte
	end     []byte
	limit   int
	reverse bool
}

// First returns the first typed key/value pair in ascending key order.
func (n *Namespace[K, V]) First(ctx context.Context) (Entry[K, V], bool, error) {
	entries, err := n.queryEntries(ctx, "first", resolvedListOptions{limit: 1})
	if err != nil {
		return Entry[K, V]{}, false, err
	}
	if len(entries) == 0 {
		return Entry[K, V]{}, false, nil
	}
	return entries[0], true, nil
}

// Last returns the last typed key/value pair in descending key order.
func (n *Namespace[K, V]) Last(ctx context.Context) (Entry[K, V], bool, error) {
	entries, err := n.queryEntries(ctx, "last", resolvedListOptions{limit: 1, reverse: true})
	if err != nil {
		return Entry[K, V]{}, false, err
	}
	if len(entries) == 0 {
		return Entry[K, V]{}, false, nil
	}
	return entries[0], true, nil
}

// List returns typed key/value pairs matching the provided options.
func (n *Namespace[K, V]) List(ctx context.Context, opts ...ListOption[K]) ([]Entry[K, V], error) {
	options, err := n.resolveListOptions("list", opts)
	if err != nil {
		return nil, err
	}
	return n.queryEntries(ctx, "list", options)
}

// Keys returns typed keys matching the provided options.
func (n *Namespace[K, V]) Keys(ctx context.Context, opts ...ListOption[K]) ([]K, error) {
	options, err := n.resolveListOptions("keys", opts)
	if err != nil {
		return nil, err
	}

	entries, err := n.queryEntries(ctx, "keys", options)
	if err != nil {
		return nil, err
	}

	keys := make([]K, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return keys, nil
}

// Values returns typed values matching the provided options.
func (n *Namespace[K, V]) Values(ctx context.Context, opts ...ListOption[K]) ([]V, error) {
	options, err := n.resolveListOptions("values", opts)
	if err != nil {
		return nil, err
	}

	entries, err := n.queryEntries(ctx, "values", options)
	if err != nil {
		return nil, err
	}

	values := make([]V, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Value)
	}
	return values, nil
}

// GetMany reads multiple keys in a single read transaction.
func (n *Namespace[K, V]) GetMany(ctx context.Context, keys ...K) ([]Lookup[K, V], error) {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("get_many"); err != nil {
		n.finishOperation(ctx, start, "get_many", err)
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "get_many", err)
		return nil, err
	}

	results := make([]Lookup[K, V], 0, len(keys))
	err := n.db.View(func(txn *badger.Txn) error {
		view := &viewTx[K, V]{
			namespace: n,
			txn:       txn,
			ctx:       ctx,
		}
		for _, key := range keys {
			value, ok, err := view.Get(key)
			if err != nil {
				return err
			}
			results = append(results, Lookup[K, V]{
				Key:   key,
				Value: value,
				Found: ok,
			})
		}
		return nil
	})
	err = n.normalizeEngineError("get_many", err)
	n.finishOperation(ctx, start, "get_many", err)
	return results, err
}

func (n *Namespace[K, V]) resolveListOptions(op string, opts []ListOption[K]) (resolvedListOptions, error) {
	options := ListOptions[K]{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}

	if options.Limit < 0 {
		return resolvedListOptions{}, n.wrapError(op, errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "limit must be non-negative")
	}

	resolved := resolvedListOptions{
		limit:   options.Limit,
		reverse: options.Reverse,
	}

	if len(options.Prefix) > 0 {
		resolved.prefix = n.fullPrefix(options.Prefix)
	}
	if options.Start != nil {
		start, err := n.fullKey(op, *options.Start)
		if err != nil {
			return resolvedListOptions{}, err
		}
		resolved.start = start
	}
	if options.End != nil {
		end, err := n.fullKey(op, *options.End)
		if err != nil {
			return resolvedListOptions{}, err
		}
		resolved.end = end
	}

	return resolved, nil
}
