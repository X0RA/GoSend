package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesDatabaseAndAppliesMigrations(t *testing.T) {
	dataDir := t.TempDir()
	store, dbPath, err := Open(dataDir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}()

	if dbPath != filepath.Join(dataDir, DefaultDBFileName) {
		t.Fatalf("unexpected db path: got %q", dbPath)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file not created: %v", err)
	}

	var version int
	if err := store.db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("expected schema version %d, got %d", len(migrations), version)
	}

	var journalMode string
	if err := store.db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected journal_mode wal, got %q", journalMode)
	}

	expectedTables := []string{
		"peers",
		"messages",
		"files",
		"seen_message_ids",
		"key_rotation_events",
		"security_events",
		"peer_settings",
	}
	for _, table := range expectedTables {
		var count int
		if err := store.db.QueryRow(
			"SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name = ?",
			table,
		).Scan(&count); err != nil {
			t.Fatalf("check table %q: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("expected table %q to exist", table)
		}
	}
}
