package bboltx

import (
	"context"

	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/storx/keycodec"
)

// RelatedOne stores the preloaded to-one relation for one source value.
type RelatedOne[S any, IK any, V any] struct {
	Source S
	Key    IK
	Value  V
	Found  bool
}

// RelatedMany stores the preloaded to-many relation for one source value.
type RelatedMany[S any, IK any, V any] struct {
	Source S
	Key    IK
	Values []V
}

// ToOneRelation resolves one unique related model from one source value.
type ToOneRelation[S any, K any, V any, IK any] struct {
	Name  string
	Kind  RelationKind
	KeyOf func(source S) IK
	Index *SecondaryIndex[K, V, IK]
	Store *ModelStore[K, V]
}

// ToManyRelation resolves many related models from one source value.
type ToManyRelation[S any, K any, V any, IK any] struct {
	Name  string
	Kind  RelationKind
	KeyOf func(source S) IK
	Index *SecondaryIndexMany[K, V, IK]
	Store *ModelStore[K, V]
}

// OrderedToManyRelation resolves many ordered related models from one source value.
type OrderedToManyRelation[S any, K any, V any, IK any, SK any] struct {
	Name    string
	Kind    RelationKind
	KeyOf   func(source S) IK
	Index   *SecondaryIndexOrdered[K, V, IK, SK]
	Store   *ModelStore[K, V]
	Reverse bool
}

func NewBelongsToRelation[S any, K any, V any, IK any](
	name string,
	keyOf func(source S) IK,
	index *SecondaryIndex[K, V, IK],
	store *ModelStore[K, V],
) *ToOneRelation[S, K, V, IK] {
	return &ToOneRelation[S, K, V, IK]{
		Name:  name,
		Kind:  RelationKindBelongsTo,
		KeyOf: keyOf,
		Index: index,
		Store: store,
	}
}

func NewHasOneRelation[S any, K any, V any, IK any](
	name string,
	keyOf func(source S) IK,
	index *SecondaryIndex[K, V, IK],
	store *ModelStore[K, V],
) *ToOneRelation[S, K, V, IK] {
	return &ToOneRelation[S, K, V, IK]{
		Name:  name,
		Kind:  RelationKindHasOne,
		KeyOf: keyOf,
		Index: index,
		Store: store,
	}
}

func NewHasManyRelation[S any, K any, V any, IK any](
	name string,
	keyOf func(source S) IK,
	index *SecondaryIndexMany[K, V, IK],
	store *ModelStore[K, V],
) *ToManyRelation[S, K, V, IK] {
	return &ToManyRelation[S, K, V, IK]{
		Name:  name,
		Kind:  RelationKindHasMany,
		KeyOf: keyOf,
		Index: index,
		Store: store,
	}
}

func NewOrderedHasManyRelation[S any, K any, V any, IK any, SK any](
	name string,
	keyOf func(source S) IK,
	index *SecondaryIndexOrdered[K, V, IK, SK],
	store *ModelStore[K, V],
	reverse bool,
) *OrderedToManyRelation[S, K, V, IK, SK] {
	return &OrderedToManyRelation[S, K, V, IK, SK]{
		Name:    name,
		Kind:    RelationKindHasMany,
		KeyOf:   keyOf,
		Index:   index,
		Store:   store,
		Reverse: reverse,
	}
}

func (r *ToOneRelation[S, K, V, IK]) LoadRelated(ctx context.Context, source S) (V, bool, error) {
	return r.Index.Load(ctx, r.Store, r.KeyOf(source))
}

func (r *ToOneRelation[S, K, V, IK]) Preload(ctx context.Context, sources ...S) ([]RelatedOne[S, IK, V], error) {
	results := make([]RelatedOne[S, IK, V], len(sources))
	if r == nil || r.Index == nil || r.Store == nil || r.KeyOf == nil {
		return results, nil
	}

	keys, encodedKeys, indexByEncoded, err := collectUniqueRelationKeys(sources, r.KeyOf, r.Index.bucket.keys)
	if err != nil {
		return nil, err
	}

	lookups, err := r.Index.bucket.GetMany(ctx, keys...)
	if err != nil {
		return nil, err
	}

	primaryKeys := make([]K, 0, len(lookups))
	encodedPrimaryToRelation := make(map[string]string, len(lookups))
	for idx, lookup := range lookups {
		if !lookup.Found {
			continue
		}
		primaryKeys = append(primaryKeys, lookup.Value)
		encodedPrimary, err := r.Store.Repository().Bucket().encodeKey("relation_preload", lookup.Value)
		if err != nil {
			return nil, err
		}
		encodedPrimaryToRelation[string(encodedPrimary)] = indexByEncoded[encodedKeys[idx]]
	}

	valuesByRelation, err := lookupRelationValues(ctx, r.Store, primaryKeys, encodedPrimaryToRelation)
	if err != nil {
		return nil, err
	}

	for idx, source := range sources {
		key := r.KeyOf(source)
		encodedKey, err := r.Index.bucket.encodeKey("relation_preload", key)
		if err != nil {
			return nil, err
		}
		value, ok := valuesByRelation[string(encodedKey)]
		results[idx] = RelatedOne[S, IK, V]{
			Source: source,
			Key:    key,
			Value:  value,
			Found:  ok,
		}
	}
	return results, nil
}

func (r *ToManyRelation[S, K, V, IK]) LoadRelated(ctx context.Context, source S) ([]V, error) {
	return r.Index.List(ctx, r.Store, r.KeyOf(source))
}

func (r *ToManyRelation[S, K, V, IK]) Preload(ctx context.Context, sources ...S) ([]RelatedMany[S, IK, V], error) {
	results := make([]RelatedMany[S, IK, V], len(sources))
	if r == nil || r.Index == nil || r.Store == nil || r.KeyOf == nil {
		return results, nil
	}

	keyOrder, keyValues, _, err := collectUniqueRelationKeys(sources, r.KeyOf, r.Index.keys)
	if err != nil {
		return nil, err
	}
	primariesByRelation, orderedPrimaryKeys, err := collectManyRelationPrimaries(ctx, keyOrder, keyValues, r.Index.ListPrimaries, r.Store.Repository().Bucket())
	if err != nil {
		return nil, err
	}
	valuesByPrimary, err := lookupPrimaryValues(ctx, r.Store, orderedPrimaryKeys)
	if err != nil {
		return nil, err
	}

	for idx, source := range sources {
		key := r.KeyOf(source)
		encodedKey, err := r.Index.keys.EncodeKey(key)
		if err != nil {
			return nil, err
		}
		var values []V
		for _, encodedPrimary := range primariesByRelation[string(encodedKey)] {
			if value, ok := valuesByPrimary[encodedPrimary]; ok {
				values = append(values, value)
			}
		}
		results[idx] = RelatedMany[S, IK, V]{
			Source: source,
			Key:    key,
			Values: values,
		}
	}
	return results, nil
}

func (r *OrderedToManyRelation[S, K, V, IK, SK]) LoadRelated(ctx context.Context, source S) ([]V, error) {
	return r.Index.List(ctx, r.Store, r.KeyOf(source), r.Reverse)
}

func (r *OrderedToManyRelation[S, K, V, IK, SK]) Preload(ctx context.Context, sources ...S) ([]RelatedMany[S, IK, V], error) {
	results := make([]RelatedMany[S, IK, V], len(sources))
	if r == nil || r.Index == nil || r.Store == nil || r.KeyOf == nil {
		return results, nil
	}

	keyOrder, keyValues, _, err := collectUniqueRelationKeys(sources, r.KeyOf, r.Index.keys)
	if err != nil {
		return nil, err
	}
	listPrimaries := func(ctx context.Context, key IK) ([]K, error) {
		return r.Index.ListPrimaries(ctx, key, r.Reverse)
	}
	primariesByRelation, orderedPrimaryKeys, err := collectManyRelationPrimaries(ctx, keyOrder, keyValues, listPrimaries, r.Store.Repository().Bucket())
	if err != nil {
		return nil, err
	}
	valuesByPrimary, err := lookupPrimaryValues(ctx, r.Store, orderedPrimaryKeys)
	if err != nil {
		return nil, err
	}

	for idx, source := range sources {
		key := r.KeyOf(source)
		encodedKey, err := r.Index.keys.EncodeKey(key)
		if err != nil {
			return nil, err
		}
		var values []V
		for _, encodedPrimary := range primariesByRelation[string(encodedKey)] {
			if value, ok := valuesByPrimary[encodedPrimary]; ok {
				values = append(values, value)
			}
		}
		results[idx] = RelatedMany[S, IK, V]{
			Source: source,
			Key:    key,
			Values: values,
		}
	}
	return results, nil
}

func collectUniqueRelationKeys[S any, IK any](sources []S, keyOf func(S) IK, keys keycodec.Codec[IK]) ([]IK, []string, map[string]string, error) {
	orderedEncodedKeys := collectionset.NewOrderedSetWithCapacity[string](len(sources))
	keyByEncoded := make(map[string]IK, len(sources))
	indexByEncoded := make(map[string]string, len(sources))
	for _, source := range sources {
		key := keyOf(source)
		encodedKey, err := keys.EncodeKey(key)
		if err != nil {
			return nil, nil, nil, err
		}
		encodedString := string(encodedKey)
		if orderedEncodedKeys.Contains(encodedString) {
			continue
		}
		orderedEncodedKeys.Add(encodedString)
		keyByEncoded[encodedString] = key
		indexByEncoded[encodedString] = encodedString
	}
	encodedKeys := orderedEncodedKeys.Values()
	uniqueKeys := make([]IK, 0, len(encodedKeys))
	for _, encodedKey := range encodedKeys {
		uniqueKeys = append(uniqueKeys, keyByEncoded[encodedKey])
	}
	return uniqueKeys, encodedKeys, indexByEncoded, nil
}

func collectManyRelationPrimaries[IK any, K any, V any](
	ctx context.Context,
	keyOrder []IK,
	keyValues []string,
	listPrimaries func(context.Context, IK) ([]K, error),
	bucket *Bucket[K, V],
) (map[string][]string, []K, error) {
	primariesByRelation := make(map[string][]string, len(keyOrder))
	orderedEncodedPrimaries := collectionset.NewOrderedSet[string]()
	primaryByEncoded := make(map[string]K)

	for idx, key := range keyOrder {
		primaries, err := listPrimaries(ctx, key)
		if err != nil {
			return nil, nil, err
		}
		encodedRelation := keyValues[idx]
		encodedPrimaries := make([]string, 0, len(primaries))
		for _, primary := range primaries {
			encodedPrimary, err := bucket.encodeKey("relation_preload", primary)
			if err != nil {
				return nil, nil, err
			}
			encodedString := string(encodedPrimary)
			encodedPrimaries = append(encodedPrimaries, encodedString)
			if orderedEncodedPrimaries.Contains(encodedString) {
				continue
			}
			orderedEncodedPrimaries.Add(encodedString)
			primaryByEncoded[encodedString] = primary
		}
		primariesByRelation[encodedRelation] = encodedPrimaries
	}
	orderedPrimaryKeys := make([]K, 0, orderedEncodedPrimaries.Len())
	for _, encodedPrimary := range orderedEncodedPrimaries.Values() {
		orderedPrimaryKeys = append(orderedPrimaryKeys, primaryByEncoded[encodedPrimary])
	}
	return primariesByRelation, orderedPrimaryKeys, nil
}

func lookupPrimaryValues[K any, V any](ctx context.Context, store *ModelStore[K, V], primaryKeys []K) (map[string]V, error) {
	valuesByPrimary := make(map[string]V, len(primaryKeys))
	if len(primaryKeys) == 0 {
		return valuesByPrimary, nil
	}
	lookups, err := store.Repository().GetMany(ctx, primaryKeys...)
	if err != nil {
		return nil, err
	}
	for _, lookup := range lookups {
		if !lookup.Found {
			continue
		}
		encodedPrimary, err := store.Repository().Bucket().encodeKey("relation_preload", lookup.Key)
		if err != nil {
			return nil, err
		}
		valuesByPrimary[string(encodedPrimary)] = lookup.Value
	}
	return valuesByPrimary, nil
}

func lookupRelationValues[K any, V any](ctx context.Context, store *ModelStore[K, V], primaryKeys []K, relationByPrimary map[string]string) (map[string]V, error) {
	valuesByPrimary, err := lookupPrimaryValues(ctx, store, primaryKeys)
	if err != nil {
		return nil, err
	}
	valuesByRelation := make(map[string]V, len(valuesByPrimary))
	for encodedPrimary, value := range valuesByPrimary {
		encodedRelation, ok := relationByPrimary[encodedPrimary]
		if ok {
			valuesByRelation[encodedRelation] = value
		}
	}
	return valuesByRelation, nil
}
