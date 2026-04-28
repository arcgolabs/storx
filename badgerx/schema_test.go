package badgerx_test

import (
	"context"
	"testing"

	"github.com/arcgolabs/storx/badgerx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/dgraph-io/badger/v4"
)

func TestModelSchemaBootstrapAndMigrate(t *testing.T) {
	db := openBadger(t)
	schema := badgerx.ModelSchema[string, indexedUser]{
		Prefix: "users",
		Keys:   keycodec.String(),
		Values: codec.JSON[indexedUser](),
		KeyOf:  func(value indexedUser) string { return value.ID },
		Indexes: []badgerx.ModelIndexDefinition[string, indexedUser]{
			badgerx.SecondaryIndexDefinition[string, indexedUser, string]{
				Prefix: "users_by_email",
				Keys:   keycodec.String(),
				KeyOf:  func(value indexedUser) string { return value.Email },
			},
			badgerx.SecondaryIndexOrderedDefinition[string, indexedUser, string, string]{
				Prefix:   "users_by_team_email",
				Keys:     keycodec.String(),
				SortKeys: keycodec.String(),
				KeyOf:    func(value indexedUser) string { return value.Name },
				SortOf:   func(value indexedUser) string { return value.Email },
			},
		},
		Relations: []badgerx.RelationDefinition{
			{
				Name:         "team_members",
				Kind:         badgerx.RelationKindHasMany,
				TargetModel:  "users",
				LocalIndex:   "users_by_team_email",
				ForeignIndex: "users_by_team_email",
			},
		},
	}
	description := schema.Describe()
	if description.Prefix != "users" || description.PrimaryKeyType != "string" || description.ValueType != "badgerx_test.indexedUser" {
		t.Fatalf("unexpected schema description: %#v", description)
	}
	if len(description.Indexes) != 2 || description.Indexes[1].Kind != badgerx.IndexKindOrdered || description.Indexes[1].SortType != "string" {
		t.Fatalf("unexpected schema index description: %#v", description.Indexes)
	}
	if len(description.Relations) != 1 || description.Relations[0].Kind != badgerx.RelationKindHasMany {
		t.Fatalf("unexpected schema relation description: %#v", description.Relations)
	}

	ctx := context.Background()
	if err := schema.Bootstrap(ctx, db); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	migrator := badgerx.NewMigrator(db, "users")
	applied := 0
	plan, err := migrator.Plan(ctx,
		badgerx.Migration{Version: 1, Up: func(ctx context.Context, txn *badger.Txn) error { return nil }},
		badgerx.Migration{Version: 2, Up: func(ctx context.Context, txn *badger.Txn) error { return nil }},
	)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if plan.CurrentVersion != 0 || plan.TargetVersion != 2 || len(plan.Pending) != 2 {
		t.Fatalf("unexpected migration plan: %#v", plan)
	}
	if err := migrator.Migrate(ctx,
		badgerx.Migration{
			Version: 1,
			Up: func(ctx context.Context, txn *badger.Txn) error {
				applied++
				return txn.Set([]byte("migration/one"), []byte("1"))
			},
		},
		badgerx.Migration{
			Version: 2,
			Up: func(ctx context.Context, txn *badger.Txn) error {
				applied++
				return txn.Set([]byte("migration/two"), []byte("2"))
			},
		},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("current version failed: %v", err)
	}
	if version != 2 || applied != 2 {
		t.Fatalf("unexpected migration state: version=%d applied=%d", version, applied)
	}
	dryRun, err := migrator.DryRun(ctx,
		badgerx.Migration{Version: 1, Up: func(ctx context.Context, txn *badger.Txn) error { return nil }},
		badgerx.Migration{Version: 2, Up: func(ctx context.Context, txn *badger.Txn) error { return nil }},
	)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if dryRun.CurrentVersion != 2 || dryRun.TargetVersion != 2 || len(dryRun.Pending) != 0 {
		t.Fatalf("unexpected migration dry run: %#v", dryRun)
	}

	if err := migrator.Migrate(ctx,
		badgerx.Migration{
			Version: 1,
			Up: func(ctx context.Context, txn *badger.Txn) error {
				applied++
				return nil
			},
		},
		badgerx.Migration{
			Version: 2,
			Up: func(ctx context.Context, txn *badger.Txn) error {
				applied++
				return nil
			},
		},
	); err != nil {
		t.Fatalf("repeat migrate failed: %v", err)
	}
	if applied != 2 {
		t.Fatalf("expected migrations not to re-run, applied=%d", applied)
	}
}
