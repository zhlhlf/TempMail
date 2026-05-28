package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tempmail/store"
)

func TestHasAdminAccount(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := store.New(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer func() {
		s.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	}()

	if !hasAdminAccount(dbPath) {
		t.Fatal("hasAdminAccount = false, want true after initial seed")
	}

	emptyPath := filepath.Join(dir, "missing.db")
	if hasAdminAccount(emptyPath) {
		t.Fatal("hasAdminAccount = true for missing db, want false")
	}
}
