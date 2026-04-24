package badgerx

import (
	"context"
	"errors"

	storx "github.com/arcgolabs/storx"
	"github.com/dgraph-io/badger/v4"
)

type ViewTx[K any, V any] interface {
	Get(key K) (V, bool, error)
	Exists(key K) (bool, error)
	Scan(fn func(key K, value V) error) error
	ScanPrefix(prefix []byte, fn func(key K, value V) error) error
}

type UpdateTx[K any, V any] interface {
	ViewTx[K, V]

	Set(key K, value V, opts ...SetOption) error
	Delete(key K) error
}

type viewTx[K any, V any] struct {
	namespace *Namespace[K, V]
	txn       *badger.Txn
	ctx       context.Context
}

type updateTx[K any, V any] struct {
	viewTx[K, V]
}

func (tx *viewTx[K, V]) Get(key K) (V, bool, error) {
	var zero V

	if err := tx.ctx.Err(); err != nil {
		return zero, false, err
	}

	fullKey, err := tx.namespace.fullKey("get", key)
	if err != nil {
		return zero, false, err
	}

	item, err := tx.txn.Get(fullKey)
	switch {
	case err == nil:
	case errors.Is(err, badger.ErrKeyNotFound):
		return zero, false, nil
	default:
		return zero, false, tx.namespace.wrapError("get", err, "read key from badger")
	}

	value, err := tx.namespace.readItemValue("get", item)
	if err != nil {
		return zero, false, err
	}
	return value, true, nil
}

func (tx *viewTx[K, V]) Exists(key K) (bool, error) {
	if err := tx.ctx.Err(); err != nil {
		return false, err
	}

	fullKey, err := tx.namespace.fullKey("exists", key)
	if err != nil {
		return false, err
	}

	_, err = tx.txn.Get(fullKey)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, badger.ErrKeyNotFound):
		return false, nil
	default:
		return false, tx.namespace.wrapError("exists", err, "read key from badger")
	}
}

func (tx *viewTx[K, V]) Scan(fn func(key K, value V) error) error {
	return tx.ScanPrefix(nil, fn)
}

func (tx *viewTx[K, V]) ScanPrefix(prefix []byte, fn func(key K, value V) error) error {
	if fn == nil {
		return tx.namespace.wrapError("scan_prefix", errors.Join(storx.ErrInvalidValue, storx.ErrCodec), "callback is nil")
	}

	if err := tx.ctx.Err(); err != nil {
		return err
	}

	targetPrefix := tx.namespace.fullPrefix(prefix)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = targetPrefix
	opts.PrefetchValues = tx.namespace.opts.copyValue

	it := tx.txn.NewIterator(opts)
	defer it.Close()

	for it.Seek(targetPrefix); it.ValidForPrefix(targetPrefix); it.Next() {
		if err := tx.ctx.Err(); err != nil {
			return err
		}

		item := it.Item()
		key, err := tx.namespace.decodeUserKey("scan_prefix", item.KeyCopy(nil))
		if err != nil {
			return err
		}

		value, err := tx.namespace.readItemValue("scan_prefix", item)
		if err != nil {
			return err
		}

		if err := fn(key, value); err != nil {
			return err
		}
	}

	return nil
}

func (tx *updateTx[K, V]) Set(key K, value V, opts ...SetOption) error {
	if err := tx.ctx.Err(); err != nil {
		return err
	}

	fullKey, err := tx.namespace.fullKey("set", key)
	if err != nil {
		return err
	}
	encodedValue, err := tx.namespace.encodeValue("set", value)
	if err != nil {
		return err
	}

	entry := badger.NewEntry(fullKey, encodedValue)
	setOpts := setOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&setOpts)
		}
	}
	if setOpts.hasTTL {
		entry = entry.WithTTL(setOpts.ttl)
	}
	if setOpts.hasMeta {
		entry = entry.WithMeta(setOpts.meta)
	}
	if setOpts.discard {
		entry = entry.WithDiscard()
	}

	if err := tx.txn.SetEntry(entry); err != nil {
		return tx.namespace.wrapError("set", err, "write key to badger")
	}
	return nil
}

func (tx *updateTx[K, V]) Delete(key K) error {
	if err := tx.ctx.Err(); err != nil {
		return err
	}

	fullKey, err := tx.namespace.fullKey("delete", key)
	if err != nil {
		return err
	}
	if err := tx.txn.Delete(fullKey); err != nil {
		return tx.namespace.wrapError("delete", err, "delete key from badger")
	}
	return nil
}
