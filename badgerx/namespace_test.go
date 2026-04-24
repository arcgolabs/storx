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
