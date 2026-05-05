package badgerx

import (
	"context"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

// UniqueIndexQuery builds a read/delete operation backed by one unique
// secondary index key.
type UniqueIndexQuery[K any, V any, IK any] struct {
	index     *SecondaryIndex[K, V, IK]
	store     *ModelStore[K, V]
	key       IK
	predicate func(Entry[K, V]) bool
}

// ManyIndexQuery builds a read/delete operation backed by one non-unique
// secondary index key.
type ManyIndexQuery[K any, V any, IK any] struct {
	index     *SecondaryIndexMany[K, V, IK]
	store     *ModelStore[K, V]
	key       IK
	predicate func(Entry[K, V]) bool
	limit     int
}

// OrderedIndexQuery builds a read/delete/page operation backed by one ordered
// secondary index key.
type OrderedIndexQuery[K any, V any, IK any, SK any] struct {
	index     *SecondaryIndexOrdered[K, V, IK, SK]
	store     *ModelStore[K, V]
	key       IK
	predicate func(Entry[K, V]) bool
	limit     int
	reverse   bool
}

// Query starts a unique-index query for one secondary key.
func (i *SecondaryIndex[K, V, IK]) Query(store *ModelStore[K, V], key IK) UniqueIndexQuery[K, V, IK] {
	return UniqueIndexQuery[K, V, IK]{index: i, store: store, key: key}
}

// Query starts a non-unique-index query for one secondary key.
func (i *SecondaryIndexMany[K, V, IK]) Query(store *ModelStore[K, V], key IK) ManyIndexQuery[K, V, IK] {
	return ManyIndexQuery[K, V, IK]{index: i, store: store, key: key}
}

// Query starts an ordered-index query for one secondary key.
func (i *SecondaryIndexOrdered[K, V, IK, SK]) Query(store *ModelStore[K, V], key IK) OrderedIndexQuery[K, V, IK, SK] {
	return OrderedIndexQuery[K, V, IK, SK]{index: i, store: store, key: key}
}

// Where filters loaded entries in memory after the indexed lookup.
func (q UniqueIndexQuery[K, V, IK]) Where(predicate func(V) bool) UniqueIndexQuery[K, V, IK] {
	if predicate == nil {
		q.predicate = nil
		return q
	}
	q.predicate = func(entry Entry[K, V]) bool {
		return predicate(entry.Value)
	}
	return q
}

// WhereEntry filters loaded entries in memory after the indexed lookup.
func (q UniqueIndexQuery[K, V, IK]) WhereEntry(predicate func(Entry[K, V]) bool) UniqueIndexQuery[K, V, IK] {
	q.predicate = predicate
	return q
}

// FirstEntry returns the indexed entry when it exists and passes the predicate.
func (q UniqueIndexQuery[K, V, IK]) FirstEntry(ctx context.Context) (Entry[K, V], bool, error) {
	primary, ok, err := q.index.GetPrimary(ctx, q.key)
	if err != nil || !ok {
		return Entry[K, V]{}, ok, err
	}
	value, ok, err := q.store.Get(ctx, primary)
	if err != nil || !ok {
		return Entry[K, V]{}, ok, err
	}
	entry := Entry[K, V]{Key: primary, Value: value}
	if q.predicate != nil && !q.predicate(entry) {
		return Entry[K, V]{}, false, nil
	}
	return entry, true, nil
}

// First returns the indexed value when it exists and passes the predicate.
func (q UniqueIndexQuery[K, V, IK]) First(ctx context.Context) (V, bool, error) {
	entry, ok, err := q.FirstEntry(ctx)
	if err != nil || !ok {
		var zero V
		return zero, ok, err
	}
	return entry.Value, true, nil
}

// EntryList returns a collectionx list containing zero or one indexed entry.
func (q UniqueIndexQuery[K, V, IK]) EntryList(ctx context.Context) (*collectionlist.List[Entry[K, V]], error) {
	entry, ok, err := q.FirstEntry(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return collectionlist.NewList[Entry[K, V]](), nil
	}
	return collectionlist.NewList(entry), nil
}

// ValueList returns a collectionx list containing zero or one indexed value.
func (q UniqueIndexQuery[K, V, IK]) ValueList(ctx context.Context) (*collectionlist.List[V], error) {
	value, ok, err := q.First(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return collectionlist.NewList[V](), nil
	}
	return collectionlist.NewList(value), nil
}

// Count returns 0 or 1 for a unique-index query.
func (q UniqueIndexQuery[K, V, IK]) Count(ctx context.Context) (int, error) {
	if q.predicate == nil {
		return q.index.CountByIndex(ctx, q.key)
	}
	_, ok, err := q.FirstEntry(ctx)
	if err != nil || !ok {
		return 0, err
	}
	return 1, nil
}

// Delete removes the matched model, respecting the predicate when present.
func (q UniqueIndexQuery[K, V, IK]) Delete(ctx context.Context) error {
	if q.predicate == nil {
		return q.index.Delete(ctx, q.store, q.key)
	}
	entry, ok, err := q.FirstEntry(ctx)
	if err != nil || !ok {
		return err
	}
	return q.store.Delete(ctx, entry.Key)
}

// Where filters loaded entries in memory after the indexed lookup.
func (q ManyIndexQuery[K, V, IK]) Where(predicate func(V) bool) ManyIndexQuery[K, V, IK] {
	if predicate == nil {
		q.predicate = nil
		return q
	}
	q.predicate = func(entry Entry[K, V]) bool {
		return predicate(entry.Value)
	}
	return q
}

// WhereEntry filters loaded entries in memory after the indexed lookup.
func (q ManyIndexQuery[K, V, IK]) WhereEntry(predicate func(Entry[K, V]) bool) ManyIndexQuery[K, V, IK] {
	q.predicate = predicate
	return q
}

// Limit caps the number of loaded entries. Values <= 0 mean no limit.
func (q ManyIndexQuery[K, V, IK]) Limit(limit int) ManyIndexQuery[K, V, IK] {
	q.limit = limit
	return q
}

// Entries returns indexed entries that pass the query filters.
func (q ManyIndexQuery[K, V, IK]) Entries(ctx context.Context) ([]Entry[K, V], error) {
	return q.index.FindEntriesByIndex(ctx, q.store, q.key, q.predicate, q.limit)
}

// EntryList returns indexed entries as a collectionx list.
func (q ManyIndexQuery[K, V, IK]) EntryList(ctx context.Context) (*collectionlist.List[Entry[K, V]], error) {
	entries, err := q.Entries(ctx)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(entries...), nil
}

// Find returns indexed values that pass the query filters.
func (q ManyIndexQuery[K, V, IK]) Find(ctx context.Context) ([]V, error) {
	entries, err := q.Entries(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]V, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Value)
	}
	return values, nil
}

// ValueList returns indexed values as a collectionx list.
func (q ManyIndexQuery[K, V, IK]) ValueList(ctx context.Context) (*collectionlist.List[V], error) {
	values, err := q.Find(ctx)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(values...), nil
}

// FirstEntry returns the first indexed entry that passes the query filters.
func (q ManyIndexQuery[K, V, IK]) FirstEntry(ctx context.Context) (Entry[K, V], bool, error) {
	entries, err := q.Limit(1).Entries(ctx)
	if err != nil || len(entries) == 0 {
		return Entry[K, V]{}, false, err
	}
	return entries[0], true, nil
}

// First returns the first indexed value that passes the query filters.
func (q ManyIndexQuery[K, V, IK]) First(ctx context.Context) (V, bool, error) {
	entry, ok, err := q.FirstEntry(ctx)
	if err != nil || !ok {
		var zero V
		return zero, ok, err
	}
	return entry.Value, true, nil
}

// Count returns the number of matched entries. Without filters it counts only
// index keys and does not load model values.
func (q ManyIndexQuery[K, V, IK]) Count(ctx context.Context) (int, error) {
	if q.predicate == nil {
		return q.index.CountByIndex(ctx, q.key)
	}
	entries, err := q.index.FindEntriesByIndex(ctx, q.store, q.key, q.predicate, 0)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Delete removes all matched models, respecting query filters when present.
func (q ManyIndexQuery[K, V, IK]) Delete(ctx context.Context) error {
	if q.predicate == nil {
		return q.index.Delete(ctx, q.store, q.key)
	}
	entries, err := q.index.FindEntriesByIndex(ctx, q.store, q.key, q.predicate, 0)
	if err != nil {
		return err
	}
	keys := make([]K, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return q.store.DeleteMany(ctx, keys...)
}

// Where filters loaded entries in memory after the indexed lookup.
func (q OrderedIndexQuery[K, V, IK, SK]) Where(predicate func(V) bool) OrderedIndexQuery[K, V, IK, SK] {
	if predicate == nil {
		q.predicate = nil
		return q
	}
	q.predicate = func(entry Entry[K, V]) bool {
		return predicate(entry.Value)
	}
	return q
}

// WhereEntry filters loaded entries in memory after the indexed lookup.
func (q OrderedIndexQuery[K, V, IK, SK]) WhereEntry(predicate func(Entry[K, V]) bool) OrderedIndexQuery[K, V, IK, SK] {
	q.predicate = predicate
	return q
}

// Limit caps the number of loaded entries. Values <= 0 mean no limit.
func (q OrderedIndexQuery[K, V, IK, SK]) Limit(limit int) OrderedIndexQuery[K, V, IK, SK] {
	q.limit = limit
	return q
}

// Reverse reads the ordered index in descending order.
func (q OrderedIndexQuery[K, V, IK, SK]) Reverse() OrderedIndexQuery[K, V, IK, SK] {
	q.reverse = true
	return q
}

// Entries returns indexed entries that pass the query filters.
func (q OrderedIndexQuery[K, V, IK, SK]) Entries(ctx context.Context) ([]Entry[K, V], error) {
	return q.index.FindEntriesByIndex(ctx, q.store, q.key, q.predicate, q.limit, q.reverse)
}

// EntryList returns indexed entries as a collectionx list.
func (q OrderedIndexQuery[K, V, IK, SK]) EntryList(ctx context.Context) (*collectionlist.List[Entry[K, V]], error) {
	entries, err := q.Entries(ctx)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(entries...), nil
}

// Find returns indexed values that pass the query filters.
func (q OrderedIndexQuery[K, V, IK, SK]) Find(ctx context.Context) ([]V, error) {
	entries, err := q.Entries(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]V, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Value)
	}
	return values, nil
}

// ValueList returns indexed values as a collectionx list.
func (q OrderedIndexQuery[K, V, IK, SK]) ValueList(ctx context.Context) (*collectionlist.List[V], error) {
	values, err := q.Find(ctx)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(values...), nil
}

// FirstEntry returns the first indexed entry that passes the query filters.
func (q OrderedIndexQuery[K, V, IK, SK]) FirstEntry(ctx context.Context) (Entry[K, V], bool, error) {
	entries, err := q.Limit(1).Entries(ctx)
	if err != nil || len(entries) == 0 {
		return Entry[K, V]{}, false, err
	}
	return entries[0], true, nil
}

// First returns the first indexed value that passes the query filters.
func (q OrderedIndexQuery[K, V, IK, SK]) First(ctx context.Context) (V, bool, error) {
	entry, ok, err := q.FirstEntry(ctx)
	if err != nil || !ok {
		var zero V
		return zero, ok, err
	}
	return entry.Value, true, nil
}

// Count returns the number of matched entries. Without filters it counts only
// index keys and does not load model values.
func (q OrderedIndexQuery[K, V, IK, SK]) Count(ctx context.Context) (int, error) {
	if q.predicate == nil {
		return q.index.CountByIndex(ctx, q.key)
	}
	entries, err := q.index.FindEntriesByIndex(ctx, q.store, q.key, q.predicate, 0, q.reverse)
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Page returns an ordered index page. A predicate, when set, filters only the
// loaded page entries; the cursor still follows raw index order.
func (q OrderedIndexQuery[K, V, IK, SK]) Page(ctx context.Context, cursor string, limit int) (OrderedPageResult[K, V], error) {
	page, err := q.index.Page(ctx, q.store, q.key, cursor, limit, q.reverse)
	if err != nil || q.predicate == nil {
		return page, err
	}
	filtered := page.Entries[:0]
	for _, entry := range page.Entries {
		if q.predicate(entry) {
			filtered = append(filtered, entry)
		}
	}
	page.Entries = filtered
	return page, nil
}

// Delete removes all matched models, respecting query filters when present.
func (q OrderedIndexQuery[K, V, IK, SK]) Delete(ctx context.Context) error {
	if q.predicate == nil {
		return q.index.Delete(ctx, q.store, q.key)
	}
	entries, err := q.index.FindEntriesByIndex(ctx, q.store, q.key, q.predicate, 0, q.reverse)
	if err != nil {
		return err
	}
	keys := make([]K, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	return q.store.DeleteMany(ctx, keys...)
}
