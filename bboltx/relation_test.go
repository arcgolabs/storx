package bboltx_test

import (
	"context"
	"testing"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
)

type blogPost struct {
	ID          uint64
	AuthorEmail string
	Team        string
	Title       string
}

type teamRef struct {
	Name string
}

func TestBelongsToRelationLoadAndPreload(t *testing.T) {
	db := openBbolt(t)
	emailIndex := bboltx.NewSecondaryIndex[uint64, seqUser, string](
		db,
		"users_by_email",
		keycodec.String(),
		keycodec.Uint64BE(),
		func(value seqUser) string { return value.Email },
	)
	userStore := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) { target.ID = id }),
		bboltx.WithModelIndex[uint64, seqUser](emailIndex),
	)

	ctx := context.Background()
	if _, _, err := userStore.Create(ctx, seqUser{Email: "alice@example.com", Name: "alice"}); err != nil {
		t.Fatalf("create first user failed: %v", err)
	}
	if _, _, err := userStore.Create(ctx, seqUser{Email: "bob@example.com", Name: "bob"}); err != nil {
		t.Fatalf("create second user failed: %v", err)
	}

	relation := bboltx.NewBelongsToRelation(
		"author",
		func(post blogPost) string { return post.AuthorEmail },
		emailIndex,
		userStore,
	)

	post := blogPost{AuthorEmail: "alice@example.com", Title: "hello"}
	author, ok, err := relation.LoadRelated(ctx, post)
	if err != nil {
		t.Fatalf("load related failed: %v", err)
	}
	if !ok || author.Email != "alice@example.com" {
		t.Fatalf("unexpected related author: ok=%v value=%#v", ok, author)
	}

	preloaded, err := relation.Preload(
		ctx,
		blogPost{AuthorEmail: "alice@example.com", Title: "p1"},
		blogPost{AuthorEmail: "bob@example.com", Title: "p2"},
		blogPost{AuthorEmail: "missing@example.com", Title: "p3"},
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
	userStore := bboltx.NewModelStore[uint64, seqUser](
		db,
		"users",
		keycodec.Uint64BE(),
		codec.JSON[seqUser](),
		func(value seqUser) uint64 { return value.ID },
		bboltx.WithUint64SequenceField(func(target *seqUser, id uint64) { target.ID = id }),
		bboltx.WithModelIndex[uint64, seqUser](teamIndex),
	)

	ctx := context.Background()
	if _, _, err := userStore.Create(ctx, seqUser{Email: "c@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create first team member failed: %v", err)
	}
	if _, _, err := userStore.Create(ctx, seqUser{Email: "a@example.com", Name: "team-a"}); err != nil {
		t.Fatalf("create second team member failed: %v", err)
	}
	if _, _, err := userStore.Create(ctx, seqUser{Email: "b@example.com", Name: "team-b"}); err != nil {
		t.Fatalf("create third team member failed: %v", err)
	}

	relation := bboltx.NewOrderedHasManyRelation(
		"members",
		func(team teamRef) string { return team.Name },
		teamIndex,
		userStore,
		false,
	)

	members, err := relation.LoadRelated(ctx, teamRef{Name: "team-a"})
	if err != nil {
		t.Fatalf("load related many failed: %v", err)
	}
	if len(members) != 2 || members[0].Email != "a@example.com" || members[1].Email != "c@example.com" {
		t.Fatalf("unexpected related members: %#v", members)
	}

	preloaded, err := relation.Preload(
		ctx,
		teamRef{Name: "team-a"},
		teamRef{Name: "team-b"},
		teamRef{Name: "team-missing"},
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
