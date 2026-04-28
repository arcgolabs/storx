package badgerx

import (
	"context"
	"errors"
	"reflect"
	"time"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/dgraph-io/badger/v4"
)

type modelWriteMode string

const (
	modelWriteModeCreate modelWriteMode = "create"
	modelWriteModeUpdate modelWriteMode = "update"
	modelWriteModeUpsert modelWriteMode = "upsert"
)

// ModelOperation identifies the model lifecycle operation being executed.
type ModelOperation string

const (
	ModelOperationCreate ModelOperation = "create"
	ModelOperationUpdate ModelOperation = "update"
	ModelOperationUpsert ModelOperation = "upsert"
	ModelOperationDelete ModelOperation = "delete"
)

// ModelHookEvent is emitted around model lifecycle operations.
type ModelHookEvent[K any, V any] struct {
	Operation ModelOperation
	Key       K
	Value     *V
	Previous  *V
	Exists    bool
}

// ModelHooks defines optional lifecycle callbacks for one Badger model store.
type ModelHooks[K any, V any] struct {
	BeforeWrite  func(ctx context.Context, event *ModelHookEvent[K, V]) error
	AfterWrite   func(ctx context.Context, event ModelHookEvent[K, V])
	BeforeDelete func(ctx context.Context, event *ModelHookEvent[K, V]) error
	AfterDelete  func(ctx context.Context, event ModelHookEvent[K, V])
}

type badgerModelIndex[K any, V any] interface {
	putIndex(ctx context.Context, txn *badger.Txn, primary K, value V, opts ...SetOption) error
	deleteIndex(ctx context.Context, txn *badger.Txn, primary K, value V) error
	resetIndex(ctx context.Context, txn *badger.Txn) error
}

// ModelIndexDefinition opens a model index from a raw DB or DB wrapper.
type ModelIndexDefinition[K any, V any] interface {
	openWithRaw(db *badger.DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V]
	openWithDB(db *DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V]
}

type modelStoreOptions[K any, V any] struct {
	indexes           []badgerModelIndex[K, V]
	defaultSetOptions func(value V) []SetOption
	hooks             []ModelHooks[K, V]
}

// ModelStoreOption configures a Badger model store.
type ModelStoreOption[K any, V any] func(*modelStoreOptions[K, V])

// WithModelIndex registers a secondary index for a model store.
func WithModelIndex[K any, V any](index badgerModelIndex[K, V]) ModelStoreOption[K, V] {
	return func(opts *modelStoreOptions[K, V]) {
		if index != nil {
			opts.indexes = append(opts.indexes, index)
		}
	}
}

// WithDefaultSetOptions derives default Badger set options from a model value.
func WithDefaultSetOptions[K any, V any](fn func(value V) []SetOption) ModelStoreOption[K, V] {
	return func(opts *modelStoreOptions[K, V]) {
		opts.defaultSetOptions = fn
	}
}

// WithModelHooks registers lifecycle hooks for a model store.
func WithModelHooks[K any, V any](hooks ...ModelHooks[K, V]) ModelStoreOption[K, V] {
	return func(opts *modelStoreOptions[K, V]) {
		for _, hook := range hooks {
			opts.hooks = append(opts.hooks, hook)
		}
	}
}

// SecondaryIndex stores a unique secondary-key to primary-key mapping.
type SecondaryIndex[K any, V any, IK any] struct {
	namespace *Namespace[IK, K]
	keyOf     func(value V) IK
}

type secondaryIndexManyKey[IK any, K any] struct {
	Secondary IK
	Primary   K
}

// SecondaryIndexMany stores a non-unique secondary-key to primary-key mapping.
type SecondaryIndexMany[K any, V any, IK any] struct {
	namespace *Namespace[secondaryIndexManyKey[IK, K], []byte]
	keys      keycodec.Codec[IK]
	keyOf     func(value V) IK
}

// SecondaryIndexDefinition declares a named secondary index for a model
// schema.
type SecondaryIndexDefinition[K any, V any, IK any] struct {
	Prefix  string
	Keys    keycodec.Codec[IK]
	KeyOf   func(value V) IK
	Options []NamespaceOption
}

// SecondaryIndexManyDefinition declares a named non-unique secondary index for
// a model schema.
type SecondaryIndexManyDefinition[K any, V any, IK any] struct {
	Prefix  string
	Keys    keycodec.Codec[IK]
	KeyOf   func(value V) IK
	Options []NamespaceOption
}

func (d SecondaryIndexDefinition[K, V, IK]) Open(db *badger.DB, primaryKeys keycodec.Codec[K]) *SecondaryIndex[K, V, IK] {
	return NewSecondaryIndex(db, d.Prefix, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexDefinition[K, V, IK]) OpenWithDB(db *DB, primaryKeys keycodec.Codec[K]) *SecondaryIndex[K, V, IK] {
	return NewSecondaryIndexWithDB(db, d.Prefix, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexDefinition[K, V, IK]) openWithRaw(db *badger.DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V] {
	return d.Open(db, primaryKeys)
}

func (d SecondaryIndexDefinition[K, V, IK]) openWithDB(db *DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V] {
	return d.OpenWithDB(db, primaryKeys)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) Open(db *badger.DB, primaryKeys keycodec.Codec[K]) *SecondaryIndexMany[K, V, IK] {
	return NewSecondaryIndexMany(db, d.Prefix, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) OpenWithDB(db *DB, primaryKeys keycodec.Codec[K]) *SecondaryIndexMany[K, V, IK] {
	return NewSecondaryIndexManyWithDB(db, d.Prefix, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) openWithRaw(db *badger.DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V] {
	return d.Open(db, primaryKeys)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) openWithDB(db *DB, primaryKeys keycodec.Codec[K]) badgerModelIndex[K, V] {
	return d.OpenWithDB(db, primaryKeys)
}

// ModelSchema declares one Badger model and its indexes.
type ModelSchema[K any, V any] struct {
	Prefix            string
	Keys              keycodec.Codec[K]
	Values            codec.Codec[V]
	KeyOf             func(value V) K
	Options           []NamespaceOption
	DefaultSetOptions func(value V) []SetOption
	Indexes           []ModelIndexDefinition[K, V]
	Hooks             []ModelHooks[K, V]
}

func (s ModelSchema[K, V]) Open(db *badger.DB) *ModelStore[K, V] {
	options := make([]ModelStoreOption[K, V], 0, len(s.Indexes)+2)
	if s.DefaultSetOptions != nil {
		options = append(options, WithDefaultSetOptions[K, V](s.DefaultSetOptions))
	}
	if len(s.Hooks) > 0 {
		options = append(options, WithModelHooks[K, V](s.Hooks...))
	}
	for _, index := range s.Indexes {
		if index != nil {
			options = append(options, WithModelIndex(index.openWithRaw(db, s.Keys)))
		}
	}
	repo := NewRepository(db, s.Prefix, s.Keys, s.Values, s.Options...)
	return NewModelStoreFromRepository(repo, s.KeyOf, options...)
}

func (s ModelSchema[K, V]) OpenWithDB(db *DB) *ModelStore[K, V] {
	options := make([]ModelStoreOption[K, V], 0, len(s.Indexes)+2)
	if s.DefaultSetOptions != nil {
		options = append(options, WithDefaultSetOptions[K, V](s.DefaultSetOptions))
	}
	if len(s.Hooks) > 0 {
		options = append(options, WithModelHooks[K, V](s.Hooks...))
	}
	for _, index := range s.Indexes {
		if index != nil {
			options = append(options, WithModelIndex(index.openWithDB(db, s.Keys)))
		}
	}
	repo := NewRepositoryWithDB(db, s.Prefix, s.Keys, s.Values, s.Options...)
	return NewModelStoreFromRepository(repo, s.KeyOf, options...)
}

func NewSecondaryIndex[K any, V any, IK any](
	db *badger.DB,
	prefix string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...NamespaceOption,
) *SecondaryIndex[K, V, IK] {
	return &SecondaryIndex[K, V, IK]{
		namespace: NewNamespace(db, prefix, keys, keyCodecValue[K]{codec: primaryKeys}, opts...),
		keyOf:     keyOf,
	}
}

func NewSecondaryIndexWithDB[K any, V any, IK any](
	db *DB,
	prefix string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...NamespaceOption,
) *SecondaryIndex[K, V, IK] {
	return &SecondaryIndex[K, V, IK]{
		namespace: NewNamespaceWithDB(db, prefix, keys, keyCodecValue[K]{codec: primaryKeys}, opts...),
		keyOf:     keyOf,
	}
}

func NewSecondaryIndexMany[K any, V any, IK any](
	db *badger.DB,
	prefix string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...NamespaceOption,
) *SecondaryIndexMany[K, V, IK] {
	return &SecondaryIndexMany[K, V, IK]{
		namespace: NewNamespace(
			db,
			prefix,
			secondaryIndexManyCodec(keys, primaryKeys),
			codec.Bytes(),
			opts...,
		),
		keys:  keys,
		keyOf: keyOf,
	}
}

func NewSecondaryIndexManyWithDB[K any, V any, IK any](
	db *DB,
	prefix string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...NamespaceOption,
) *SecondaryIndexMany[K, V, IK] {
	return &SecondaryIndexMany[K, V, IK]{
		namespace: NewNamespaceWithDB(
			db,
			prefix,
			secondaryIndexManyCodec(keys, primaryKeys),
			codec.Bytes(),
			opts...,
		),
		keys:  keys,
		keyOf: keyOf,
	}
}

func (i *SecondaryIndex[K, V, IK]) Namespace() *Namespace[IK, K] {
	if i == nil {
		return nil
	}
	return i.namespace
}

func (i *SecondaryIndex[K, V, IK]) GetPrimary(ctx context.Context, key IK) (K, bool, error) {
	return i.Namespace().Get(ctx, key)
}

func (i *SecondaryIndex[K, V, IK]) CountByIndex(ctx context.Context, key IK) (int, error) {
	_, ok, err := i.GetPrimary(ctx, key)
	if err != nil {
		return 0, err
	}
	if ok {
		return 1, nil
	}
	return 0, nil
}

func (i *SecondaryIndex[K, V, IK]) Load(ctx context.Context, store *ModelStore[K, V], key IK) (V, bool, error) {
	primary, ok, err := i.GetPrimary(ctx, key)
	if err != nil || !ok {
		var zero V
		return zero, ok, err
	}
	return store.Get(ctx, primary)
}

func (i *SecondaryIndex[K, V, IK]) Get(ctx context.Context, store *ModelStore[K, V], key IK) (V, bool, error) {
	return i.Load(ctx, store, key)
}

func (i *SecondaryIndex[K, V, IK]) GetByIndex(ctx context.Context, store *ModelStore[K, V], key IK) (V, bool, error) {
	return i.Load(ctx, store, key)
}

func (i *SecondaryIndex[K, V, IK]) Delete(ctx context.Context, store *ModelStore[K, V], key IK) error {
	primary, ok, err := i.GetPrimary(ctx, key)
	if err != nil || !ok {
		return err
	}
	return store.Delete(ctx, primary)
}

func (i *SecondaryIndex[K, V, IK]) DeleteByIndex(ctx context.Context, store *ModelStore[K, V], key IK) error {
	return i.Delete(ctx, store, key)
}

func (i *SecondaryIndexMany[K, V, IK]) Namespace() *Namespace[secondaryIndexManyKey[IK, K], []byte] {
	if i == nil {
		return nil
	}
	return i.namespace
}

func (i *SecondaryIndexMany[K, V, IK]) ListPrimaries(ctx context.Context, key IK) ([]K, error) {
	prefix, err := keycodec.ComponentPrefix(i.keys, key)
	if err != nil {
		return nil, err
	}
	iter, err := i.Namespace().Iter(ctx, WithPrefix[secondaryIndexManyKey[IK, K]](prefix))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = iter.Close()
	}()

	var primaries []K
	for {
		entry, ok, err := iter.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return primaries, nil
		}
		primaries = append(primaries, entry.Key.Primary)
	}
}

func (i *SecondaryIndexMany[K, V, IK]) CountByIndex(ctx context.Context, key IK) (int, error) {
	prefix, err := keycodec.ComponentPrefix(i.keys, key)
	if err != nil {
		return 0, err
	}
	iter, err := i.Namespace().Iter(ctx, WithPrefix[secondaryIndexManyKey[IK, K]](prefix))
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = iter.Close()
	}()

	count := 0
	for {
		_, ok, err := iter.Next()
		if err != nil {
			return 0, err
		}
		if !ok {
			return count, nil
		}
		count++
	}
}

func (i *SecondaryIndexMany[K, V, IK]) List(ctx context.Context, store *ModelStore[K, V], key IK) ([]V, error) {
	primaries, err := i.ListPrimaries(ctx, key)
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

func (i *SecondaryIndexMany[K, V, IK]) ListByIndex(ctx context.Context, store *ModelStore[K, V], key IK) ([]V, error) {
	return i.List(ctx, store, key)
}

func (i *SecondaryIndexMany[K, V, IK]) Delete(ctx context.Context, store *ModelStore[K, V], key IK) error {
	primaries, err := i.ListPrimaries(ctx, key)
	if err != nil {
		return err
	}
	return store.DeleteMany(ctx, primaries...)
}

func (i *SecondaryIndexMany[K, V, IK]) DeleteByIndex(ctx context.Context, store *ModelStore[K, V], key IK) error {
	return i.Delete(ctx, store, key)
}

func (i *SecondaryIndex[K, V, IK]) putIndex(ctx context.Context, txn *badger.Txn, primary K, value V, opts ...SetOption) error {
	if i == nil || i.namespace == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update := &updateTx[IK, K]{
		viewTx: viewTx[IK, K]{
			namespace: i.namespace,
			txn:       txn,
			ctx:       ctx,
		},
	}
	indexKey := i.keyOf(value)
	existingPrimary, ok, err := update.Get(indexKey)
	if err != nil {
		return err
	}
	if ok && !reflect.DeepEqual(existingPrimary, primary) {
		return i.namespace.wrapError("model_index_put", storx.ErrAlreadyExists, "secondary index key already exists")
	}
	return update.Set(indexKey, primary, opts...)
}

func (i *SecondaryIndex[K, V, IK]) deleteIndex(ctx context.Context, txn *badger.Txn, primary K, value V) error {
	if i == nil || i.namespace == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update := &updateTx[IK, K]{
		viewTx: viewTx[IK, K]{
			namespace: i.namespace,
			txn:       txn,
			ctx:       ctx,
		},
	}
	return update.Delete(i.keyOf(value))
}

func (i *SecondaryIndex[K, V, IK]) resetIndex(ctx context.Context, txn *badger.Txn) error {
	return resetRawNamespace(txn, i.namespace.prefix)
}

func (i *SecondaryIndexMany[K, V, IK]) putIndex(ctx context.Context, txn *badger.Txn, primary K, value V, opts ...SetOption) error {
	if i == nil || i.namespace == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update := &updateTx[secondaryIndexManyKey[IK, K], []byte]{
		viewTx: viewTx[secondaryIndexManyKey[IK, K], []byte]{
			namespace: i.namespace,
			txn:       txn,
			ctx:       ctx,
		},
	}
	return update.Set(secondaryIndexManyKey[IK, K]{
		Secondary: i.keyOf(value),
		Primary:   primary,
	}, []byte{1}, opts...)
}

func (i *SecondaryIndexMany[K, V, IK]) deleteIndex(ctx context.Context, txn *badger.Txn, primary K, value V) error {
	if i == nil || i.namespace == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update := &updateTx[secondaryIndexManyKey[IK, K], []byte]{
		viewTx: viewTx[secondaryIndexManyKey[IK, K], []byte]{
			namespace: i.namespace,
			txn:       txn,
			ctx:       ctx,
		},
	}
	return update.Delete(secondaryIndexManyKey[IK, K]{
		Secondary: i.keyOf(value),
		Primary:   primary,
	})
}

func (i *SecondaryIndexMany[K, V, IK]) resetIndex(ctx context.Context, txn *badger.Txn) error {
	return resetRawNamespace(txn, i.namespace.prefix)
}

type keyCodecValue[K any] struct {
	codec keycodec.Codec[K]
}

func (c keyCodecValue[K]) Marshal(value K) ([]byte, error) {
	if c.codec == nil {
		return nil, errors.Join(storx.ErrCodec, storx.ErrInvalidValue)
	}
	return c.codec.EncodeKey(value)
}

func (c keyCodecValue[K]) Unmarshal(data []byte) (K, error) {
	if c.codec == nil {
		var zero K
		return zero, errors.Join(storx.ErrCodec, storx.ErrInvalidValue)
	}
	return c.codec.DecodeKey(data)
}

// ModelStore is a small ORM-style layer for one typed Badger namespace.
type ModelStore[K any, V any] struct {
	repo              *Repository[K, V]
	keyOf             func(value V) K
	indexes           []badgerModelIndex[K, V]
	defaultSetOptions func(value V) []SetOption
	hooks             []ModelHooks[K, V]
}

func NewModelStore[K any, V any](
	db *badger.DB,
	prefix string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	keyOf func(value V) K,
	opts ...ModelStoreOption[K, V],
) *ModelStore[K, V] {
	return NewModelStoreFromRepository(NewRepository(db, prefix, keys, values), keyOf, opts...)
}

func NewModelStoreWithDB[K any, V any](
	db *DB,
	prefix string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	keyOf func(value V) K,
	opts ...ModelStoreOption[K, V],
) *ModelStore[K, V] {
	return NewModelStoreFromRepository(NewRepositoryWithDB(db, prefix, keys, values), keyOf, opts...)
}

func NewModelStoreFromRepository[K any, V any](
	repo *Repository[K, V],
	keyOf func(value V) K,
	opts ...ModelStoreOption[K, V],
) *ModelStore[K, V] {
	options := modelStoreOptions[K, V]{}
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return &ModelStore[K, V]{
		repo:              repo,
		keyOf:             keyOf,
		indexes:           append([]badgerModelIndex[K, V](nil), options.indexes...),
		defaultSetOptions: options.defaultSetOptions,
		hooks:             append([]ModelHooks[K, V](nil), options.hooks...),
	}
}

func (s *ModelStore[K, V]) Repository() *Repository[K, V] {
	if s == nil {
		return nil
	}
	return s.repo
}

func (s *ModelStore[K, V]) Get(ctx context.Context, key K) (V, bool, error) {
	return s.Repository().Get(ctx, key)
}

func (s *ModelStore[K, V]) GetRecord(ctx context.Context, key K) (Record[K, V], bool, error) {
	return s.Repository().GetRecord(ctx, key)
}

// Save preserves backward compatibility and behaves like Upsert.
func (s *ModelStore[K, V]) Save(ctx context.Context, value V, opts ...SetOption) (K, error) {
	return s.Upsert(ctx, value, opts...)
}

func (s *ModelStore[K, V]) SaveMany(ctx context.Context, values []V, opts ...SetOption) ([]Entry[K, V], error) {
	return s.UpsertMany(ctx, values, opts...)
}

func (s *ModelStore[K, V]) Create(ctx context.Context, value V, opts ...SetOption) (K, error) {
	entries, err := s.writeMany(ctx, modelWriteModeCreate, []V{value}, opts...)
	if err != nil {
		var zeroK K
		return zeroK, err
	}
	return entries[0].Key, nil
}

func (s *ModelStore[K, V]) CreateMany(ctx context.Context, values []V, opts ...SetOption) ([]Entry[K, V], error) {
	return s.writeMany(ctx, modelWriteModeCreate, values, opts...)
}

func (s *ModelStore[K, V]) Update(ctx context.Context, value V, opts ...SetOption) (K, error) {
	entries, err := s.writeMany(ctx, modelWriteModeUpdate, []V{value}, opts...)
	if err != nil {
		var zeroK K
		return zeroK, err
	}
	return entries[0].Key, nil
}

func (s *ModelStore[K, V]) UpdateMany(ctx context.Context, values []V, opts ...SetOption) ([]Entry[K, V], error) {
	return s.writeMany(ctx, modelWriteModeUpdate, values, opts...)
}

func (s *ModelStore[K, V]) Upsert(ctx context.Context, value V, opts ...SetOption) (K, error) {
	entries, err := s.writeMany(ctx, modelWriteModeUpsert, []V{value}, opts...)
	if err != nil {
		var zeroK K
		return zeroK, err
	}
	return entries[0].Key, nil
}

func (s *ModelStore[K, V]) UpsertMany(ctx context.Context, values []V, opts ...SetOption) ([]Entry[K, V], error) {
	return s.writeMany(ctx, modelWriteModeUpsert, values, opts...)
}

func (s *ModelStore[K, V]) Delete(ctx context.Context, key K) error {
	return s.DeleteMany(ctx, key)
}

func (s *ModelStore[K, V]) DeleteMany(ctx context.Context, keys ...K) error {
	if err := s.validate(); err != nil {
		return err
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}

	var after []ModelHookEvent[K, V]
	err := s.Repository().Namespace().db.Update(func(txn *badger.Txn) error {
		main := &updateTx[K, V]{
			viewTx: viewTx[K, V]{
				namespace: s.Repository().Namespace(),
				txn:       txn,
				ctx:       ctx,
			},
		}
		for _, key := range keys {
			event, existed, err := s.deleteOne(ctx, txn, main, key)
			if err != nil {
				return err
			}
			if existed {
				after = append(after, event)
			}
		}
		return nil
	})
	err = s.Repository().Namespace().normalizeEngineError("model_delete_many", err)
	if err != nil {
		return err
	}
	for _, event := range after {
		s.runAfterDeleteHooks(ctx, event)
	}
	return nil
}

func (s *ModelStore[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	return s.Repository().Exists(ctx, key)
}

func (s *ModelStore[K, V]) List(ctx context.Context, opts ...ListOption[K]) ([]Entry[K, V], error) {
	return s.Repository().List(ctx, opts...)
}

func (s *ModelStore[K, V]) Iter(ctx context.Context, opts ...ListOption[K]) (Iterator[K, V], error) {
	return s.Repository().Iter(ctx, opts...)
}

func (s *ModelStore[K, V]) Walk(ctx context.Context, fn func(entry Entry[K, V]) error, opts ...ListOption[K]) error {
	return s.Repository().Walk(ctx, fn, opts...)
}

func (s *ModelStore[K, V]) RebuildIndexes(ctx context.Context) error {
	if err := s.validate(); err != nil {
		return err
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}

	now := time.Now()
	err := s.Repository().Namespace().db.Update(func(txn *badger.Txn) error {
		for _, index := range s.indexes {
			if err := index.resetIndex(ctx, txn); err != nil {
				return err
			}
		}

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = s.Repository().Namespace().opts.copyValue
		opts.Prefix = s.Repository().Namespace().prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(s.Repository().Namespace().prefix); it.ValidForPrefix(s.Repository().Namespace().prefix); it.Next() {
			item := it.Item()
			key, err := s.Repository().Namespace().decodeUserKey("model_rebuild_indexes", item.KeyCopy(nil))
			if err != nil {
				return err
			}
			value, err := s.Repository().Namespace().readItemValue("model_rebuild_indexes", item)
			if err != nil {
				return err
			}
			setOpts := rebuildSetOptions(value, item, now)
			for _, index := range s.indexes {
				if err := index.putIndex(ctx, txn, key, value, setOpts...); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return s.Repository().Namespace().normalizeEngineError("model_rebuild_indexes", err)
}

func (s *ModelStore[K, V]) RepairIndexes(ctx context.Context) error {
	return s.RebuildIndexes(ctx)
}

func (s *ModelStore[K, V]) writeMany(ctx context.Context, mode modelWriteMode, values []V, opts ...SetOption) ([]Entry[K, V], error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries := make([]Entry[K, V], 0, len(values))
	after := make([]ModelHookEvent[K, V], 0, len(values))
	err := s.Repository().Namespace().db.Update(func(txn *badger.Txn) error {
		main := &updateTx[K, V]{
			viewTx: viewTx[K, V]{
				namespace: s.Repository().Namespace(),
				txn:       txn,
				ctx:       ctx,
			},
		}
		for _, value := range values {
			entry, event, err := s.writeOne(ctx, txn, main, value, mode, opts...)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
			after = append(after, event)
		}
		return nil
	})
	err = s.Repository().Namespace().normalizeEngineError("model_"+string(mode)+"_many", err)
	if err != nil {
		return nil, err
	}
	for _, event := range after {
		s.runAfterWriteHooks(ctx, event)
	}
	return entries, nil
}

func (s *ModelStore[K, V]) writeOne(
	ctx context.Context,
	txn *badger.Txn,
	main *updateTx[K, V],
	value V,
	mode modelWriteMode,
	opts ...SetOption,
) (Entry[K, V], ModelHookEvent[K, V], error) {
	stored := value
	primary := s.keyOf(stored)
	setOpts := s.resolveSetOptions(stored, opts)

	existing, existed, err := main.Get(primary)
	if err != nil {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
	}
	switch mode {
	case modelWriteModeCreate:
		if existed {
			return Entry[K, V]{}, ModelHookEvent[K, V]{}, s.Repository().Namespace().wrapError("model_create", storx.ErrAlreadyExists, "model already exists")
		}
	case modelWriteModeUpdate:
		if !existed {
			return Entry[K, V]{}, ModelHookEvent[K, V]{}, s.Repository().Namespace().wrapError("model_update", storx.ErrNotFound, "model not found")
		}
	}

	event := newWriteEvent[K, V](mode, primary, &stored, existed, valuePtr(existing, existed))
	if err := s.runBeforeWriteHooks(ctx, &event); err != nil {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
	}
	if mutatedKey := s.keyOf(stored); !reflect.DeepEqual(mutatedKey, primary) {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, s.Repository().Namespace().wrapError("model_write_hooks", storx.ErrInvalidKey, "hooks must not mutate primary key")
	}
	setOpts = s.resolveSetOptions(stored, opts)

	if existed {
		for _, index := range s.indexes {
			if err := index.deleteIndex(ctx, txn, primary, existing); err != nil {
				return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
			}
		}
	}
	if err := main.Set(primary, stored, setOpts...); err != nil {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
	}
	for _, index := range s.indexes {
		if err := index.putIndex(ctx, txn, primary, stored, setOpts...); err != nil {
			return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
		}
	}

	event = newWriteEvent[K, V](mode, primary, &stored, existed, valuePtr(existing, existed))
	return Entry[K, V]{Key: primary, Value: stored}, event, nil
}

func (s *ModelStore[K, V]) deleteOne(
	ctx context.Context,
	txn *badger.Txn,
	main *updateTx[K, V],
	key K,
) (ModelHookEvent[K, V], bool, error) {
	existing, existed, err := main.Get(key)
	if err != nil {
		return ModelHookEvent[K, V]{}, false, err
	}
	if !existed {
		return ModelHookEvent[K, V]{}, false, nil
	}

	event := newDeleteEvent(key, existing)
	if err := s.runBeforeDeleteHooks(ctx, &event); err != nil {
		return ModelHookEvent[K, V]{}, false, err
	}
	for _, index := range s.indexes {
		if err := index.deleteIndex(ctx, txn, key, existing); err != nil {
			return ModelHookEvent[K, V]{}, false, err
		}
	}
	if err := main.Delete(key); err != nil {
		return ModelHookEvent[K, V]{}, false, err
	}
	return newDeleteEvent(key, existing), true, nil
}

func (s *ModelStore[K, V]) resolveSetOptions(value V, opts []SetOption) []SetOption {
	resolved := make([]SetOption, 0, len(opts)+2)
	if s.defaultSetOptions != nil {
		resolved = append(resolved, s.defaultSetOptions(value)...)
	}
	resolved = append(resolved, opts...)
	return resolved
}

func (s *ModelStore[K, V]) runBeforeWriteHooks(ctx context.Context, event *ModelHookEvent[K, V]) error {
	for _, hook := range s.hooks {
		if hook.BeforeWrite != nil {
			if err := hook.BeforeWrite(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ModelStore[K, V]) runAfterWriteHooks(ctx context.Context, event ModelHookEvent[K, V]) {
	for _, hook := range s.hooks {
		if hook.AfterWrite != nil {
			hook.AfterWrite(ctx, event)
		}
	}
}

func (s *ModelStore[K, V]) runBeforeDeleteHooks(ctx context.Context, event *ModelHookEvent[K, V]) error {
	for _, hook := range s.hooks {
		if hook.BeforeDelete != nil {
			if err := hook.BeforeDelete(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ModelStore[K, V]) runAfterDeleteHooks(ctx context.Context, event ModelHookEvent[K, V]) {
	for _, hook := range s.hooks {
		if hook.AfterDelete != nil {
			hook.AfterDelete(ctx, event)
		}
	}
}

func (s *ModelStore[K, V]) validate() error {
	switch {
	case s == nil:
		return errors.Join(storx.ErrInvalidValue, storx.ErrClosed)
	case s.repo == nil:
		return errors.Join(storx.ErrInvalidValue, storx.ErrClosed)
	case s.repo.Namespace() == nil:
		return errors.Join(storx.ErrInvalidValue, storx.ErrClosed)
	case s.keyOf == nil:
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	default:
		return nil
	}
}

func secondaryIndexManyCodec[IK any, K any](keys keycodec.Codec[IK], primaryKeys keycodec.Codec[K]) keycodec.Codec[secondaryIndexManyKey[IK, K]] {
	return keycodec.Composite(
		keycodec.Field(
			keys,
			func(value secondaryIndexManyKey[IK, K]) IK { return value.Secondary },
			func(target *secondaryIndexManyKey[IK, K], field IK) { target.Secondary = field },
		),
		keycodec.Field(
			primaryKeys,
			func(value secondaryIndexManyKey[IK, K]) K { return value.Primary },
			func(target *secondaryIndexManyKey[IK, K], field K) { target.Primary = field },
		),
	)
}

func resetRawNamespace(txn *badger.Txn, prefix []byte) error {
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()

	var keys [][]byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		keys = append(keys, it.Item().KeyCopy(nil))
	}
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func rebuildSetOptions[V any](value V, item *badger.Item, now time.Time) []SetOption {
	opts := []SetOption{WithMeta(item.UserMeta())}
	if item.DiscardEarlierVersions() {
		opts = append(opts, WithDiscard())
	}
	if expiresAt := item.ExpiresAt(); expiresAt > 0 {
		expiry := time.Unix(int64(expiresAt), 0).UTC()
		if ttl := expiry.Sub(now); ttl > 0 {
			opts = append(opts, WithTTL(ttl))
		}
	}
	return opts
}

func newWriteEvent[K any, V any](mode modelWriteMode, key K, value *V, existed bool, previous *V) ModelHookEvent[K, V] {
	return ModelHookEvent[K, V]{
		Operation: modelWriteModeToOperation(mode),
		Key:       key,
		Value:     value,
		Previous:  previous,
		Exists:    existed,
	}
}

func newDeleteEvent[K any, V any](key K, value V) ModelHookEvent[K, V] {
	current := value
	return ModelHookEvent[K, V]{
		Operation: ModelOperationDelete,
		Key:       key,
		Value:     &current,
		Previous:  &current,
		Exists:    true,
	}
}

func modelWriteModeToOperation(mode modelWriteMode) ModelOperation {
	switch mode {
	case modelWriteModeCreate:
		return ModelOperationCreate
	case modelWriteModeUpdate:
		return ModelOperationUpdate
	default:
		return ModelOperationUpsert
	}
}

func valuePtr[V any](value V, ok bool) *V {
	if !ok {
		return nil
	}
	copyValue := value
	return &copyValue
}
