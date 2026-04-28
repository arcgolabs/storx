package bboltx_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
)

type seqUser struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

func TestModelStoreSequenceAndSecondaryIndex(t *testing.T) {
	db := openBbolt(t)
	emailIndexDef := bboltx.SecondaryIndexDefinition[uint64, seqUser, string]{
		Name:  "users_by_email",
		Keys:  keycodec.String(),
		KeyOf: func(value seqUser) string { return value.Email },
	}
	schema := bboltx.ModelSchema[uint64, seqUser]{
		Name:   "users",
		Keys:   keycodec.Uint64BE(),
		Values: codec.JSON[seqUser](),
		KeyOf:  func(value seqUser) uint64 { return value.ID },
		SequenceAssigner: func(seq uint64, value *seqUser) uint64 {
			value.ID = seq
			return seq
		},
		Indexes: []bboltx.ModelIndexDefinition[uint64, seqUser]{
			emailIndexDef,
		},
	}
	store := schema.Open(db)
	emailIndex := emailIndexDef.Open(db, keycodec.Uint64BE())

	ctx := context.Background()
	saved, id, err := store.Create(ctx, seqUser{
		Email: "alice@example.com",
		Name:  "alice",
	})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if id == 0 || saved.ID != id {
		t.Fatalf("expected assigned sequence id, got id=%d saved=%#v", id, saved)
	}

	loaded, ok, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok || loaded.Email != "alice@example.com" {
		t.Fatalf("unexpected loaded model: ok=%v value=%#v", ok, loaded)
	}

	byEmail, ok, err := emailIndex.Load(ctx, store, "alice@example.com")
	if err != nil {
		t.Fatalf("load by email failed: %v", err)
	}
	if !ok || byEmail.ID != id {
		t.Fatalf("unexpected indexed load: ok=%v value=%#v", ok, byEmail)
	}
	count, err := emailIndex.CountByIndex(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("count by unique index failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	updated := loaded
	updated.Email = "alice+new@example.com"
	if _, _, err := store.Update(ctx, updated); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	_, ok, err = emailIndex.Load(ctx, store, "alice@example.com")
	if err != nil {
		t.Fatalf("load old email failed: %v", err)
	}
	if ok {
		t.Fatalf("expected old email index entry to be removed")
	}

	byEmail, ok, err = emailIndex.Load(ctx, store, "alice+new@example.com")
	if err != nil {
		t.Fatalf("load new email failed: %v", err)
	}
	if !ok || byEmail.Email != "alice+new@example.com" {
		t.Fatalf("unexpected new email indexed load: ok=%v value=%#v", ok, byEmail)
	}

	if err := emailIndex.Delete(ctx, store, "alice+new@example.com"); err != nil {
		t.Fatalf("delete by unique index failed: %v", err)
	}

	_, ok, err = store.Get(ctx, id)
	if err != nil {
		t.Fatalf("get after unique-index delete failed: %v", err)
	}
	if ok {
		t.Fatalf("expected model to be deleted through unique index")
	}

	recreated, id, err := store.Create(ctx, seqUser{
		Email: "alice+new@example.com",
		Name:  "recreated",
	})
	if err != nil {
		t.Fatalf("recreate after unique-index delete failed: %v", err)
	}
	if recreated.ID != id {
		t.Fatalf("unexpected recreated model: %#v", recreated)
	}

	_, ok, err = emailIndex.Load(ctx, store, "alice+new@example.com")
	if err != nil {
		t.Fatalf("load indexed value after delete failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected recreated index entry to exist")
	}

	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestModelStoreCreateUpdateSemantics(t *testing.T) {
	db := openBbolt(t)
	emailIndex := bboltx.NewSecondaryIndex[uint64, seqUser, string](
		db,
		"users_by_email",
		keycodec.String(),
		keycodec.Uint64BE(),
		func(value seqUser) string { return value.Email },
	)
	store := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) {
			target.ID = id
		}),
		bboltx.WithModelIndex[uint64, seqUser](emailIndex),
	)

	ctx := context.Background()
	saved, id, err := store.Create(ctx, seqUser{Email: "a@example.com", Name: "a"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	_, _, err = store.Create(ctx, saved)
	if !errors.Is(err, storx.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	_, _, err = store.Update(ctx, seqUser{ID: id + 100, Email: "missing@example.com", Name: "missing"})
	if !errors.Is(err, storx.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	_, _, err = store.Create(ctx, seqUser{Email: "b@example.com", Name: "b"})
	if err != nil {
		t.Fatalf("second create failed: %v", err)
	}

	_, _, err = store.Create(ctx, seqUser{Email: "b@example.com", Name: "collision"})
	if !errors.Is(err, storx.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists for duplicate secondary index, got %v", err)
	}
}

func TestModelStoreNonUniqueIndexListAndDelete(t *testing.T) {
	db := openBbolt(t)
	teamIndex := bboltx.NewSecondaryIndexMany[uint64, seqUser, string](
		db,
		"users_by_team",
		keycodec.String(),
		keycodec.Uint64BE(),
		func(value seqUser) string { return value.Name },
	)
	store := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) { target.ID = id }),
		bboltx.WithModelIndex[uint64, seqUser](teamIndex),
	)

	ctx := context.Background()
	if _, _, err := store.Create(ctx, seqUser{Email: "a1@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create first failed: %v", err)
	}
	if _, _, err := store.Create(ctx, seqUser{Email: "a2@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create second failed: %v", err)
	}
	if _, _, err := store.Create(ctx, seqUser{Email: "b1@example.com", Name: "team-b"}); err != nil {
		t.Fatalf("create third failed: %v", err)
	}

	primaries, err := teamIndex.ListPrimaries(ctx, "team-a")
	if err != nil {
		t.Fatalf("list primaries failed: %v", err)
	}
	if len(primaries) != 2 {
		t.Fatalf("expected 2 primaries, got %#v", primaries)
	}
	count, err := teamIndex.CountByIndex(ctx, "team-a")
	if err != nil {
		t.Fatalf("count by non-unique index failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	values, err := teamIndex.List(ctx, store, "team-a")
	if err != nil {
		t.Fatalf("list values failed: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %#v", values)
	}

	if err := teamIndex.Delete(ctx, store, "team-a"); err != nil {
		t.Fatalf("delete by non-unique index failed: %v", err)
	}

	values, err = teamIndex.List(ctx, store, "team-a")
	if err != nil {
		t.Fatalf("list after delete failed: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("expected no values after non-unique index delete, got %#v", values)
	}

	values, err = teamIndex.List(ctx, store, "team-b")
	if err != nil {
		t.Fatalf("list unrelated values failed: %v", err)
	}
	if len(values) != 1 || values[0].Email != "b1@example.com" {
		t.Fatalf("expected unrelated value to remain, got %#v", values)
	}
}

func TestModelStoreCreateManyAndDeleteMany(t *testing.T) {
	db := openBbolt(t)
	store := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) { target.ID = id }),
	)

	ctx := context.Background()
	entries, err := store.CreateMany(ctx, []seqUser{
		{Email: "u1@example.com", Name: "u1"},
		{Email: "u2@example.com", Name: "u2"},
	})
	if err != nil {
		t.Fatalf("create many failed: %v", err)
	}
	if len(entries) != 2 || entries[0].Key == 0 || entries[1].Key == 0 {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	if err := store.DeleteMany(ctx, entries[0].Key, entries[1].Key); err != nil {
		t.Fatalf("delete many failed: %v", err)
	}
	for _, entry := range entries {
		_, ok, err := store.Get(ctx, entry.Key)
		if err != nil {
			t.Fatalf("get after delete many failed: %v", err)
		}
		if ok {
			t.Fatalf("expected deleted model for key %d", entry.Key)
		}
	}
}

func TestModelStoreRebuildIndexes(t *testing.T) {
	db := openBbolt(t)
	emailIndex := bboltx.NewSecondaryIndex[uint64, seqUser, string](
		db,
		"users_by_email",
		keycodec.String(),
		keycodec.Uint64BE(),
		func(value seqUser) string { return value.Email },
	)
	store := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) { target.ID = id }),
		bboltx.WithModelIndex[uint64, seqUser](emailIndex),
	)

	ctx := context.Background()
	_, id, err := store.Create(ctx, seqUser{Email: "repair@example.com", Name: "repair"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := emailIndex.Bucket().Delete(ctx, "repair@example.com"); err != nil {
		t.Fatalf("delete raw index failed: %v", err)
	}
	if _, ok, err := emailIndex.Load(ctx, store, "repair@example.com"); err != nil {
		t.Fatalf("load broken index failed: %v", err)
	} else if ok {
		t.Fatalf("expected broken index lookup to miss")
	}

	if err := store.RebuildIndexes(ctx); err != nil {
		t.Fatalf("rebuild indexes failed: %v", err)
	}

	value, ok, err := emailIndex.Load(ctx, store, "repair@example.com")
	if err != nil {
		t.Fatalf("load repaired index failed: %v", err)
	}
	if !ok || value.ID != id {
		t.Fatalf("expected repaired index entry, got ok=%v value=%#v", ok, value)
	}
}

func TestModelStoreHooks(t *testing.T) {
	db := openBbolt(t)
	var beforeOps []string
	var afterOps []string

	store := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) { target.ID = id }),
		bboltx.WithModelHooks(bboltx.ModelHooks[uint64, seqUser]{
			BeforeWrite: func(ctx context.Context, event *bboltx.ModelHookEvent[uint64, seqUser]) error {
				beforeOps = append(beforeOps, string(event.Operation))
				event.Value.Name = event.Value.Name + "-before"
				return nil
			},
			AfterWrite: func(ctx context.Context, event bboltx.ModelHookEvent[uint64, seqUser]) {
				afterOps = append(afterOps, string(event.Operation))
			},
			BeforeDelete: func(ctx context.Context, event *bboltx.ModelHookEvent[uint64, seqUser]) error {
				beforeOps = append(beforeOps, string(event.Operation))
				return nil
			},
			AfterDelete: func(ctx context.Context, event bboltx.ModelHookEvent[uint64, seqUser]) {
				afterOps = append(afterOps, string(event.Operation))
			},
		}),
	)

	ctx := context.Background()
	saved, id, err := store.Create(ctx, seqUser{Email: "hook@example.com", Name: "hook"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if saved.Name != "hook-before" {
		t.Fatalf("expected before hook mutation, got %#v", saved)
	}
	if err := store.Delete(ctx, id); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if !reflect.DeepEqual(beforeOps, []string{"create", "delete"}) {
		t.Fatalf("unexpected before ops: %#v", beforeOps)
	}
	if !reflect.DeepEqual(afterOps, []string{"create", "delete"}) {
		t.Fatalf("unexpected after ops: %#v", afterOps)
	}
}

func TestModelStoreOrderedIndexAndPage(t *testing.T) {
	db := openBbolt(t)
	teamIndex := bboltx.NewSecondaryIndexOrdered[uint64, seqUser, string, string](
		db,
		"users_by_team_email",
		keycodec.String(),
		keycodec.String(),
		keycodec.Uint64BE(),
		func(value seqUser) string { return value.Name },
		func(value seqUser) string { return value.Email },
	)
	store := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) { target.ID = id }),
		bboltx.WithModelIndex[uint64, seqUser](teamIndex),
	)

	ctx := context.Background()
	if _, _, err := store.Create(ctx, seqUser{Email: "b@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create first failed: %v", err)
	}
	if _, _, err := store.Create(ctx, seqUser{Email: "a@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create second failed: %v", err)
	}
	if _, _, err := store.Create(ctx, seqUser{Email: "c@example.com", Name: "team-b"}); err != nil {
		t.Fatalf("create third failed: %v", err)
	}

	values, err := teamIndex.List(ctx, store, "team-a", false)
	if err != nil {
		t.Fatalf("ordered list failed: %v", err)
	}
	if len(values) != 2 || values[0].Email != "a@example.com" || values[1].Email != "b@example.com" {
		t.Fatalf("unexpected ordered values: %#v", values)
	}

	page, err := teamIndex.Page(ctx, store, "team-a", "", 1, false)
	if err != nil {
		t.Fatalf("ordered page failed: %v", err)
	}
	if !page.HasMore || page.NextCursor == "" || len(page.Entries) != 1 || page.Entries[0].Value.Email != "a@example.com" {
		t.Fatalf("unexpected first ordered page: %#v", page)
	}

	nextPage, err := teamIndex.Page(ctx, store, "team-a", page.NextCursor, 1, false)
	if err != nil {
		t.Fatalf("ordered next page failed: %v", err)
	}
	if len(nextPage.Entries) != 1 || nextPage.Entries[0].Value.Email != "b@example.com" {
		t.Fatalf("unexpected second ordered page: %#v", nextPage)
	}
}
