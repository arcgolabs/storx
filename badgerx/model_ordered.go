package badgerx

import (
	"context"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/dgraph-io/badger/v4"
)

type secondaryIndexOrderedKey[IK any, SK any, K any] struct {
	Secondary IK
	Sort      SK
	Primary   K
}

// OrderedPageResult contains one page of primaries or values from an ordered
// secondary index.
type OrderedPageResult[K any, V any] struct {
	Entries    []Entry[K, V]
	NextCursor string
	HasMore    bool
}

// SecondaryIndexOrdered stores a non-unique secondary index ordered by one
// extra sort field and then primary key.
type SecondaryIndexOrdered[K any, V any, IK any, SK any] struct {
	namespace *Namespace[secondaryIndexOrderedKey[IK, SK, K], []byte]
	keys      keycodec.Codec[IK]
	keyOf     func(value V) IK
	sortOf    func(value V) SK
}

// SecondaryIndexOrderedDefinition declares an ordered non-unique secondary
// index for a model schema.
type SecondaryIndexOrderedDefinition[K any, V any, IK any, SK any] struct {
	Prefix   string
	Keys     keycodec.Codec[IK]
	SortKeys keycodec.Codec[SK]
	KeyOf    func(value V) IK
	SortOf   func(value V) SK
	Options  []NamespaceOption
}

func (d SecondaryIndexOrderedDefinition[K, V, IK, SK]) describeIndex() SchemaIndexDescription {
	return SchemaIndexDescription{
		Name:          d.Prefix,
		Kind:          IndexKindOrdered,
		SecondaryType: typeOf[IK](),
		SortType:      typeOf[SK](),
	}
}

func (d SecondaryIndexOrderedDefinition[K, V, IK, SK]) Open(db *badger.DB, primaryKeys keycodec.Codec[K]) *SecondaryIndexOrdered[K, V, IK, SK] {
	return NewSecondaryIndexOrdered(db, d.Prefix, d.Keys, d.SortKeys, primaryKeys, d.KeyOf, d.SortOf, d.Options...)
}

func (d SecondaryIndexOrderedDefinition[K, V, IK, SK]) OpenWithDB(db *DB, primaryKeys keycodec.Codec[K]) *SecondaryIndexOrdered[K, V, IK, SK] {
	return NewSecondaryIndexOrderedWithDB(db, d.Prefix, d.Keys, d.SortKeys, primaryKeys, d.KeyOf, d.SortOf, d.Options...)
}

func (d SecondaryIndexOrderedDefinition[K, V, IK, SK]) openWithRaw(db *badger.DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V] {
	return d.Open(db, primaryKeys)
}

func (d SecondaryIndexOrderedDefinition[K, V, IK, SK]) openWithDB(db *DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V] {
	return d.OpenWithDB(db, primaryKeys)
}

func NewSecondaryIndexOrdered[K any, V any, IK any, SK any](
	db *badger.DB,
	prefix string,
	keys keycodec.Codec[IK],
	sortKeys keycodec.Codec[SK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	sortOf func(value V) SK,
	opts ...NamespaceOption,
) *SecondaryIndexOrdered[K, V, IK, SK] {
	return &SecondaryIndexOrdered[K, V, IK, SK]{
		namespace: NewNamespace(
			db,
			prefix,
			secondaryIndexOrderedCodec(keys, sortKeys, primaryKeys),
			codec.Bytes(),
			opts...,
		),
		keys:   keys,
		keyOf:  keyOf,
		sortOf: sortOf,
	}
}

func NewSecondaryIndexOrderedWithDB[K any, V any, IK any, SK any](
	db *DB,
	prefix string,
	keys keycodec.Codec[IK],
	sortKeys keycodec.Codec[SK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	sortOf func(value V) SK,
	opts ...NamespaceOption,
) *SecondaryIndexOrdered[K, V, IK, SK] {
	return &SecondaryIndexOrdered[K, V, IK, SK]{
		namespace: NewNamespaceWithDB(
			db,
			prefix,
			secondaryIndexOrderedCodec(keys, sortKeys, primaryKeys),
			codec.Bytes(),
			opts...,
		),
		keys:   keys,
		keyOf:  keyOf,
		sortOf: sortOf,
	}
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) Namespace() *Namespace[secondaryIndexOrderedKey[IK, SK, K], []byte] {
	if i == nil {
		return nil
	}
	return i.namespace
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) ListPrimaries(ctx context.Context, key IK, reverse bool) ([]K, error) {
	prefix, err := keycodec.ComponentPrefix(i.keys, key)
	if err != nil {
		return nil, err
	}
	entries, err := i.Namespace().List(
		ctx,
		WithPrefix[secondaryIndexOrderedKey[IK, SK, K]](prefix),
		WithReverse[secondaryIndexOrderedKey[IK, SK, K]](reverse),
	)
	if err != nil {
		return nil, err
	}
	primaries := make([]K, 0, len(entries))
	for _, entry := range entries {
		primaries = append(primaries, entry.Key.Primary)
	}
	return primaries, nil
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) CountByIndex(ctx context.Context, key IK) (int, error) {
	primaries, err := i.ListPrimaries(ctx, key, false)
	if err != nil {
		return 0, err
	}
	return len(primaries), nil
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) List(ctx context.Context, store *ModelStore[K, V], key IK, reverse bool) ([]V, error) {
	primaries, err := i.ListPrimaries(ctx, key, reverse)
	if err != nil {
		return nil, err
	}
	values := make([]V, 0, len(primaries))
	for _, primary := range primaries {
		value, ok, err := store.Get(ctx, primary)
		if err != nil {
			return nil, err
		}
		if ok {
			values = append(values, value)
		}
	}
	return values, nil
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) PagePrimaries(ctx context.Context, key IK, cursor string, limit int, reverse bool) (OrderedPageResult[K, K], error) {
	prefix, err := keycodec.ComponentPrefix(i.keys, key)
	if err != nil {
		return OrderedPageResult[K, K]{}, err
	}
	page, err := i.Namespace().Page(
		ctx,
		cursor,
		WithPrefix[secondaryIndexOrderedKey[IK, SK, K]](prefix),
		WithLimit[secondaryIndexOrderedKey[IK, SK, K]](limit),
		WithReverse[secondaryIndexOrderedKey[IK, SK, K]](reverse),
	)
	if err != nil {
		return OrderedPageResult[K, K]{}, err
	}
	result := OrderedPageResult[K, K]{
		Entries:    make([]Entry[K, K], 0, len(page.Entries)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	for _, entry := range page.Entries {
		result.Entries = append(result.Entries, Entry[K, K]{
			Key:   entry.Key.Primary,
			Value: entry.Key.Primary,
		})
	}
	return result, nil
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) Page(ctx context.Context, store *ModelStore[K, V], key IK, cursor string, limit int, reverse bool) (OrderedPageResult[K, V], error) {
	prefix, err := keycodec.ComponentPrefix(i.keys, key)
	if err != nil {
		return OrderedPageResult[K, V]{}, err
	}
	page, err := i.Namespace().Page(
		ctx,
		cursor,
		WithPrefix[secondaryIndexOrderedKey[IK, SK, K]](prefix),
		WithLimit[secondaryIndexOrderedKey[IK, SK, K]](limit),
		WithReverse[secondaryIndexOrderedKey[IK, SK, K]](reverse),
	)
	if err != nil {
		return OrderedPageResult[K, V]{}, err
	}

	result := OrderedPageResult[K, V]{
		Entries:    make([]Entry[K, V], 0, len(page.Entries)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	for _, entry := range page.Entries {
		value, ok, err := store.Get(ctx, entry.Key.Primary)
		if err != nil {
			return OrderedPageResult[K, V]{}, err
		}
		if ok {
			result.Entries = append(result.Entries, Entry[K, V]{
				Key:   entry.Key.Primary,
				Value: value,
			})
		}
	}
	return result, nil
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) Delete(ctx context.Context, store *ModelStore[K, V], key IK) error {
	primaries, err := i.ListPrimaries(ctx, key, false)
	if err != nil {
		return err
	}
	return store.DeleteMany(ctx, primaries...)
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) DeleteByIndex(ctx context.Context, store *ModelStore[K, V], key IK) error {
	return i.Delete(ctx, store, key)
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) putIndex(ctx context.Context, txn *badger.Txn, primary K, value V, opts ...SetOption) error {
	if i == nil || i.namespace == nil || i.keyOf == nil || i.sortOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update := &updateTx[secondaryIndexOrderedKey[IK, SK, K], []byte]{
		viewTx: viewTx[secondaryIndexOrderedKey[IK, SK, K], []byte]{
			namespace: i.namespace,
			txn:       txn,
			ctx:       ctx,
		},
	}
	return update.Set(secondaryIndexOrderedKey[IK, SK, K]{
		Secondary: i.keyOf(value),
		Sort:      i.sortOf(value),
		Primary:   primary,
	}, []byte{1}, opts...)
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) deleteIndex(ctx context.Context, txn *badger.Txn, primary K, value V) error {
	if i == nil || i.namespace == nil || i.keyOf == nil || i.sortOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update := &updateTx[secondaryIndexOrderedKey[IK, SK, K], []byte]{
		viewTx: viewTx[secondaryIndexOrderedKey[IK, SK, K], []byte]{
			namespace: i.namespace,
			txn:       txn,
			ctx:       ctx,
		},
	}
	return update.Delete(secondaryIndexOrderedKey[IK, SK, K]{
		Secondary: i.keyOf(value),
		Sort:      i.sortOf(value),
		Primary:   primary,
	})
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) resetIndex(ctx context.Context, txn *badger.Txn) error {
	return resetRawNamespace(txn, i.namespace.prefix)
}

func (i *SecondaryIndexOrdered[K, V, IK, SK]) bootstrapIndex(ctx context.Context, txn *badger.Txn) error {
	_ = ctx
	_ = txn
	return nil
}

func secondaryIndexOrderedCodec[IK any, SK any, K any](
	keys keycodec.Codec[IK],
	sortKeys keycodec.Codec[SK],
	primaryKeys keycodec.Codec[K],
) keycodec.Codec[secondaryIndexOrderedKey[IK, SK, K]] {
	return keycodec.Composite(
		keycodec.Field(
			keys,
			func(value secondaryIndexOrderedKey[IK, SK, K]) IK { return value.Secondary },
			func(target *secondaryIndexOrderedKey[IK, SK, K], field IK) { target.Secondary = field },
		),
		keycodec.Field(
			sortKeys,
			func(value secondaryIndexOrderedKey[IK, SK, K]) SK { return value.Sort },
			func(target *secondaryIndexOrderedKey[IK, SK, K], field SK) { target.Sort = field },
		),
		keycodec.Field(
			primaryKeys,
			func(value secondaryIndexOrderedKey[IK, SK, K]) K { return value.Primary },
			func(target *secondaryIndexOrderedKey[IK, SK, K], field K) { target.Primary = field },
		),
	)
}
