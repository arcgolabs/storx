package bboltx

import (
	"context"

	"go.etcd.io/bbolt"
)

func (i *SecondaryIndex[K, V, IK]) bootstrapIndex(ctx context.Context, tx *bbolt.Tx) error {
	_ = ctx
	if i == nil || i.bucket == nil {
		return nil
	}
	_, err := tx.CreateBucketIfNotExists(i.bucket.nameBytes)
	return err
}

func (i *SecondaryIndexMany[K, V, IK]) bootstrapIndex(ctx context.Context, tx *bbolt.Tx) error {
	_ = ctx
	if i == nil || i.bucket == nil {
		return nil
	}
	_, err := tx.CreateBucketIfNotExists(i.bucket.nameBytes)
	return err
}
