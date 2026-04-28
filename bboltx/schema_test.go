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
