package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// DefaultDBFileName is the SQLite filename under app data dir.
	DefaultDBFileName = "app.db"
	// DefaultWALCheckpointInterval controls periodic WAL truncation.
	DefaultWALCheckpointInterval = 24 * time.Hour
	// DefaultSecurityEventRetention controls automatic security event pruning.
	DefaultSecurityEventRetention = 90 * 24 * time.Hour
)

var migrations = []string{
	`
CREATE TABLE IF NOT EXISTS peers (
  device_id           TEXT PRIMARY KEY,
  device_name         TEXT NOT NULL,
  ed25519_public_key  TEXT NOT NULL,
  key_fingerprint     TEXT NOT NULL,
  status              TEXT CHECK(status IN ('online','offline','pending','blocked')) DEFAULT 'pending',
  added_timestamp     INTEGER NOT NULL,
  last_seen_timestamp INTEGER,
  last_known_ip       TEXT,
  last_known_port     INTEGER
);
`,
	`
CREATE TABLE IF NOT EXISTS messages (
  message_id         TEXT PRIMARY KEY,
  from_device_id     TEXT REFERENCES peers(device_id),
  to_device_id       TEXT REFERENCES peers(device_id),
  content            TEXT NOT NULL,
  content_type       TEXT CHECK(content_type IN ('text','image','file')) DEFAULT 'text',
  timestamp_sent     INTEGER NOT NULL,
  timestamp_received INTEGER,
  is_read            INTEGER DEFAULT 0,
  delivery_status    TEXT CHECK(delivery_status IN ('pending','sent','delivered','failed')) DEFAULT 'pending',
  signature          TEXT
);
`,
	`
CREATE TABLE IF NOT EXISTS files (
  file_id            TEXT PRIMARY KEY,
  message_id         TEXT REFERENCES messages(message_id),
  from_device_id     TEXT,
  to_device_id       TEXT,
  filename           TEXT NOT NULL,
  filesize           INTEGER NOT NULL,
  filetype           TEXT,
  stored_path        TEXT NOT NULL,
  checksum           TEXT NOT NULL,
  timestamp_received INTEGER,
  transfer_status    TEXT CHECK(transfer_status IN ('pending','accepted','rejected','complete','failed')) DEFAULT 'pending'
);
`,
	`
CREATE TABLE IF NOT EXISTS seen_message_ids (
  message_id  TEXT PRIMARY KEY,
  received_at INTEGER NOT NULL
);
`,
	`
CREATE INDEX IF NOT EXISTS idx_messages_peer_time
ON messages (to_device_id, delivery_status, timestamp_sent);
`,
	`
CREATE INDEX IF NOT EXISTS idx_seen_message_received_at
ON seen_message_ids (received_at);
`,
	`
CREATE TABLE IF NOT EXISTS key_rotation_events (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  peer_device_id      TEXT NOT NULL REFERENCES peers(device_id) ON DELETE CASCADE,
  old_key_fingerprint TEXT NOT NULL,
  new_key_fingerprint TEXT NOT NULL,
  decision            TEXT NOT NULL CHECK(decision IN ('trusted','rejected')),
  timestamp           INTEGER NOT NULL
);
`,
	`
CREATE INDEX IF NOT EXISTS idx_key_rotation_events_peer_time
ON key_rotation_events (peer_device_id, timestamp DESC, id DESC);
`,
	`
CREATE TABLE IF NOT EXISTS security_events (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  event_type     TEXT NOT NULL,
  peer_device_id TEXT,
  details        TEXT NOT NULL,
  severity       TEXT NOT NULL CHECK(severity IN ('info','warning','critical')),
  timestamp      INTEGER NOT NULL
);
`,
	`
CREATE INDEX IF NOT EXISTS idx_security_events_time
ON security_events (timestamp DESC, id DESC);
`,
	`
CREATE INDEX IF NOT EXISTS idx_security_events_type
ON security_events (event_type, timestamp DESC, id DESC);
`,
	`
CREATE INDEX IF NOT EXISTS idx_security_events_peer
ON security_events (peer_device_id, timestamp DESC, id DESC);
`,
	`
CREATE TABLE IF NOT EXISTS peer_settings (
  peer_device_id      TEXT PRIMARY KEY REFERENCES peers(device_id) ON DELETE CASCADE,
  auto_accept_files   INTEGER NOT NULL DEFAULT 0,
  max_file_size       INTEGER NOT NULL DEFAULT 0,
  download_directory  TEXT NOT NULL DEFAULT '',
  custom_name         TEXT NOT NULL DEFAULT '',
  trust_level         TEXT NOT NULL CHECK(trust_level IN ('normal','trusted')) DEFAULT 'normal'
);
`,
	`
CREATE TABLE IF NOT EXISTS transfer_checkpoints (
  file_id            TEXT NOT NULL,
  direction          TEXT NOT NULL CHECK(direction IN ('send','receive')),
  next_chunk         INTEGER NOT NULL DEFAULT 0,
  bytes_transferred  INTEGER NOT NULL DEFAULT 0,
  temp_path          TEXT NOT NULL DEFAULT '',
  updated_at         INTEGER NOT NULL,
  PRIMARY KEY (file_id, direction)
);
`,
	`
CREATE INDEX IF NOT EXISTS idx_transfer_checkpoints_updated_at
ON transfer_checkpoints (updated_at DESC, file_id, direction);
`,
	`
ALTER TABLE files ADD COLUMN folder_id TEXT;
`,
	`
ALTER TABLE files ADD COLUMN relative_path TEXT;
`,
	`
CREATE TABLE IF NOT EXISTS folder_transfers (
  folder_id        TEXT PRIMARY KEY,
  from_device_id   TEXT,
  to_device_id     TEXT,
  folder_name      TEXT NOT NULL,
  root_path        TEXT NOT NULL,
  total_files      INTEGER NOT NULL,
  total_size       INTEGER NOT NULL,
  transfer_status  TEXT NOT NULL CHECK(transfer_status IN ('pending','accepted','rejected','complete','failed')) DEFAULT 'pending',
  timestamp        INTEGER NOT NULL
);
`,
	`
CREATE INDEX IF NOT EXISTS idx_folder_transfers_peer_time
ON folder_transfers (to_device_id, from_device_id, timestamp DESC, folder_id);
`,
	`
ALTER TABLE peer_settings ADD COLUMN notifications_muted INTEGER NOT NULL DEFAULT 0;
`,
	`
ALTER TABLE peer_settings ADD COLUMN verified INTEGER NOT NULL DEFAULT 0;
`,
	`
CREATE INDEX IF NOT EXISTS idx_messages_peer_content
ON messages (from_device_id, to_device_id, content);
`,
	`
CREATE INDEX IF NOT EXISTS idx_files_peer_status_time
ON files (to_device_id, from_device_id, transfer_status, timestamp_received DESC, file_id);
`,
}

// Store is a thin wrapper around a SQLite connection.
type Store struct {
	db *sql.DB

	walCheckpointInterval  time.Duration
	walCheckpointStop      chan struct{}
	walCheckpointWG        sync.WaitGroup
	securityEventRetention time.Duration
	closeOnce              sync.Once
}

// Open opens (or creates) app.db under the given data directory and runs migrations.
func Open(dataDir string) (*Store, string, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, "", fmt.Errorf("create storage directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, DefaultDBFileName)
	store, err := OpenPath(dbPath)
	if err != nil {
		return nil, "", err
	}

	return store, dbPath, nil
}

// OpenPath opens SQLite at an explicit path and runs schema migrations.
func OpenPath(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_busy_timeout=5000", filepath.ToSlash(dbPath))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}

	store := &Store{
		db:                     db,
		walCheckpointInterval:  DefaultWALCheckpointInterval,
		walCheckpointStop:      make(chan struct{}),
		securityEventRetention: DefaultSecurityEventRetention,
	}
	if err := store.enableWALMode(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.applyMigrations(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.checkpointWAL(); err != nil {
		_ = db.Close()
		return nil, err
	}
	store.startWALCheckpointLoop()

	return store, nil
}

// Close closes the SQLite connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	var closeErr error
	s.closeOnce.Do(func() {
		if s.walCheckpointStop != nil {
			close(s.walCheckpointStop)
			s.walCheckpointWG.Wait()
		}
		closeErr = s.db.Close()
		s.db = nil
	})
	return closeErr
}

func (s *Store) applyMigrations() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version;").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if version >= len(migrations) {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for i := version; i < len(migrations); i++ {
		if _, err := tx.Exec(migrations[i]); err != nil {
			return fmt.Errorf("apply migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d;", i+1)); err != nil {
			return fmt.Errorf("set schema version %d: %w", i+1, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration transaction: %w", err)
	}

	return nil
}

func (s *Store) enableWALMode() error {
	var journalMode string
	if err := s.db.QueryRow("PRAGMA journal_mode=WAL;").Scan(&journalMode); err != nil {
		return fmt.Errorf("enable WAL mode: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("enable WAL mode: unexpected journal mode %q", journalMode)
	}
	return nil
}

func (s *Store) checkpointWAL() error {
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
		return fmt.Errorf("wal checkpoint truncate: %w", err)
	}
	return nil
}

func (s *Store) startWALCheckpointLoop() {
	interval := s.walCheckpointInterval
	if interval <= 0 || s.walCheckpointStop == nil {
		return
	}

	s.walCheckpointWG.Add(1)
	go func() {
		defer s.walCheckpointWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = s.checkpointWAL()
			case <-s.walCheckpointStop:
				return
			}
		}
	}()
}
