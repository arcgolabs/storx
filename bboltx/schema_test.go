package bboltx_test

import (
	"context"
	"testing"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"go.etcd.io/bbolt"
)

func TestModelSchemaBootstrapAndMigrate(t *testing.T) {
	db := openBbolt(t)
	schema := bboltx.ModelSchema[uint64, seqUser]{
		Name:   "users",
		Keys:   keycodec.Uint64BE(),
		Values: codec.JSON[seqUser](),
		KeyOf:  func(value seqUser) uint64 { return value.ID },
		Indexes: []bboltx.ModelIndexDefinition[uint64, seqUser]{
			bboltx.SecondaryIndexDefinition[uint64, seqUser, string]{
				Name:  "users_by_email",
				Keys:  keycodec.String(),
				KeyOf: func(value seqUser) string { return value.Email },
			},
			bboltx.SecondaryIndexOrderedDefinition[uint64, seqUser, string, string]{
				Name:     "users_by_team_email",
				Keys:     keycodec.String(),
				SortKeys: keycodec.String(),
				KeyOf:    func(value seqUser) string { return value.Name },
				SortOf:   func(value seqUser) string { return value.Email },
			},
		},
		Relations: []bboltx.RelationDefinition{
			{
				Name:         "team_members",
				Kind:         bboltx.RelationKindHasMany,
				TargetModel:  "users",
				LocalIndex:   "users_by_team_email",
				ForeignIndex: "users_by_team_email",
			},
		},
	}
	description := schema.Describe()
	if description.Name != "users" || description.PrimaryKeyType != "uint64" || description.ValueType != "bboltx_test.seqUser" {
		t.Fatalf("unexpected schema description: %#v", description)
	}
	if len(description.Indexes) != 2 || description.Indexes[1].Kind != bboltx.IndexKindOrdered || description.Indexes[1].SortType != "string" {
		t.Fatalf("unexpected schema index description: %#v", description.Indexes)
	}
	if len(description.Relations) != 1 || description.Relations[0].Kind != bboltx.RelationKindHasMany {
		t.Fatalf("unexpected schema relation description: %#v", description.Relations)
	}

	ctx := context.Background()
	if err := schema.Bootstrap(ctx, db); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	if err := db.View(func(tx *bbolt.Tx) error {
		for _, name := range []string{"users", "users_by_email", "users_by_team_email"} {
			if tx.Bucket([]byte(name)) == nil {
				t.Fatalf("expected bucket %q to exist", name)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("view failed: %v", err)
	}

	migrator := bboltx.NewMigrator(db, "users")
	applied := 0
	plan, err := migrator.Plan(ctx,
		bboltx.Migration{Version: 1, Up: func(ctx context.Context, tx *bbolt.Tx) error { return nil }},
		bboltx.Migration{Version: 2, Up: func(ctx context.Context, tx *bbolt.Tx) error { return nil }},
	)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if plan.CurrentVersion != 0 || plan.TargetVersion != 2 || len(plan.Pending) != 2 {
		t.Fatalf("unexpected migration plan: %#v", plan)
	}
	if err := migrator.Migrate(ctx,
		bboltx.Migration{
			Version: 1,
			Up: func(ctx context.Context, tx *bbolt.Tx) error {
				applied++
				_, err := tx.CreateBucketIfNotExists([]byte("migration_one"))
				return err
			},
		},
		bboltx.Migration{
			Version: 2,
			Up: func(ctx context.Context, tx *bbolt.Tx) error {
				applied++
				_, err := tx.CreateBucketIfNotExists([]byte("migration_two"))
				return err
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
		bboltx.Migration{Version: 1, Up: func(ctx context.Context, tx *bbolt.Tx) error { return nil }},
		bboltx.Migration{Version: 2, Up: func(ctx context.Context, tx *bbolt.Tx) error { return nil }},
	)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if dryRun.CurrentVersion != 2 || dryRun.TargetVersion != 2 || len(dryRun.Pending) != 0 {
		t.Fatalf("unexpected migration dry run: %#v", dryRun)
	}

	if err := migrator.Migrate(ctx,
		bboltx.Migration{
			Version: 1,
			Up: func(ctx context.Context, tx *bbolt.Tx) error {
				applied++
				return nil
			},
		},
		bboltx.Migration{
			Version: 2,
			Up: func(ctx context.Context, tx *bbolt.Tx) error {
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
