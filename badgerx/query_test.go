package badgerx_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/arcgolabs/storx/badgerx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
)

func TestFirstLast(t *testing.T) {
	users := newQueryNamespace(t)
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
	users := newQueryNamespace(t)
	ctx := context.Background()

	entries, err := users.List(
		ctx,
		badgerx.WithPrefix[string]([]byte("a/")),
		badgerx.WithReverse[string](true),
	)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(entries) != 2 || entries[0].Key != "a/2" || entries[1].Key != "a/1" {
		t.Fatalf("unexpected list result: %#v", entries)
	}

	keys, err := users.Keys(
		ctx,
		badgerx.WithStart("a/2"),
		badgerx.WithEnd("b/1"),
	)
	if err != nil {
		t.Fatalf("keys failed: %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"a/2", "b/1"}) {
		t.Fatalf("unexpected keys: %#v", keys)
	}

	values, err := users.Values(
		ctx,
		badgerx.WithPrefix[string]([]byte("a/")),
		badgerx.WithLimit[string](1),
	)
	if err != nil {
		t.Fatalf("values failed: %v", err)
	}
	if len(values) != 1 || values[0].Name != "alice" {
		t.Fatalf("unexpected values: %#v", values)
	}

	entryList, err := users.EntryList(ctx, badgerx.WithPrefix[string]([]byte("a/")))
	if err != nil {
		t.Fatalf("entry list failed: %v", err)
	}
	if entryList.Len() != 2 {
		t.Fatalf("unexpected entry list length: %d", entryList.Len())
	}
	keyList, err := users.KeyList(ctx, badgerx.WithStart("a/2"), badgerx.WithEnd("b/1"))
	if err != nil {
		t.Fatalf("key list failed: %v", err)
	}
	if !keyList.AnyMatch(func(_ int, key string) bool { return key == "b/1" }) {
		t.Fatalf("expected key list to include b/1: %#v", keyList.Values())
	}
	valueList, err := users.ValueList(ctx, badgerx.WithPrefix[string]([]byte("a/")))
	if err != nil {
		t.Fatalf("value list failed: %v", err)
	}
	firstValue, ok := valueList.GetFirst()
	if !ok || firstValue.Name != "alice" {
		t.Fatalf("unexpected first collection value: ok=%v value=%#v", ok, firstValue)
	}
}

func TestGetMany(t *testing.T) {
	users := newQueryNamespace(t)

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

func TestIterAndWalk(t *testing.T) {
	users := newQueryNamespace(t)
	ctx := context.Background()

	iter, err := users.Iter(
		ctx,
		badgerx.WithPrefix[string]([]byte("a/")),
		badgerx.WithReverse[string](true),
	)
	if err != nil {
		t.Fatalf("iter failed: %v", err)
	}
	defer func() {
		if closeErr := iter.Close(); closeErr != nil {
			t.Fatalf("close iter failed: %v", closeErr)
		}
	}()

	var iterKeys []string
	for {
		entry, ok, err := iter.Next()
		if err != nil {
			t.Fatalf("iter next failed: %v", err)
		}
		if !ok {
			break
		}
		iterKeys = append(iterKeys, entry.Key)
	}
	if !reflect.DeepEqual(iterKeys, []string{"a/2", "a/1"}) {
		t.Fatalf("unexpected iter keys: %#v", iterKeys)
	}

	var walkKeys []string
	err = users.Walk(
		ctx,
		func(entry badgerx.Entry[string, user]) error {
			walkKeys = append(walkKeys, entry.Key)
			return nil
		},
		badgerx.WithStart("a/2"),
		badgerx.WithEnd("b/1"),
	)
	if err != nil {
		t.Fatalf("walk failed: %v", err)
	}
	if !reflect.DeepEqual(walkKeys, []string{"a/2", "b/1"}) {
		t.Fatalf("unexpected walk keys: %#v", walkKeys)
	}
}

func TestPage(t *testing.T) {
	users := newQueryNamespace(t)
	ctx := context.Background()

	firstPage, err := users.Page(ctx, "", badgerx.WithLimit[string](1))
	if err != nil {
		t.Fatalf("first page failed: %v", err)
	}
	if !firstPage.HasMore || firstPage.NextCursor == "" {
		t.Fatalf("expected first page to have next cursor: %#v", firstPage)
	}
	if len(firstPage.Entries) != 1 || firstPage.Entries[0].Key != "a/1" {
		t.Fatalf("unexpected first page: %#v", firstPage.Entries)
	}

	secondPage, err := users.Page(ctx, firstPage.NextCursor, badgerx.WithLimit[string](1))
	if err != nil {
		t.Fatalf("second page failed: %v", err)
	}
	if len(secondPage.Entries) != 1 || secondPage.Entries[0].Key != "a/2" {
		t.Fatalf("unexpected second page: %#v", secondPage.Entries)
	}

	reversePage, err := users.Page(
		ctx,
		"",
		badgerx.WithPrefix[string]([]byte("a/")),
		badgerx.WithLimit[string](1),
		badgerx.WithReverse[string](true),
	)
	if err != nil {
		t.Fatalf("reverse page failed: %v", err)
	}
	if len(reversePage.Entries) != 1 || reversePage.Entries[0].Key != "a/2" {
		t.Fatalf("unexpected reverse page: %#v", reversePage.Entries)
	}
}

func newQueryNamespace(t *testing.T) *badgerx.Namespace[string, user] {
	t.Helper()

	db := openBadger(t)
	users := badgerx.NewNamespace[string, user](
		db,
		"users",
		keycodec.String(),
		codec.JSON[user](),
	)

	ctx := context.Background()
	mustSet(t, users, ctx, "a/1", user{ID: "u1", Name: "alice"})
	mustSet(t, users, ctx, "a/2", user{ID: "u2", Name: "bob"})
	mustSet(t, users, ctx, "b/1", user{ID: "u3", Name: "eve"})
	return users
}
