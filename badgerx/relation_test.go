package badgerx_test

import (
	"context"
	"testing"

	"github.com/arcgolabs/storx/badgerx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
)

type article struct {
	ID          string
	AuthorEmail string
	Team        string
	Title       string
}

type teamLookup struct {
	Name string
}

func TestBelongsToRelationLoadAndPreload(t *testing.T) {
	db := openBadger(t)
	emailIndex := badgerx.NewSecondaryIndex[string, indexedUser, string](
		db,
		"users_by_email",
		keycodec.String(),
		keycodec.String(),
		func(value indexedUser) string { return value.Email },
	)
	userStore := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelIndex[string, indexedUser](emailIndex),
	)

	ctx := context.Background()
	if _, err := userStore.Create(ctx, indexedUser{ID: "u1", Email: "alice@example.com", Name: "alice"}); err != nil {
		t.Fatalf("create first user failed: %v", err)
	}
	if _, err := userStore.Create(ctx, indexedUser{ID: "u2", Email: "bob@example.com", Name: "bob"}); err != nil {
		t.Fatalf("create second user failed: %v", err)
	}

	relation := badgerx.NewBelongsToRelation(
		"author",
		func(post article) string { return post.AuthorEmail },
		emailIndex,
		userStore,
	)

	author, ok, err := relation.LoadRelated(ctx, article{AuthorEmail: "alice@example.com", Title: "hello"})
	if err != nil {
		t.Fatalf("load related failed: %v", err)
	}
	if !ok || author.Email != "alice@example.com" {
		t.Fatalf("unexpected related author: ok=%v value=%#v", ok, author)
	}

	preloaded, err := relation.Preload(
		ctx,
		article{AuthorEmail: "alice@example.com", Title: "p1"},
		article{AuthorEmail: "bob@example.com", Title: "p2"},
		article{AuthorEmail: "missing@example.com", Title: "p3"},
	)
	if err != nil {
		t.Fatalf("preload related failed: %v", err)
	}
	if len(preloaded) != 3 {
		t.Fatalf("unexpected preload length: %d", len(preloaded))
	}
	if !preloaded[0].Found || preloaded[0].Value.Name != "alice" {
		t.Fatalf("unexpected first preload result: %#v", preloaded[0])
	}
	if !preloaded[1].Found || preloaded[1].Value.Name != "bob" {
		t.Fatalf("unexpected second preload result: %#v", preloaded[1])
	}
	if preloaded[2].Found {
		t.Fatalf("expected missing relation, got %#v", preloaded[2])
	}
}

func TestHasManyRelationPreload(t *testing.T) {
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
	userStore := badgerx.NewModelStore[string, indexedUser](
		db,
		"users",
		keycodec.String(),
		codec.JSON[indexedUser](),
		func(value indexedUser) string { return value.ID },
		badgerx.WithModelIndex[string, indexedUser](teamIndex),
	)

	ctx := context.Background()
	if _, err := userStore.Create(ctx, indexedUser{ID: "u1", Email: "c@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create first team member failed: %v", err)
	}
	if _, err := userStore.Create(ctx, indexedUser{ID: "u2", Email: "a@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create second team member failed: %v", err)
	}
	if _, err := userStore.Create(ctx, indexedUser{ID: "u3", Email: "b@example.com", Name: "team-b"}); err != nil {
		t.Fatalf("create third team member failed: %v", err)
	}

	relation := badgerx.NewOrderedHasManyRelation(
		"members",
		func(team teamLookup) string { return team.Name },
		teamIndex,
		userStore,
		false,
	)

	members, err := relation.LoadRelated(ctx, teamLookup{Name: "team-a"})
	if err != nil {
		t.Fatalf("load related many failed: %v", err)
	}
	if len(members) != 2 || members[0].Email != "a@example.com" || members[1].Email != "c@example.com" {
		t.Fatalf("unexpected related members: %#v", members)
	}

	preloaded, err := relation.Preload(
		ctx,
		teamLookup{Name: "team-a"},
		teamLookup{Name: "team-b"},
		teamLookup{Name: "team-missing"},
	)
	if err != nil {
		t.Fatalf("preload related many failed: %v", err)
	}
	if len(preloaded) != 3 {
		t.Fatalf("unexpected preload length: %d", len(preloaded))
	}
	if len(preloaded[0].Values) != 2 || preloaded[0].Values[0].Email != "a@example.com" || preloaded[0].Values[1].Email != "c@example.com" {
		t.Fatalf("unexpected first relation preload: %#v", preloaded[0])
	}
	if len(preloaded[1].Values) != 1 || preloaded[1].Values[0].Email != "b@example.com" {
		t.Fatalf("unexpected second relation preload: %#v", preloaded[1])
	}
	if len(preloaded[2].Values) != 0 {
		t.Fatalf("expected empty missing relation preload, got %#v", preloaded[2])
	}
}
