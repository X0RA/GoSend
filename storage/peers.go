package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// AddPeer inserts a new peer row.
func (s *Store) AddPeer(peer Peer) error {
	if peer.DeviceID == "" {
		return errors.New("device_id is required")
	}
	if peer.DeviceName == "" {
		return errors.New("device_name is required")
	}
	if peer.Ed25519PublicKey == "" {
		return errors.New("ed25519_public_key is required")
	}
	if peer.KeyFingerprint == "" {
		return errors.New("key_fingerprint is required")
	}
	if peer.Status == "" {
		peer.Status = peerStatusPending
	}
	if err := validatePeerStatus(peer.Status); err != nil {
		return err
	}
	if peer.AddedTimestamp == 0 {
		peer.AddedTimestamp = nowUnixMilli()
	}

	_, err := s.db.Exec(
		`INSERT INTO peers (
			device_id,
			device_name,
			ed25519_public_key,
			key_fingerprint,
			status,
			added_timestamp,
			last_seen_timestamp,
			last_known_ip,
			last_known_port
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		peer.DeviceID,
		peer.DeviceName,
		peer.Ed25519PublicKey,
		peer.KeyFingerprint,
		peer.Status,
		peer.AddedTimestamp,
		nullInt64(peer.LastSeenTimestamp),
		nullString(peer.LastKnownIP),
		nullInt64FromInt(peer.LastKnownPort),
	)
	if err != nil {
		return fmt.Errorf("insert peer %q: %w", peer.DeviceID, err)
	}

	return nil
}

// GetPeer fetches a peer by device ID.
func (s *Store) GetPeer(deviceID string) (*Peer, error) {
	row := s.db.QueryRow(
		`SELECT
			device_id,
			device_name,
			ed25519_public_key,
			key_fingerprint,
			status,
			added_timestamp,
			last_seen_timestamp,
			last_known_ip,
			last_known_port
		FROM peers
		WHERE device_id = ?`,
		deviceID,
	)

	peer, err := scanPeer(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get peer %q: %w", deviceID, err)
	}

	return peer, nil
}

// ListPeers returns all peers sorted by device name.
func (s *Store) ListPeers() ([]Peer, error) {
	rows, err := s.db.Query(
		`SELECT
			device_id,
			device_name,
			ed25519_public_key,
			key_fingerprint,
			status,
			added_timestamp,
			last_seen_timestamp,
			last_known_ip,
			last_known_port
		FROM peers
		ORDER BY device_name, device_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}
	defer rows.Close()

	peers := make([]Peer, 0)
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, fmt.Errorf("scan peer row: %w", err)
		}
		peers = append(peers, *peer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer rows: %w", err)
	}

	return peers, nil
}

// UpdatePeerStatus updates status and optionally last seen timestamp (when > 0).
func (s *Store) UpdatePeerStatus(deviceID, status string, lastSeenTimestamp int64) error {
	if deviceID == "" {
		return errors.New("device_id is required")
	}
	if err := validatePeerStatus(status); err != nil {
		return err
	}

	res, err := s.db.Exec(
		`UPDATE peers
		SET status = ?,
		    last_seen_timestamp = CASE
				WHEN ? > 0 THEN ?
				ELSE last_seen_timestamp
			END
		WHERE device_id = ?`,
		status,
		lastSeenTimestamp,
		lastSeenTimestamp,
		deviceID,
	)
	if err != nil {
		return fmt.Errorf("update peer status %q: %w", deviceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for peer status update %q: %w", deviceID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// RemovePeer deletes a peer by device ID.
func (s *Store) RemovePeer(deviceID string) error {
	if deviceID == "" {
		return errors.New("device_id is required")
	}

	res, err := s.db.Exec(`DELETE FROM peers WHERE device_id = ?`, deviceID)
	if err != nil {
		return fmt.Errorf("remove peer %q: %w", deviceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for remove peer %q: %w", deviceID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanPeer(row scanner) (*Peer, error) {
	var (
		peer          Peer
		lastSeen      sql.NullInt64
		lastKnownIP   sql.NullString
		lastKnownPort sql.NullInt64
	)

	if err := row.Scan(
		&peer.DeviceID,
		&peer.DeviceName,
		&peer.Ed25519PublicKey,
		&peer.KeyFingerprint,
		&peer.Status,
		&peer.AddedTimestamp,
		&lastSeen,
		&lastKnownIP,
		&lastKnownPort,
	); err != nil {
		return nil, err
	}

	peer.LastSeenTimestamp = int64Ptr(lastSeen)
	peer.LastKnownIP = stringPtr(lastKnownIP)
	peer.LastKnownPort = intPtrFromNullInt64(lastKnownPort)

	return &peer, nil
}
