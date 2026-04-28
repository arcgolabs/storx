package bboltx_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"go.etcd.io/bbolt"
)

type user struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestBucketCRUD(t *testing.T) {
	db := openBbolt(t)
	users := bboltx.NewBucket[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := users.Put(ctx, "u1", user{ID: "u1", Name: "alice"}); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	value, ok, err := users.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected value to exist")
	}
	if value.Name != "alice" {
		t.Fatalf("unexpected value: %#v", value)
	}

	exists, err := users.Exists(ctx, "u1")
	if err != nil {
		t.Fatalf("exists failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected key to exist")
	}

	if err := users.Delete(ctx, "u1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, ok, err = users.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get after delete failed: %v", err)
	}
	if ok {
		t.Fatalf("expected value to be deleted")
	}
}

func TestPutManyAndDeleteMany(t *testing.T) {
	db := openBbolt(t)
	users := bboltx.NewBucket[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := users.PutMany(
		ctx,
		bboltx.Entry[string, user]{Key: "u1", Value: user{ID: "u1", Name: "alice"}},
		bboltx.Entry[string, user]{Key: "u2", Value: user{ID: "u2", Name: "bob"}},
	); err != nil {
		t.Fatalf("put many failed: %v", err)
	}

	results, err := users.GetMany(ctx, "u1", "u2")
	if err != nil {
		t.Fatalf("get many failed: %v", err)
	}
	if len(results) != 2 || !results[0].Found || !results[1].Found {
		t.Fatalf("expected batch results to exist, got %#v", results)
	}

	if err := users.DeleteMany(ctx, "u1", "u2"); err != nil {
		t.Fatalf("delete many failed: %v", err)
	}

	results, err = users.GetMany(ctx, "u1", "u2")
	if err != nil {
		t.Fatalf("get many after delete failed: %v", err)
	}
	if results[0].Found || results[1].Found {
		t.Fatalf("expected batch delete to remove values, got %#v", results)
	}
}

func TestRepositorySaveAndGet(t *testing.T) {
	db := openBbolt(t)
	repo := bboltx.NewRepository[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := repo.Save(ctx, "u1", user{ID: "u1", Name: "alice"}); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	value, ok, err := repo.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok || value.Name != "alice" {
		t.Fatalf("unexpected repo value: ok=%v value=%#v", ok, value)
	}
}

func TestMissingBucketAsEmpty(t *testing.T) {
	db := openBbolt(t)
	users := bboltx.NewBucket[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	value, ok, err := users.Get(context.Background(), "missing")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if ok || value != (user{}) {
		t.Fatalf("expected empty result, got ok=%v value=%#v", ok, value)
	}
}

func TestMissingBucketError(t *testing.T) {
	db := openBbolt(t)
	users := bboltx.NewBucket[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
		bboltx.WithReadOnlyMissingAsEmpty(false),
	)

	_, _, err := users.Get(context.Background(), "missing")
	if !errors.Is(err, storx.ErrBucketNotFound) {
		t.Fatalf("expected ErrBucketNotFound, got %v", err)
	}
}

func TestScanPrefixAndCursor(t *testing.T) {
	db := openBbolt(t)
	users := bboltx.NewBucket[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	mustPut(t, users, ctx, "active/u1", user{ID: "u1", Name: "alice"})
	mustPut(t, users, ctx, "active/u2", user{ID: "u2", Name: "bob"})
	mustPut(t, users, ctx, "inactive/u3", user{ID: "u3", Name: "eve"})

	var keys []string
	err := users.View(ctx, func(tx bboltx.ViewTx[string, user]) error {
		if err := tx.ScanPrefix([]byte("active/"), func(key string, value user) error {
			keys = append(keys, key)
			return nil
		}); err != nil {
			return err
		}

		cur := tx.Cursor()
		key, value, ok, err := cur.First()
		if err != nil {
			return err
		}
		if !ok || key != "active/u1" || value.Name != "alice" {
			t.Fatalf("unexpected first cursor result: ok=%v key=%q value=%#v", ok, key, value)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("view failed: %v", err)
	}

	if len(keys) != 2 || keys[0] != "active/u1" || keys[1] != "active/u2" {
		t.Fatalf("unexpected scan result: %#v", keys)
	}
}

func TestContextCanceled(t *testing.T) {
	db := openBbolt(t)
	users := bboltx.NewBucket[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := users.Put(ctx, "u1", user{ID: "u1", Name: "alice"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestBytesCodecReturnsDetachedValue(t *testing.T) {
	db := openBbolt(t)
	values := bboltx.NewBucket[string, []byte](
		db,
		"values",
		keycodec.String(),
		codec.Bytes(),
	)

	ctx := context.Background()
	if err := values.Put(ctx, "k1", []byte("abc")); err != nil {
		t.Fatalf("put failed: %v", err)
	}

	value, ok, err := values.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected value to exist")
	}
	value[0] = 'z'

	value, ok, err = values.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("second get failed: %v", err)
	}
	if !ok || string(value) != "abc" {
		t.Fatalf("expected detached stored value, got %q", string(value))
	}
}

func openBbolt(t *testing.T) *bbolt.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := bbolt.Open(dbPath, 0o600, nil)
	if err != nil {
		t.Fatalf("open bbolt: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func mustPut(t *testing.T, bucket *bboltx.Bucket[string, user], ctx context.Context, key string, value user) {
	t.Helper()
	if err := bucket.Put(ctx, key, value); err != nil {
		t.Fatalf("put %q failed: %v", key, err)
	}
}
