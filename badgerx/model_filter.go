package badgerx

import "context"

// Match loads one unique-index value and applies predicate when present.
func (i *SecondaryIndex[K, V, IK]) Match(ctx context.Context, store *ModelStore[K, V], key IK, predicate func(V) bool) (V, bool, error) {
	value, ok, err := i.Load(ctx, store, key)
	if err != nil || !ok {
		var zero V
		return zero, ok, err
	}
	if predicate != nil && !predicate(value) {
		var zero V
		return zero, false, nil
	}
	return value, true, nil
}

// FindByIndex loads non-unique indexed values and filters them in memory.
func (i *SecondaryIndexMany[K, V, IK]) FindByIndex(ctx context.Context, store *ModelStore[K, V], key IK, predicate func(V) bool, limit int) ([]V, error) {
	entries, err := i.FindEntriesByIndex(ctx, store, key, func(entry Entry[K, V]) bool {
		if predicate == nil {
			return true
		}
		return predicate(entry.Value)
	}, limit)
	if err != nil {
		return nil, err
	}
	values := make([]V, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Value)
	}
	return values, nil
}

// FindEntriesByIndex loads non-unique indexed entries and filters them in memory.
func (i *SecondaryIndexMany[K, V, IK]) FindEntriesByIndex(ctx context.Context, store *ModelStore[K, V], key IK, predicate func(Entry[K, V]) bool, limit int) ([]Entry[K, V], error) {
	primaries, err := i.ListPrimaries(ctx, key)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry[K, V], 0, len(primaries))
	for _, primary := range primaries {
		value, ok, err := store.Get(ctx, primary)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		entry := Entry[K, V]{Key: primary, Value: value}
		if predicate != nil && !predicate(entry) {
			continue
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	return entries, nil
}

// FindByIndex loads ordered indexed values and filters them in memory.
func (i *SecondaryIndexOrdered[K, V, IK, SK]) FindByIndex(ctx context.Context, store *ModelStore[K, V], key IK, predicate func(V) bool, limit int, reverse bool) ([]V, error) {
	entries, err := i.FindEntriesByIndex(ctx, store, key, func(entry Entry[K, V]) bool {
		if predicate == nil {
			return true
		}
		return predicate(entry.Value)
	}, limit, reverse)
	if err != nil {
		return nil, err
	}
	values := make([]V, 0, len(entries))
	for _, entry := range entries {
		values = append(values, entry.Value)
	}
	return values, nil
}

// FindEntriesByIndex loads ordered indexed entries and filters them in memory.
func (i *SecondaryIndexOrdered[K, V, IK, SK]) FindEntriesByIndex(ctx context.Context, store *ModelStore[K, V], key IK, predicate func(Entry[K, V]) bool, limit int, reverse bool) ([]Entry[K, V], error) {
	primaries, err := i.ListPrimaries(ctx, key, reverse)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry[K, V], 0, len(primaries))
	for _, primary := range primaries {
		value, ok, err := store.Get(ctx, primary)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		entry := Entry[K, V]{Key: primary, Value: value}
		if predicate != nil && !predicate(entry) {
			continue
		}
		entries = append(entries, entry)
		if limit > 0 && len(entries) >= limit {
			break
		}
	}
	return entries, nil
}
