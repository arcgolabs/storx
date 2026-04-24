// Package main demonstrates basic storx bbolt usage with a DB wrapper and observer.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/arcgolabs/storx/bboltx"
	"github.com/arcgolabs/storx/codec"
	"github.com/arcgolabs/storx/keycodec"
	"github.com/arcgolabs/storx/observer"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("storx-example-%d.db", time.Now().UnixNano()))
	db, err := openExampleDB(dbPath, logger)
	if err != nil {
		panic(err)
	}
	defer cleanupExampleDB(logger, db, dbPath)

	users := bboltx.NewBucketWithDB(
		db,
		"users",
		keycodec.String(),
		codec.JSON[User](),
	)

	err = users.Put(ctx, "u_1001", User{
		ID:   "u_1001",
		Name: "Alice",
	})
	if err != nil {
		panic(err)
	}

	user, ok, err := users.Get(ctx, "u_1001")
	if err != nil {
		panic(err)
	}
	if !ok {
		panic("user not found")
	}

	if err := printLoadedUser(user); err != nil {
		panic(err)
	}

	if err := users.View(ctx, func(tx bboltx.ViewTx[string, User]) error {
		return tx.ScanPrefix([]byte("u_"), printUserLine)
	}); err != nil {
		panic(err)
	}
}

func openExampleDB(path string, logger *slog.Logger) (*bboltx.DB, error) {
	db, err := bboltx.Open(
		path,
		0o600,
		nil,
		bboltx.WithDBLogger(logger),
		bboltx.WithDBObserver(observer.NewSlog(logger, observer.WithSlogMessage("storx event"))),
	)
	if err != nil {
		return nil, fmt.Errorf("open example db: %w", err)
	}
	return db, nil
}

func cleanupExampleDB(logger *slog.Logger, db *bboltx.DB, path string) {
	if closeErr := db.Close(); closeErr != nil {
		logger.Error("close example db", "err", closeErr)
	}
	if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		logger.Error("remove example db", "err", removeErr)
	}
}

func printLoadedUser(user User) error {
	if _, err := fmt.Fprintf(os.Stdout, "loaded user: %+v\n", user); err != nil {
		return fmt.Errorf("write loaded user: %w", err)
	}
	return nil
}

func printUserLine(key string, user User) error {
	if _, err := fmt.Fprintf(os.Stdout, "%s -> %s\n", key, user.Name); err != nil {
		return fmt.Errorf("write user line: %w", err)
	}
	return nil
}
