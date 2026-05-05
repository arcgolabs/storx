package badgerx

import (
	"context"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

// EntryList returns typed key/value pairs as a collectionx list.
func (n *Namespace[K, V]) EntryList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[Entry[K, V]], error) {
	entries, err := n.List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(entries...), nil
}

// KeyList returns typed keys as a collectionx list.
func (n *Namespace[K, V]) KeyList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[K], error) {
	keys, err := n.Keys(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(keys...), nil
}

// ValueList returns typed values as a collectionx list.
func (n *Namespace[K, V]) ValueList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[V], error) {
	values, err := n.Values(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(values...), nil
}

// EntryList returns repository entries as a collectionx list.
func (r *Repository[K, V]) EntryList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[Entry[K, V]], error) {
	return r.Namespace().EntryList(ctx, opts...)
}

// KeyList returns repository keys as a collectionx list.
func (r *Repository[K, V]) KeyList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[K], error) {
	return r.Namespace().KeyList(ctx, opts...)
}

// ValueList returns repository values as a collectionx list.
func (r *Repository[K, V]) ValueList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[V], error) {
	return r.Namespace().ValueList(ctx, opts...)
}

// EntryList returns model store entries as a collectionx list.
func (s *ModelStore[K, V]) EntryList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[Entry[K, V]], error) {
	entries, err := s.List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(entries...), nil
}

// ValueList returns model store values as a collectionx list.
func (s *ModelStore[K, V]) ValueList(ctx context.Context, opts ...ListOption[K]) (*collectionlist.List[V], error) {
	entries, err := s.List(ctx, opts...)
	if err != nil {
		return nil, err
	}
	values := collectionlist.NewListWithCapacity[V](len(entries))
	for _, entry := range entries {
		values.Add(entry.Value)
	}
	return values, nil
}
