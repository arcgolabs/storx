package observer_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/arcgolabs/storx/observer"
)

func TestNewSlog(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	obs := observer.NewSlog(
		logger,
		observer.WithSlogLevel(slog.LevelWarn),
		observer.WithSlogMessage("db operation"),
	)

	obs.Observe(context.Background(), observer.Event{
		Engine:     "bbolt",
		Target:     "users",
		TargetType: "bucket",
		Operation:  "get",
		StartedAt:  time.Unix(0, 0).UTC(),
		Duration:   10 * time.Millisecond,
		Err:        errors.New("boom"),
	})

	output := buf.String()
	checkContains(t, output, `"msg":"db operation"`)
	checkContains(t, output, `"level":"WARN"`)
	checkContains(t, output, `"engine":"bbolt"`)
	checkContains(t, output, `"target_type":"bucket"`)
	checkContains(t, output, `"target":"users"`)
	checkContains(t, output, `"op":"get"`)
	checkContains(t, output, `"status":"error"`)
	checkContains(t, output, `"err":"boom"`)
}

func checkContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Fatalf("expected output %q to contain %q", output, want)
	}
}
