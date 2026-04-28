package badgerx_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/arcgolabs/storx/badgerx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/dgraph-io/badger/v4"
)

type user struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestNamespaceCRUD(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := users.Set(ctx, "u1", user{ID: "u1", Name: "alice"}); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	value, ok, err := users.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok || value.Name != "alice" {
		t.Fatalf("unexpected get result: ok=%v value=%#v", ok, value)
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

func TestSetManyAndDeleteMany(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := users.SetMany(ctx, []badgerx.Entry[string, user]{
		{Key: "u1", Value: user{ID: "u1", Name: "alice"}},
		{Key: "u2", Value: user{ID: "u2", Name: "bob"}},
	}); err != nil {
		t.Fatalf("set many failed: %v", err)
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
	db := openBadger(t)
	repo := badgerx.NewRepository[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := repo.Save(ctx, "u1", user{ID: "u1", Name: "alice"}, badgerx.WithMeta(1)); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	value, ok, err := repo.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok || value.Name != "alice" {
		t.Fatalf("unexpected repo value: ok=%v value=%#v", ok, value)
	}

	meta, ok, err := repo.GetMetadata(ctx, "u1")
	if err != nil {
		t.Fatalf("get metadata failed: %v", err)
	}
	if !ok || meta.UserMeta != 1 {
		t.Fatalf("unexpected repo metadata: ok=%v meta=%#v", ok, meta)
	}
}

func TestScanPrefix(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	mustSet(t, users, ctx, "active/u1", user{ID: "u1", Name: "alice"})
	mustSet(t, users, ctx, "active/u2", user{ID: "u2", Name: "bob"})
	mustSet(t, users, ctx, "inactive/u3", user{ID: "u3", Name: "eve"})

	var keys []string
	err := users.View(ctx, func(tx badgerx.ViewTx[string, user]) error {
		return tx.ScanPrefix([]byte("active/"), func(key string, value user) error {
			keys = append(keys, key)
			return nil
		})
	})
	if err != nil {
		t.Fatalf("view failed: %v", err)
	}

	if len(keys) != 2 || keys[0] != "active/u1" || keys[1] != "active/u2" {
		t.Fatalf("unexpected scan result: %#v", keys)
	}
}

func TestTTL(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := users.Set(ctx, "u1", user{ID: "u1", Name: "alice"}, badgerx.WithTTL(50*time.Millisecond)); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	time.Sleep(120 * time.Millisecond)

	_, ok, err := users.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if ok {
		t.Fatalf("expected TTL value to expire")
	}
}

func TestGetRecordAndMetadata(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	err := users.Set(
		ctx,
		"u1",
		user{ID: "u1", Name: "alice"},
		badgerx.WithTTL(time.Hour),
		badgerx.WithMeta(7),
		badgerx.WithDiscard(),
	)
	if err != nil {
		t.Fatalf("set failed: %v", err)
	}

	record, ok, err := users.GetRecord(ctx, "u1")
	if err != nil {
		t.Fatalf("get record failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected record to exist")
	}
	if record.Key != "u1" || record.Value.Name != "alice" {
		t.Fatalf("unexpected record: %#v", record)
	}
	if record.Metadata.UserMeta != 7 {
		t.Fatalf("unexpected user meta: %#v", record.Metadata)
	}
	if !record.Metadata.HasTTL || record.Metadata.ExpiresAt.IsZero() || record.Metadata.TTL <= 0 {
		t.Fatalf("expected ttl metadata, got %#v", record.Metadata)
	}
	if record.Metadata.Version == 0 || record.Metadata.ValueSize == 0 || record.Metadata.EstimatedSize == 0 {
		t.Fatalf("expected populated size/version metadata, got %#v", record.Metadata)
	}
	if !record.Metadata.DiscardEarlierVersions {
		t.Fatalf("expected discard flag metadata, got %#v", record.Metadata)
	}
	if record.Metadata.DeletedOrExpired {
		t.Fatalf("did not expect live record to be deleted/expired")
	}

	meta, ok, err := users.GetMetadata(ctx, "u1")
	if err != nil {
		t.Fatalf("get metadata failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected metadata to exist")
	}
	if meta.UserMeta != record.Metadata.UserMeta || meta.Version != record.Metadata.Version {
		t.Fatalf("expected metadata to match record metadata: meta=%#v record=%#v", meta, record.Metadata)
	}
}

func TestSetManyTTL(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	if err := users.SetMany(ctx, []badgerx.Entry[string, user]{
		{Key: "u1", Value: user{ID: "u1", Name: "alice"}},
		{Key: "u2", Value: user{ID: "u2", Name: "bob"}},
	}, badgerx.WithTTL(50*time.Millisecond)); err != nil {
		t.Fatalf("set many with ttl failed: %v", err)
	}

	time.Sleep(120 * time.Millisecond)

	results, err := users.GetMany(ctx, "u1", "u2")
	if err != nil {
		t.Fatalf("get many failed: %v", err)
	}
	if results[0].Found || results[1].Found {
		t.Fatalf("expected TTL batch values to expire, got %#v", results)
	}
}

func TestContextCanceled(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := users.Set(ctx, "u1", user{ID: "u1", Name: "alice"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestBytesCodecReturnsDetachedValue(t *testing.T) {
	db := openBadger(t)
	values := badgerx.NewNamespace[string, []byte](
		db,
		"values",
		keycodec.String(),
		codec.Bytes(),
	)

	ctx := context.Background()
	if err := values.Set(ctx, "k1", []byte("abc")); err != nil {
		t.Fatalf("set failed: %v", err)
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

func TestRunValueLogGC(t *testing.T) {
	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	if err := users.RunValueLogGC(context.Background(), 0.5); err != nil {
		t.Fatalf("gc failed: %v", err)
	}
}

func openBadger(t *testing.T) *badger.DB {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "badger")
	opts := badger.DefaultOptions(dir).WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("open badger: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func mustSet(t *testing.T, namespace *badgerx.Namespace[string, user], ctx context.Context, key string, value user) {
	t.Helper()
	if err := namespace.Set(ctx, key, value); err != nil {
		t.Fatalf("set %q failed: %v", key, err)
	}
}
