package storage

import (
	"errors"
	"fmt"
)

// InsertSeenID records a message ID used for replay protection.
func (s *Store) InsertSeenID(messageID string, receivedAt int64) error {
	if messageID == "" {
		return errors.New("message_id is required")
	}
	if receivedAt == 0 {
		receivedAt = nowUnixMilli()
	}

	_, err := s.db.Exec(
		`INSERT INTO seen_message_ids (message_id, received_at)
		VALUES (?, ?)
		ON CONFLICT(message_id) DO UPDATE SET received_at = excluded.received_at`,
		messageID,
		receivedAt,
	)
	if err != nil {
		return fmt.Errorf("insert seen message ID %q: %w", messageID, err)
	}

	return nil
}

// HasSeenID returns true if a message ID has already been seen.
func (s *Store) HasSeenID(messageID string) (bool, error) {
	if messageID == "" {
		return false, errors.New("message_id is required")
	}

	var exists int
	if err := s.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM seen_message_ids WHERE message_id = ?)`,
		messageID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("check seen message ID %q: %w", messageID, err)
	}

	return exists == 1, nil
}

// PruneOldEntries removes seen_message_ids rows older than cutoff timestamp.
func (s *Store) PruneOldEntries(cutoffTimestamp int64) (int64, error) {
	if cutoffTimestamp <= 0 {
		return 0, errors.New("cutoff timestamp must be > 0")
	}

	res, err := s.db.Exec(`DELETE FROM seen_message_ids WHERE received_at < ?`, cutoffTimestamp)
	if err != nil {
		return 0, fmt.Errorf("prune seen message IDs: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read rows affected for seen ID prune: %w", err)
	}

	return rowsAffected, nil
}
