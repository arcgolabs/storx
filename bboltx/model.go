package bboltx

import (
	"context"
	"errors"
	"reflect"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"go.etcd.io/bbolt"
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

// ModelHooks defines optional lifecycle callbacks for one bbolt model store.
type ModelHooks[K any, V any] struct {
	BeforeWrite  func(ctx context.Context, event *ModelHookEvent[K, V]) error
	AfterWrite   func(ctx context.Context, event ModelHookEvent[K, V])
	BeforeDelete func(ctx context.Context, event *ModelHookEvent[K, V]) error
	AfterDelete  func(ctx context.Context, event ModelHookEvent[K, V])
}

type bboltModelIndex[K any, V any] interface {
	putIndex(ctx context.Context, tx *bbolt.Tx, primary K, value V) error
	deleteIndex(ctx context.Context, tx *bbolt.Tx, primary K, value V) error
	resetIndex(ctx context.Context, tx *bbolt.Tx) error
}

// ModelIndexDefinition opens a model index from a raw DB or DB wrapper.
type ModelIndexDefinition[K any, V any] interface {
	openWithRaw(db *bbolt.DB, primaryKeys keycodec.Codec[K]) bboltModelIndex[K, V]
	openWithDB(db *DB, primaryKeys keycodec.Codec[K]) bboltModelIndex[K, V]
}

type modelStoreOptions[K any, V any] struct {
	assignSequence func(seq uint64, value *V) K
	indexes        []bboltModelIndex[K, V]
	hooks          []ModelHooks[K, V]
}

// ModelStoreOption configures a bbolt model store.
type ModelStoreOption[K any, V any] func(*modelStoreOptions[K, V])

// WithSequenceAssigner enables auto-generated primary keys backed by bbolt
// bucket sequence values.
func WithSequenceAssigner[K any, V any](assign func(seq uint64, value *V) K) ModelStoreOption[K, V] {
	return func(opts *modelStoreOptions[K, V]) {
		opts.assignSequence = assign
	}
}

// WithUint64SequenceField populates a uint64 primary key field from the bbolt
// bucket sequence.
func WithUint64SequenceField[V any](set func(target *V, id uint64)) ModelStoreOption[uint64, V] {
	return WithSequenceAssigner(func(seq uint64, value *V) uint64 {
		if set != nil {
			set(value, seq)
		}
		return seq
	})
}

// WithModelIndex registers a secondary index for a model store.
func WithModelIndex[K any, V any](index bboltModelIndex[K, V]) ModelStoreOption[K, V] {
	return func(opts *modelStoreOptions[K, V]) {
		if index != nil {
			opts.indexes = append(opts.indexes, index)
		}
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
	bucket *Bucket[IK, K]
	keyOf  func(value V) IK
}

type secondaryIndexManyKey[IK any, K any] struct {
	Secondary IK
	Primary   K
}

// SecondaryIndexMany stores a non-unique secondary-key to primary-key mapping.
type SecondaryIndexMany[K any, V any, IK any] struct {
	bucket *Bucket[secondaryIndexManyKey[IK, K], []byte]
	keys   keycodec.Codec[IK]
	keyOf  func(value V) IK
}

// SecondaryIndexDefinition declares a named secondary index for a model
// schema.
type SecondaryIndexDefinition[K any, V any, IK any] struct {
	Name    string
	Keys    keycodec.Codec[IK]
	KeyOf   func(value V) IK
	Options []BucketOption
}

// SecondaryIndexManyDefinition declares a named non-unique secondary index for
// a model schema.
type SecondaryIndexManyDefinition[K any, V any, IK any] struct {
	Name    string
	Keys    keycodec.Codec[IK]
	KeyOf   func(value V) IK
	Options []BucketOption
}

func (d SecondaryIndexDefinition[K, V, IK]) Open(db *bbolt.DB, primaryKeys keycodec.Codec[K]) *SecondaryIndex[K, V, IK] {
	return NewSecondaryIndex(db, d.Name, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexDefinition[K, V, IK]) OpenWithDB(db *DB, primaryKeys keycodec.Codec[K]) *SecondaryIndex[K, V, IK] {
	return NewSecondaryIndexWithDB(db, d.Name, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexDefinition[K, V, IK]) openWithRaw(db *bbolt.DB, primaryKeys keycodec.Codec[K]) bboltModelIndex[K, V] {
	return d.Open(db, primaryKeys)
}

func (d SecondaryIndexDefinition[K, V, IK]) openWithDB(db *DB, primaryKeys keycodec.Codec[K]) bboltModelIndex[K, V] {
	return d.OpenWithDB(db, primaryKeys)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) Open(db *bbolt.DB, primaryKeys keycodec.Codec[K]) *SecondaryIndexMany[K, V, IK] {
	return NewSecondaryIndexMany(db, d.Name, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) OpenWithDB(db *DB, primaryKeys keycodec.Codec[K]) *SecondaryIndexMany[K, V, IK] {
	return NewSecondaryIndexManyWithDB(db, d.Name, d.Keys, primaryKeys, d.KeyOf, d.Options...)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) openWithRaw(db *bbolt.DB, primaryKeys keycodec.Codec[K]) bboltModelIndex[K, V] {
	return d.Open(db, primaryKeys)
}

func (d SecondaryIndexManyDefinition[K, V, IK]) openWithDB(db *DB, primaryKeys keycodec.Codec[K]) bboltModelIndex[K, V] {
	return d.OpenWithDB(db, primaryKeys)
}

// ModelSchema declares one bbolt model and its indexes.
type ModelSchema[K any, V any] struct {
	Name             string
	Keys             keycodec.Codec[K]
	Values           codec.Codec[V]
	KeyOf            func(value V) K
	Options          []BucketOption
	SequenceAssigner func(seq uint64, value *V) K
	Indexes          []ModelIndexDefinition[K, V]
	Hooks            []ModelHooks[K, V]
}

func (s ModelSchema[K, V]) Open(db *bbolt.DB) *ModelStore[K, V] {
	options := make([]ModelStoreOption[K, V], 0, len(s.Indexes)+2)
	if s.SequenceAssigner != nil {
		options = append(options, WithSequenceAssigner(s.SequenceAssigner))
	}
	if len(s.Hooks) > 0 {
		options = append(options, WithModelHooks(s.Hooks...))
	}
	for _, index := range s.Indexes {
		if index != nil {
			options = append(options, WithModelIndex(index.openWithRaw(db, s.Keys)))
		}
	}
	repo := NewRepository(db, s.Name, s.Keys, s.Values, s.Options...)
	return NewModelStoreFromRepository(repo, s.KeyOf, options...)
}

func (s ModelSchema[K, V]) OpenWithDB(db *DB) *ModelStore[K, V] {
	options := make([]ModelStoreOption[K, V], 0, len(s.Indexes)+2)
	if s.SequenceAssigner != nil {
		options = append(options, WithSequenceAssigner(s.SequenceAssigner))
	}
	if len(s.Hooks) > 0 {
		options = append(options, WithModelHooks(s.Hooks...))
	}
	for _, index := range s.Indexes {
		if index != nil {
			options = append(options, WithModelIndex(index.openWithDB(db, s.Keys)))
		}
	}
	repo := NewRepositoryWithDB(db, s.Name, s.Keys, s.Values, s.Options...)
	return NewModelStoreFromRepository(repo, s.KeyOf, options...)
}

// NewSecondaryIndex creates a typed secondary index bucket.
func NewSecondaryIndex[K any, V any, IK any](
	db *bbolt.DB,
	name string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...BucketOption,
) *SecondaryIndex[K, V, IK] {
	return &SecondaryIndex[K, V, IK]{
		bucket: NewBucket(db, name, keys, keyCodecValue[K]{codec: primaryKeys}, opts...),
		keyOf:  keyOf,
	}
}

// NewSecondaryIndexWithDB creates a typed secondary index bucket from a DB
// wrapper.
func NewSecondaryIndexWithDB[K any, V any, IK any](
	db *DB,
	name string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...BucketOption,
) *SecondaryIndex[K, V, IK] {
	return &SecondaryIndex[K, V, IK]{
		bucket: NewBucketWithDB(db, name, keys, keyCodecValue[K]{codec: primaryKeys}, opts...),
		keyOf:  keyOf,
	}
}

// NewSecondaryIndexMany creates a typed non-unique secondary index bucket.
func NewSecondaryIndexMany[K any, V any, IK any](
	db *bbolt.DB,
	name string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...BucketOption,
) *SecondaryIndexMany[K, V, IK] {
	return &SecondaryIndexMany[K, V, IK]{
		bucket: NewBucket(
			db,
			name,
			secondaryIndexManyCodec(keys, primaryKeys),
			codec.Bytes(),
			opts...,
		),
		keys:  keys,
		keyOf: keyOf,
	}
}

// NewSecondaryIndexManyWithDB creates a typed non-unique secondary index bucket
// from a DB wrapper.
func NewSecondaryIndexManyWithDB[K any, V any, IK any](
	db *DB,
	name string,
	keys keycodec.Codec[IK],
	primaryKeys keycodec.Codec[K],
	keyOf func(value V) IK,
	opts ...BucketOption,
) *SecondaryIndexMany[K, V, IK] {
	return &SecondaryIndexMany[K, V, IK]{
		bucket: NewBucketWithDB(
			db,
			name,
			secondaryIndexManyCodec(keys, primaryKeys),
			codec.Bytes(),
			opts...,
		),
		keys:  keys,
		keyOf: keyOf,
	}
}

func (i *SecondaryIndex[K, V, IK]) Bucket() *Bucket[IK, K] {
	if i == nil {
		return nil
	}
	return i.bucket
}

func (i *SecondaryIndex[K, V, IK]) GetPrimary(ctx context.Context, key IK) (K, bool, error) {
	return i.Bucket().Get(ctx, key)
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

func (i *SecondaryIndexMany[K, V, IK]) Bucket() *Bucket[secondaryIndexManyKey[IK, K], []byte] {
	if i == nil {
		return nil
	}
	return i.bucket
}

func (i *SecondaryIndexMany[K, V, IK]) ListPrimaries(ctx context.Context, key IK) ([]K, error) {
	prefix, err := keycodec.ComponentPrefix(i.keys, key)
	if err != nil {
		return nil, err
	}
	iter, err := i.Bucket().Iter(ctx, WithPrefix[secondaryIndexManyKey[IK, K]](prefix))
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
	iter, err := i.Bucket().Iter(ctx, WithPrefix[secondaryIndexManyKey[IK, K]](prefix))
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

func (i *SecondaryIndex[K, V, IK]) putIndex(ctx context.Context, tx *bbolt.Tx, primary K, value V) error {
	if i == nil || i.bucket == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update, err := i.bucket.newUpdateTx(ctx, tx)
	if err != nil {
		return err
	}

	indexKey := i.keyOf(value)
	existingPrimary, ok, err := update.Get(indexKey)
	if err != nil {
		return err
	}
	if ok && !reflect.DeepEqual(existingPrimary, primary) {
		return i.bucket.wrapError("model_index_put", storx.ErrAlreadyExists, "secondary index key already exists")
	}
	return update.Put(indexKey, primary)
}

func (i *SecondaryIndex[K, V, IK]) deleteIndex(ctx context.Context, tx *bbolt.Tx, primary K, value V) error {
	if i == nil || i.bucket == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update, err := i.bucket.newUpdateTx(ctx, tx)
	if err != nil {
		return err
	}
	return update.Delete(i.keyOf(value))
}

func (i *SecondaryIndex[K, V, IK]) resetIndex(ctx context.Context, tx *bbolt.Tx) error {
	bucket := tx.Bucket(i.bucket.nameBytes)
	if bucket == nil {
		return nil
	}
	return clearRawBucket(bucket)
}

func (i *SecondaryIndexMany[K, V, IK]) putIndex(ctx context.Context, tx *bbolt.Tx, primary K, value V) error {
	if i == nil || i.bucket == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update, err := i.bucket.newUpdateTx(ctx, tx)
	if err != nil {
		return err
	}
	return update.Put(secondaryIndexManyKey[IK, K]{
		Secondary: i.keyOf(value),
		Primary:   primary,
	}, []byte{1})
}

func (i *SecondaryIndexMany[K, V, IK]) deleteIndex(ctx context.Context, tx *bbolt.Tx, primary K, value V) error {
	if i == nil || i.bucket == nil || i.keyOf == nil {
		return errors.Join(storx.ErrInvalidValue, storx.ErrCodec)
	}
	update, err := i.bucket.newUpdateTx(ctx, tx)
	if err != nil {
		return err
	}
	return update.Delete(secondaryIndexManyKey[IK, K]{
		Secondary: i.keyOf(value),
		Primary:   primary,
	})
}

func (i *SecondaryIndexMany[K, V, IK]) resetIndex(ctx context.Context, tx *bbolt.Tx) error {
	bucket := tx.Bucket(i.bucket.nameBytes)
	if bucket == nil {
		return nil
	}
	return clearRawBucket(bucket)
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

// ModelStore is a small ORM-style layer for one typed bbolt model bucket.
type ModelStore[K any, V any] struct {
	repo           *Repository[K, V]
	keyOf          func(value V) K
	assignSequence func(seq uint64, value *V) K
	indexes        []bboltModelIndex[K, V]
	hooks          []ModelHooks[K, V]
}

func NewModelStore[K any, V any](
	db *bbolt.DB,
	name string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	keyOf func(value V) K,
	opts ...ModelStoreOption[K, V],
) *ModelStore[K, V] {
	return NewModelStoreFromRepository(NewRepository(db, name, keys, values), keyOf, opts...)
}

func NewModelStoreWithDB[K any, V any](
	db *DB,
	name string,
	keys keycodec.Codec[K],
	values codec.Codec[V],
	keyOf func(value V) K,
	opts ...ModelStoreOption[K, V],
) *ModelStore[K, V] {
	return NewModelStoreFromRepository(NewRepositoryWithDB(db, name, keys, values), keyOf, opts...)
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
		repo:           repo,
		keyOf:          keyOf,
		assignSequence: options.assignSequence,
		indexes:        append([]bboltModelIndex[K, V](nil), options.indexes...),
		hooks:          append([]ModelHooks[K, V](nil), options.hooks...),
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

// Save preserves backward compatibility and behaves like Upsert.
func (s *ModelStore[K, V]) Save(ctx context.Context, value V) (V, K, error) {
	return s.Upsert(ctx, value)
}

func (s *ModelStore[K, V]) SaveMany(ctx context.Context, values []V) ([]Entry[K, V], error) {
	return s.UpsertMany(ctx, values)
}

func (s *ModelStore[K, V]) Create(ctx context.Context, value V) (V, K, error) {
	entries, err := s.writeMany(ctx, modelWriteModeCreate, []V{value})
	if err != nil {
		var zeroV V
		var zeroK K
		return zeroV, zeroK, err
	}
	return entries[0].Value, entries[0].Key, nil
}

func (s *ModelStore[K, V]) CreateMany(ctx context.Context, values []V) ([]Entry[K, V], error) {
	return s.writeMany(ctx, modelWriteModeCreate, values)
}

func (s *ModelStore[K, V]) Update(ctx context.Context, value V) (V, K, error) {
	entries, err := s.writeMany(ctx, modelWriteModeUpdate, []V{value})
	if err != nil {
		var zeroV V
		var zeroK K
		return zeroV, zeroK, err
	}
	return entries[0].Value, entries[0].Key, nil
}

func (s *ModelStore[K, V]) UpdateMany(ctx context.Context, values []V) ([]Entry[K, V], error) {
	return s.writeMany(ctx, modelWriteModeUpdate, values)
}

func (s *ModelStore[K, V]) Upsert(ctx context.Context, value V) (V, K, error) {
	entries, err := s.writeMany(ctx, modelWriteModeUpsert, []V{value})
	if err != nil {
		var zeroV V
		var zeroK K
		return zeroV, zeroK, err
	}
	return entries[0].Value, entries[0].Key, nil
}

func (s *ModelStore[K, V]) UpsertMany(ctx context.Context, values []V) ([]Entry[K, V], error) {
	return s.writeMany(ctx, modelWriteModeUpsert, values)
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
	err := s.Repository().Bucket().db.Update(func(tx *bbolt.Tx) error {
		main, err := s.Repository().Bucket().newUpdateTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, key := range keys {
			event, existed, err := s.deleteOne(ctx, tx, main, key)
			if err != nil {
				return err
			}
			if existed {
				after = append(after, event)
			}
		}
		return nil
	})
	err = s.Repository().Bucket().normalizeEngineError("model_delete_many", err)
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

	err := s.Repository().Bucket().db.Update(func(tx *bbolt.Tx) error {
		for _, index := range s.indexes {
			if err := index.resetIndex(ctx, tx); err != nil {
				return err
			}
		}

		bucket := tx.Bucket(s.Repository().Bucket().nameBytes)
		if bucket == nil {
			return nil
		}

		return bucket.ForEach(func(rawKey, rawValue []byte) error {
			if rawValue == nil {
				return nil
			}
			key, err := s.Repository().Bucket().decodeKey("model_rebuild_indexes", rawKey)
			if err != nil {
				return err
			}
			value, err := s.Repository().Bucket().decodeValue("model_rebuild_indexes", rawValue)
			if err != nil {
				return err
			}
			for _, index := range s.indexes {
				if err := index.putIndex(ctx, tx, key, value); err != nil {
					return err
				}
			}
			return nil
		})
	})
	return s.Repository().Bucket().normalizeEngineError("model_rebuild_indexes", err)
}

func (s *ModelStore[K, V]) RepairIndexes(ctx context.Context) error {
	return s.RebuildIndexes(ctx)
}

func (s *ModelStore[K, V]) writeMany(ctx context.Context, mode modelWriteMode, values []V) ([]Entry[K, V], error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	ctx = normalizeContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries := make([]Entry[K, V], 0, len(values))
	after := make([]ModelHookEvent[K, V], 0, len(values))
	err := s.Repository().Bucket().db.Update(func(tx *bbolt.Tx) error {
		main, err := s.Repository().Bucket().newUpdateTx(ctx, tx)
		if err != nil {
			return err
		}
		for _, value := range values {
			entry, event, err := s.writeOne(ctx, tx, main, value, mode)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
			after = append(after, event)
		}
		return nil
	})
	err = s.Repository().Bucket().normalizeEngineError("model_"+string(mode)+"_many", err)
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
	tx *bbolt.Tx,
	main *updateTx[K, V],
	value V,
	mode modelWriteMode,
) (Entry[K, V], ModelHookEvent[K, V], error) {
	stored := value
	primary := s.keyOf(stored)

	if mode != modelWriteModeUpdate && isZeroValue(primary) && s.assignSequence != nil {
		seq, err := main.NextSequence()
		if err != nil {
			return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
		}
		primary = s.assignSequence(seq, &stored)
	}

	existing, existed, err := main.Get(primary)
	if err != nil {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
	}
	switch mode {
	case modelWriteModeCreate:
		if existed {
			return Entry[K, V]{}, ModelHookEvent[K, V]{}, s.Repository().Bucket().wrapError("model_create", storx.ErrAlreadyExists, "model already exists")
		}
	case modelWriteModeUpdate:
		if !existed {
			return Entry[K, V]{}, ModelHookEvent[K, V]{}, s.Repository().Bucket().wrapError("model_update", storx.ErrNotFound, "model not found")
		}
	}

	event := newWriteEvent[K, V](mode, primary, &stored, existed, valuePtr(existing, existed))
	if err := s.runBeforeWriteHooks(ctx, &event); err != nil {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
	}
	if mutatedKey := s.keyOf(stored); !reflect.DeepEqual(mutatedKey, primary) {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, s.Repository().Bucket().wrapError("model_write_hooks", storx.ErrInvalidKey, "hooks must not mutate primary key")
	}

	if existed {
		for _, index := range s.indexes {
			if err := index.deleteIndex(ctx, tx, primary, existing); err != nil {
				return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
			}
		}
	}
	if err := main.Put(primary, stored); err != nil {
		return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
	}
	for _, index := range s.indexes {
		if err := index.putIndex(ctx, tx, primary, stored); err != nil {
			return Entry[K, V]{}, ModelHookEvent[K, V]{}, err
		}
	}

	event = newWriteEvent[K, V](mode, primary, &stored, existed, valuePtr(existing, existed))
	return Entry[K, V]{Key: primary, Value: stored}, event, nil
}

func (s *ModelStore[K, V]) deleteOne(
	ctx context.Context,
	tx *bbolt.Tx,
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
		if err := index.deleteIndex(ctx, tx, key, existing); err != nil {
			return ModelHookEvent[K, V]{}, false, err
		}
	}
	if err := main.Delete(key); err != nil {
		return ModelHookEvent[K, V]{}, false, err
	}
	return newDeleteEvent(key, existing), true, nil
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
	case s.repo.Bucket() == nil:
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

func clearRawBucket(bucket *bbolt.Bucket) error {
	cursor := bucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		if err := cursor.Delete(); err != nil {
			return err
		}
	}
	return nil
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

func isZeroValue[T any](value T) bool {
	var zero T
	return reflect.DeepEqual(value, zero)
}
