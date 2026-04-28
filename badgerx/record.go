package badgerx

import (
	"context"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// Metadata exposes Badger item metadata in typed APIs.
type Metadata struct {
	UserMeta               byte
	Version                uint64
	ExpiresAt              time.Time
	TTL                    time.Duration
	HasTTL                 bool
	DeletedOrExpired       bool
	ValueSize              int64
	EstimatedSize          int64
	DiscardEarlierVersions bool
}

// Record is a typed key/value pair with Badger metadata.
type Record[K any, V any] struct {
	Key      K
	Value    V
	Metadata Metadata
}

// GetMetadata reads Badger metadata without decoding the value payload.
func (n *Namespace[K, V]) GetMetadata(ctx context.Context, key K) (Metadata, bool, error) {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("get_metadata"); err != nil {
		n.finishOperation(ctx, start, "get_metadata", err)
		return Metadata{}, false, err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "get_metadata", err)
		return Metadata{}, false, err
	}

	var (
		meta Metadata
		ok   bool
	)

	err := n.db.View(func(txn *badger.Txn) error {
		view := &viewTx[K, V]{
			namespace: n,
			txn:       txn,
			ctx:       ctx,
		}

		var err error
		meta, ok, err = view.GetMetadata(key)
		return err
	})
	err = n.normalizeEngineError("get_metadata", err)
	n.finishOperation(ctx, start, "get_metadata", err)
	return meta, ok, err
}

// GetRecord reads a typed value together with Badger metadata.
func (n *Namespace[K, V]) GetRecord(ctx context.Context, key K) (Record[K, V], bool, error) {
	ctx, start := n.startOperation(ctx)

	if err := n.validate("get_record"); err != nil {
		n.finishOperation(ctx, start, "get_record", err)
		return Record[K, V]{}, false, err
	}
	if err := ctx.Err(); err != nil {
		n.finishOperation(ctx, start, "get_record", err)
		return Record[K, V]{}, false, err
	}

	var (
		record Record[K, V]
		ok     bool
	)

	err := n.db.View(func(txn *badger.Txn) error {
		view := &viewTx[K, V]{
			namespace: n,
			txn:       txn,
			ctx:       ctx,
		}

		var err error
		record, ok, err = view.GetRecord(key)
		return err
	})
	err = n.normalizeEngineError("get_record", err)
	n.finishOperation(ctx, start, "get_record", err)
	return record, ok, err
}

func readMetadata(item *badger.Item, now time.Time) Metadata {
	expiresAtUnix := item.ExpiresAt()
	meta := Metadata{
		UserMeta:               item.UserMeta(),
		Version:                item.Version(),
		HasTTL:                 expiresAtUnix > 0,
		DeletedOrExpired:       item.IsDeletedOrExpired(),
		ValueSize:              item.ValueSize(),
		EstimatedSize:          item.EstimatedSize(),
		DiscardEarlierVersions: item.DiscardEarlierVersions(),
	}
	if expiresAtUnix == 0 {
		return meta
	}

	meta.ExpiresAt = time.Unix(int64(expiresAtUnix), 0).UTC()
	if ttl := meta.ExpiresAt.Sub(now); ttl > 0 {
		meta.TTL = ttl
	}
	return meta
}
