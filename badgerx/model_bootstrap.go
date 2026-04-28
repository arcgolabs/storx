package badgerx

import (
	"context"

	"github.com/dgraph-io/badger/v4"
)

func (i *SecondaryIndex[K, V, IK]) bootstrapIndex(ctx context.Context, txn *badger.Txn) error {
	_ = ctx
	_ = txn
	return nil
}

func (i *SecondaryIndexMany[K, V, IK]) bootstrapIndex(ctx context.Context, txn *badger.Txn) error {
	_ = ctx
	_ = txn
	return nil
}
