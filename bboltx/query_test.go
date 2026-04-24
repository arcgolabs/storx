package bboltx_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
)

func TestFirstLast(t *testing.T) {
	users := newQueryBucket(t)
	ctx := context.Background()

	first, ok, err := users.First(ctx)
	if err != nil {
		t.Fatalf("first failed: %v", err)
	}
	if !ok || first.Key != "a/1" || first.Value.Name != "alice" {
		t.Fatalf("unexpected first result: ok=%v entry=%#v", ok, first)
	}

	last, ok, err := users.Last(ctx)
	if err != nil {
		t.Fatalf("last failed: %v", err)
	}
	if !ok || last.Key != "b/1" || last.Value.Name != "eve" {
		t.Fatalf("unexpected last result: ok=%v entry=%#v", ok, last)
	}
}

func TestListKeysValues(t *testing.T) {
	users := newQueryBucket(t)
	ctx := context.Background()

	entries, err := users.List(
		ctx,
		bboltx.WithPrefix[string]([]byte("a/")),
		bboltx.WithReverse[string](true),
	)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(entries) != 2 || entries[0].Key != "a/2" || entries[1].Key != "a/1" {
		t.Fatalf("unexpected list result: %#v", entries)
	}

	keys, err := users.Keys(
		ctx,
		bboltx.WithStart("a/2"),
		bboltx.WithEnd("b/1"),
	)
	if err != nil {
		t.Fatalf("keys failed: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"a/2", "b/1"}) {
		t.Fatalf("unexpected keys: %#v", keys)
	}

	values, err := users.Values(
		ctx,
		bboltx.WithPrefix[string]([]byte("a/")),
		bboltx.WithLimit[string](1),
	)
	if err != nil {
		t.Fatalf("values failed: %v", err)
	}
	if len(values) != 1 || values[0].Name != "alice" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestGetMany(t *testing.T) {
	users := newQueryBucket(t)

	results, err := users.GetMany(context.Background(), "a/1", "missing", "b/1")
	if err != nil {
		t.Fatalf("get many failed: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("unexpected result length: %d", len(results))
	}
	if !results[0].Found || results[0].Value.Name != "alice" {
		t.Fatalf("unexpected first lookup: %#v", results[0])
	}
	if results[1].Found {
		t.Fatalf("expected missing lookup, got %#v", results[1])
	}
	if !results[2].Found || results[2].Value.Name != "eve" {
		t.Fatalf("unexpected last lookup: %#v", results[2])
	}
}

func newQueryBucket(t *testing.T) *bboltx.Bucket[string, user] {
	t.Helper()

	db := openBbolt(t)
	users := bboltx.NewBucket[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	mustPut(t, users, ctx, "a/1", user{ID: "u1", Name: "alice"})
	mustPut(t, users, ctx, "a/2", user{ID: "u2", Name: "bob"})
	mustPut(t, users, ctx, "b/1", user{ID: "u3", Name: "eve"})
	return users
}
