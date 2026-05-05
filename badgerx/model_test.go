package badgerx_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	storx "github.com/arcgolabs/storx"
	"github.com/arcgolabs/storx/badgerx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
)

type indexedUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

func TestModelStoreSecondaryIndex(t *testing.T) {
	db := openBadger(t)
	emailIndexDef := badgerx.SecondaryIndexDefinition[string, indexedUser, string]{
		Prefix: "users_by_email",
		Keys:   keycodec.String(),
		KeyOf:  func(value indexedUser) string { return value.Email },
	}
	schema := badgerx.ModelSchema[string, indexedUser]{
		Prefix: "users",
		Keys:   keycodec.String(),
		Values: codec.JSON[indexedUser](),
		KeyOf:  func(value indexedUser) string { return value.ID },
		Indexes: []badgerx.ModelIndexDefinition[string, indexedUser]{
			emailIndexDef,
		},
	}
	store := schema.Open(db)
	emailIndex := emailIndexDef.Open(db, keycodec.String())

	ctx := context.Background()
	id, err := store.Create(ctx, indexedUser{
		ID:    "u1",
		Email: "alice@example.com",
		Name:  "alice",
	}, badgerx.WithMeta(9))
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if id != "u1" {
		t.Fatalf("unexpected saved id: %q", id)
	}

	record, ok, err := store.GetRecord(ctx, "u1")
	if err != nil {
		t.Fatalf("get record failed: %v", err)
	}
	if !ok || record.Value.Email != "alice@example.com" || record.Metadata.UserMeta != 9 {
		t.Fatalf("unexpected record: ok=%v record=%#v", ok, record)
	}

	byEmail, ok, err := emailIndex.Load(ctx, store, "alice@example.com")
	if err != nil {
		t.Fatalf("load by email failed: %v", err)
	}
	if !ok || byEmail.ID != "u1" {
		t.Fatalf("unexpected indexed load: ok=%v value=%#v", ok, byEmail)
	}
	count, err := emailIndex.CountByIndex(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("count by unique index failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}

	updated := byEmail
	updated.Email = "alice+new@example.com"
	if _, err := store.Update(ctx, updated); err != nil {
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

	_, ok, err = store.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get after unique-index delete failed: %v", err)
	}
	if ok {
		t.Fatalf("expected model to be deleted through unique index")
	}

	if _, err := store.Create(ctx, indexedUser{
		ID:    "u1",
		Email: "alice+new@example.com",
		Name:  "recreated",
	}); err != nil {
		t.Fatalf("recreate after unique-index delete failed: %v", err)
	}

	_, ok, err = emailIndex.Load(ctx, store, "alice+new@example.com")
	if err != nil {
		t.Fatalf("load indexed value after delete failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected recreated index entry to exist")
	}

	if err := store.Delete(ctx, "u1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
}

func TestModelStoreTTLPropagatesToIndex(t *testing.T) {
	db := openBadger(t)

	emailIndexDef := badgerx.SecondaryIndexDefinition[string, indexedUser, string]{
		Prefix: "users_by_email",
		Keys:   keycodec.String(),
		KeyOf:  func(value indexedUser) string { return value.Email },
	}
	schema := badgerx.ModelSchema[string, indexedUser]{
		Prefix:  "users",
		Keys:    keycodec.String(),
		Values:  codec.JSON[indexedUser](),
		KeyOf:   func(value indexedUser) string { return value.ID },
		Indexes: []badgerx.ModelIndexDefinition[string, indexedUser]{emailIndexDef},
		DefaultSetOptions: func(value indexedUser) []badgerx.SetOption {
			if value.Email == "ttl@example.com" {
				return []badgerx.SetOption{badgerx.WithTTL(50 * time.Millisecond)}
			}
			return nil
		},
	}
	store := schema.Open(db)
	emailIndex := emailIndexDef.Open(db, keycodec.String())

	ctx := context.Background()
	if _, err := store.Create(ctx, indexedUser{
		ID:    "u1",
		Email: "ttl@example.com",
		Name:  "ttl",
	}); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	time.Sleep(120 * time.Millisecond)

	_, ok, err := store.Get(ctx, "u1")
	if err != nil {
		t.Fatalf("get after ttl failed: %v", err)
	}
	if ok {
		t.Fatalf("expected ttl model to expire")
	}

	_, ok, err = emailIndex.GetPrimary(ctx, "ttl@example.com")
	if err != nil {
		t.Fatalf("get index primary after ttl failed: %v", err)
	}
	if ok {
		t.Fatalf("expected ttl index entry to expire")
	}
}

func TestModelStoreCreateUpdateSemantics(t *testing.T) {
	db := openBadger(t)
	emailIndex := badgerx.NewSecondaryIndex[string, indexedUser, string](
		db,
		"users_by_email",
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Email },
	)
	store := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelIndex[string, indexedUser](emailIndex),
	)

	ctx := context.Background()
	id, err := store.Create(ctx, indexedUser{ID: "u1", Email: "a@example.com", Name: "a"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if id != "u1" {
		t.Fatalf("unexpected created id: %q", id)
	}

	_, err = store.Create(ctx, indexedUser{ID: "u1", Email: "a@example.com", Name: "dup"})
	if !errors.Is(err, storx.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	_, err = store.Update(ctx, indexedUser{ID: "missing", Email: "missing@example.com", Name: "missing"})
	if !errors.Is(err, storx.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := store.Create(ctx, indexedUser{ID: "u2", Email: "b@example.com", Name: "b"}); err != nil {
		t.Fatalf("second create failed: %v", err)
	}
	if _, err := store.Create(ctx, indexedUser{ID: "u3", Email: "b@example.com", Name: "collision"}); !errors.Is(err, storx.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists for duplicate secondary index, got %v", err)
	}
}

func TestModelStoreNonUniqueIndexListAndDelete(t *testing.T) {
	db := openBadger(t)
	teamIndex := badgerx.NewSecondaryIndexMany[string, indexedUser, string](
		db,
		"users_by_team",
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Name },
	)
	store := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelIndex[string, indexedUser](teamIndex),
	)

	ctx := context.Background()
	if _, err := store.Create(ctx, indexedUser{ID: "u1", Email: "a1@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create first failed: %v", err)
	}
	if _, err := store.Create(ctx, indexedUser{ID: "u2", Email: "a2@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create second failed: %v", err)
	}
	if _, err := store.Create(ctx, indexedUser{ID: "u3", Email: "b1@example.com", Name: "team-b"}); err != nil {
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
	db := openBadger(t)
	store := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
	)

	ctx := context.Background()
	entries, err := store.CreateMany(ctx, []indexedUser{
		{ID: "u1", Email: "u1@example.com", Name: "u1"},
		{ID: "u2", Email: "u2@example.com", Name: "u2"},
	}, badgerx.WithMeta(3))
	if err != nil {
		t.Fatalf("create many failed: %v", err)
	}
	if len(entries) != 2 || entries[0].Key != "u1" || entries[1].Key != "u2" {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	if err := store.DeleteMany(ctx, "u1", "u2"); err != nil {
		t.Fatalf("delete many failed: %v", err)
	}
	for _, key := range []string{"u1", "u2"} {
		_, ok, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("get after delete many failed: %v", err)
		}
		if ok {
			t.Fatalf("expected deleted model for key %q", key)
		}
	}
}

func TestModelStoreRebuildIndexes(t *testing.T) {
	db := openBadger(t)
	emailIndex := badgerx.NewSecondaryIndex[string, indexedUser, string](
		db,
		"users_by_email",
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Email },
	)
	store := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelIndex[string, indexedUser](emailIndex),
	)

	ctx := context.Background()
	if _, err := store.Create(ctx, indexedUser{ID: "u1", Email: "repair@example.com", Name: "repair"}, badgerx.WithTTL(time.Hour)); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := emailIndex.Namespace().Delete(ctx, "repair@example.com"); err != nil {
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
	if !ok || value.ID != "u1" {
		t.Fatalf("expected repaired index entry, got ok=%v value=%#v", ok, value)
	}
}

func TestModelStoreHooks(t *testing.T) {
	db := openBadger(t)
	var beforeOps []string
	var afterOps []string

	store := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelHooks(badgerx.ModelHooks[string, indexedUser]{
			BeforeWrite: func(ctx context.Context, event *badgerx.ModelHookEvent[string, indexedUser]) error {
				beforeOps = append(beforeOps, string(event.Operation))
				event.Value.Name = event.Value.Name + "-before"
				return nil
			},
			AfterWrite: func(ctx context.Context, event badgerx.ModelHookEvent[string, indexedUser]) {
				afterOps = append(afterOps, string(event.Operation))
			},
			BeforeDelete: func(ctx context.Context, event *badgerx.ModelHookEvent[string, indexedUser]) error {
				beforeOps = append(beforeOps, string(event.Operation))
				return nil
			},
			AfterDelete: func(ctx context.Context, event badgerx.ModelHookEvent[string, indexedUser]) {
				afterOps = append(afterOps, string(event.Operation))
			},
		}),
	)

	ctx := context.Background()
	id, err := store.Create(ctx, indexedUser{ID: "u1", Email: "hook@example.com", Name: "hook"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	value, ok, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("get after hook create failed: %v", err)
	}
	if !ok || value.Name != "hook-before" {
		t.Fatalf("expected before hook mutation, got ok=%v value=%#v", ok, value)
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
	db := openBadger(t)
	teamIndex := badgerx.NewSecondaryIndexOrdered[string, indexedUser, string, string](
		db,
		"users_by_team_email",
		keycodec.String(),
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Name },
		func(value indexedUser) string { return value.Email },
	)
	store := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelIndex[string, indexedUser](teamIndex),
	)

	ctx := context.Background()
	if _, err := store.Create(ctx, indexedUser{ID: "u1", Email: "b@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create first failed: %v", err)
	}
	if _, err := store.Create(ctx, indexedUser{ID: "u2", Email: "a@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create second failed: %v", err)
	}
	if _, err := store.Create(ctx, indexedUser{ID: "u3", Email: "c@example.com", Name: "team-b"}); err != nil {
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

	filtered, err := teamIndex.FindByIndex(ctx, store, "team-a", func(value indexedUser) bool {
		return value.Email >= "b@example.com"
	}, 1, false)
	if err != nil {
		t.Fatalf("ordered find by index failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Email != "b@example.com" {
		t.Fatalf("unexpected ordered filtered values: %#v", filtered)
	}
}

func TestModelIndexQueryBuilder(t *testing.T) {
	db := openBadger(t)
	emailIndex := badgerx.NewSecondaryIndex[string, indexedUser, string](
		db,
		"users_by_email",
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Email },
	)
	teamIndex := badgerx.NewSecondaryIndexMany[string, indexedUser, string](
		db,
		"users_by_team",
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Name },
	)
	orderedTeamIndex := badgerx.NewSecondaryIndexOrdered[string, indexedUser, string, string](
		db,
		"users_by_team_email",
		keycodec.String(),
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Name },
		func(value indexedUser) string { return value.Email },
	)
	store := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelIndex[string, indexedUser](emailIndex),
		badgerx.WithModelIndex[string, indexedUser](teamIndex),
		badgerx.WithModelIndex[string, indexedUser](orderedTeamIndex),
	)

	ctx := context.Background()
	if _, err := store.Create(ctx, indexedUser{ID: "u1", Email: "b@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create first failed: %v", err)
	}
	if _, err := store.Create(ctx, indexedUser{ID: "u2", Email: "a@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create second failed: %v", err)
	}
	if _, err := store.Create(ctx, indexedUser{ID: "u3", Email: "c@example.com", Name: "team-b"}); err != nil {
		t.Fatalf("create third failed: %v", err)
	}

	value, ok, err := emailIndex.Query(store, "a@example.com").First(ctx)
	if err != nil {
		t.Fatalf("unique query failed: %v", err)
	}
	if !ok || value.Email != "a@example.com" {
		t.Fatalf("unexpected unique query result: ok=%v value=%#v", ok, value)
	}
	count, err := emailIndex.Query(store, "a@example.com").Where(func(value indexedUser) bool {
		return value.Name == "team-a"
	}).Count(ctx)
	if err != nil {
		t.Fatalf("unique query count failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected unique query count 1, got %d", count)
	}

	values, err := teamIndex.Query(store, "team-a").Where(func(value indexedUser) bool {
		return value.Email >= "b@example.com"
	}).Limit(1).Find(ctx)
	if err != nil {
		t.Fatalf("many query failed: %v", err)
	}
	if len(values) != 1 || values[0].Email != "b@example.com" {
		t.Fatalf("unexpected many query values: %#v", values)
	}
	valueList, err := teamIndex.Query(store, "team-a").ValueList(ctx)
	if err != nil {
		t.Fatalf("many query value list failed: %v", err)
	}
	if valueList.Len() != 2 || !valueList.AnyMatch(func(_ int, value indexedUser) bool {
		return value.Email == "a@example.com"
	}) {
		t.Fatalf("unexpected collectionx value list: %#v", valueList.Values())
	}

	page, err := orderedTeamIndex.Query(store, "team-a").Reverse().Page(ctx, "", 1)
	if err != nil {
		t.Fatalf("ordered query page failed: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Value.Email != "b@example.com" {
		t.Fatalf("unexpected ordered query page: %#v", page)
	}
	entryList, err := orderedTeamIndex.Query(store, "team-a").Reverse().EntryList(ctx)
	if err != nil {
		t.Fatalf("ordered query entry list failed: %v", err)
	}
	firstEntry, ok := entryList.GetFirst()
	if !ok || firstEntry.Value.Email != "b@example.com" {
		t.Fatalf("unexpected collectionx entry list first entry: ok=%v entry=%#v", ok, firstEntry)
	}

	if err := teamIndex.Query(store, "team-a").Where(func(value indexedUser) bool {
		return value.Email == "a@example.com"
	}).Delete(ctx); err != nil {
		t.Fatalf("filtered delete failed: %v", err)
	}
	count, err = teamIndex.Query(store, "team-a").Count(ctx)
	if err != nil {
		t.Fatalf("count after filtered delete failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one team-a user after filtered delete, got %d", count)
	}
}
