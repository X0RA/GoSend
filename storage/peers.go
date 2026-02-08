package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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

// UpdatePeerEndpoint updates last known endpoint fields and optional last seen timestamp.
func (s *Store) UpdatePeerEndpoint(deviceID, ip string, port int, lastSeenTimestamp int64) error {
	if deviceID == "" {
		return errors.New("device_id is required")
	}
	if strings.TrimSpace(ip) == "" {
		return errors.New("ip is required")
	}
	if port <= 0 {
		return errors.New("port must be > 0")
	}

	res, err := s.db.Exec(
		`UPDATE peers
		SET last_known_ip = ?,
		    last_known_port = ?,
		    last_seen_timestamp = CASE
				WHEN ? > 0 THEN ?
				ELSE last_seen_timestamp
			END
		WHERE device_id = ?`,
		ip,
		port,
		lastSeenTimestamp,
		lastSeenTimestamp,
		deviceID,
	)
	if err != nil {
		return fmt.Errorf("update peer endpoint %q: %w", deviceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for update peer endpoint %q: %w", deviceID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdatePeerDeviceName updates the stored peer device name.
func (s *Store) UpdatePeerDeviceName(deviceID, deviceName string) error {
	if deviceID == "" {
		return errors.New("device_id is required")
	}
	if strings.TrimSpace(deviceName) == "" {
		return errors.New("device_name is required")
	}

	res, err := s.db.Exec(
		`UPDATE peers
		SET device_name = ?
		WHERE device_id = ?`,
		deviceName,
		deviceID,
	)
	if err != nil {
		return fmt.Errorf("update peer device name %q: %w", deviceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for update peer device name %q: %w", deviceID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdatePeerIdentity updates the pinned Ed25519 public key and fingerprint for a peer.
func (s *Store) UpdatePeerIdentity(deviceID, ed25519PublicKey, keyFingerprint string) error {
	if deviceID == "" {
		return errors.New("device_id is required")
	}
	if ed25519PublicKey == "" {
		return errors.New("ed25519_public_key is required")
	}
	if keyFingerprint == "" {
		return errors.New("key_fingerprint is required")
	}

	res, err := s.db.Exec(
		`UPDATE peers
		SET ed25519_public_key = ?,
		    key_fingerprint = ?
		WHERE device_id = ?`,
		ed25519PublicKey,
		keyFingerprint,
		deviceID,
	)
	if err != nil {
		return fmt.Errorf("update peer identity %q: %w", deviceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for update peer identity %q: %w", deviceID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	// A key change invalidates any previous out-of-band fingerprint verification.
	if _, err := s.db.Exec(
		`UPDATE peer_settings
		SET verified = 0
		WHERE peer_device_id = ?`,
		deviceID,
	); err != nil {
		return fmt.Errorf("reset peer verified flag %q: %w", deviceID, err)
	}

	return nil
}

// EnsurePeerSettingsExist creates a default peer_settings row when absent.
func (s *Store) EnsurePeerSettingsExist(peerDeviceID string) error {
	if peerDeviceID == "" {
		return errors.New("peer_device_id is required")
	}

	_, err := s.db.Exec(
		`INSERT INTO peer_settings (
			peer_device_id,
			auto_accept_files,
			max_file_size,
			download_directory,
			custom_name,
			trust_level,
			notifications_muted,
			verified
		) VALUES (?, 0, 0, '', '', ?, 0, 0)
		ON CONFLICT(peer_device_id) DO NOTHING`,
		peerDeviceID,
		PeerTrustLevelNormal,
	)
	if err != nil {
		return fmt.Errorf("ensure peer settings for %q: %w", peerDeviceID, err)
	}

	return nil
}

// GetPeerSettings fetches one peer settings row.
func (s *Store) GetPeerSettings(peerDeviceID string) (*PeerSettings, error) {
	if peerDeviceID == "" {
		return nil, errors.New("peer_device_id is required")
	}

	row := s.db.QueryRow(
		`SELECT
			peer_device_id,
			auto_accept_files,
			max_file_size,
			download_directory,
			custom_name,
			trust_level,
			notifications_muted,
			verified
		FROM peer_settings
		WHERE peer_device_id = ?`,
		peerDeviceID,
	)

	settings, err := scanPeerSettings(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get peer settings for %q: %w", peerDeviceID, err)
	}

	return settings, nil
}

// UpdatePeerSettings updates one peer settings row.
func (s *Store) UpdatePeerSettings(settings PeerSettings) error {
	if settings.PeerDeviceID == "" {
		return errors.New("peer_device_id is required")
	}
	if settings.MaxFileSize < 0 {
		return errors.New("max_file_size must be >= 0")
	}
	if settings.TrustLevel == "" {
		settings.TrustLevel = PeerTrustLevelNormal
	}
	if err := validatePeerTrustLevel(settings.TrustLevel); err != nil {
		return err
	}

	if err := s.EnsurePeerSettingsExist(settings.PeerDeviceID); err != nil {
		return err
	}

	autoAccept := 0
	if settings.AutoAcceptFiles {
		autoAccept = 1
	}
	notificationsMuted := 0
	if settings.NotificationsMuted {
		notificationsMuted = 1
	}
	verified := 0
	if settings.Verified {
		verified = 1
	}

	res, err := s.db.Exec(
		`UPDATE peer_settings
		SET auto_accept_files = ?,
		    max_file_size = ?,
		    download_directory = ?,
		    custom_name = ?,
		    trust_level = ?,
		    notifications_muted = ?,
		    verified = ?
		WHERE peer_device_id = ?`,
		autoAccept,
		settings.MaxFileSize,
		settings.DownloadDirectory,
		settings.CustomName,
		settings.TrustLevel,
		notificationsMuted,
		verified,
		settings.PeerDeviceID,
	)
	if err != nil {
		return fmt.Errorf("update peer settings for %q: %w", settings.PeerDeviceID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for update peer settings %q: %w", settings.PeerDeviceID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// ResetPeerVerified clears the verified flag for one peer.
func (s *Store) ResetPeerVerified(peerDeviceID string) error {
	if peerDeviceID == "" {
		return errors.New("peer_device_id is required")
	}
	if err := s.EnsurePeerSettingsExist(peerDeviceID); err != nil {
		return err
	}

	res, err := s.db.Exec(
		`UPDATE peer_settings
		SET verified = 0
		WHERE peer_device_id = ?`,
		peerDeviceID,
	)
	if err != nil {
		return fmt.Errorf("reset peer verified for %q: %w", peerDeviceID, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for reset peer verified %q: %w", peerDeviceID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordKeyRotationEvent persists one trusted/rejected key-change decision.
func (s *Store) RecordKeyRotationEvent(event KeyRotationEvent) error {
	if event.PeerDeviceID == "" {
		return errors.New("peer_device_id is required")
	}
	if event.OldKeyFingerprint == "" {
		return errors.New("old_key_fingerprint is required")
	}
	if event.NewKeyFingerprint == "" {
		return errors.New("new_key_fingerprint is required")
	}
	if err := validateKeyRotationDecision(event.Decision); err != nil {
		return err
	}
	if event.Timestamp == 0 {
		event.Timestamp = nowUnixMilli()
	}

	_, err := s.db.Exec(
		`INSERT INTO key_rotation_events (
			peer_device_id,
			old_key_fingerprint,
			new_key_fingerprint,
			decision,
			timestamp
		) VALUES (?, ?, ?, ?, ?)`,
		event.PeerDeviceID,
		event.OldKeyFingerprint,
		event.NewKeyFingerprint,
		event.Decision,
		event.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert key rotation event for peer %q: %w", event.PeerDeviceID, err)
	}

	return nil
}

// GetRecentKeyRotationEvents returns key-rotation history for one peer, newest first.
func (s *Store) GetRecentKeyRotationEvents(peerDeviceID string, limit int) ([]KeyRotationEvent, error) {
	if peerDeviceID == "" {
		return nil, errors.New("peer_device_id is required")
	}
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(
		`SELECT
			id,
			peer_device_id,
			old_key_fingerprint,
			new_key_fingerprint,
			decision,
			timestamp
		FROM key_rotation_events
		WHERE peer_device_id = ?
		ORDER BY timestamp DESC, id DESC
		LIMIT ?`,
		peerDeviceID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get key rotation events for peer %q: %w", peerDeviceID, err)
	}
	defer rows.Close()

	events := make([]KeyRotationEvent, 0)
	for rows.Next() {
		event, err := scanKeyRotationEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan key rotation event row: %w", err)
		}
		events = append(events, *event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate key rotation event rows: %w", err)
	}

	return events, nil
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

func scanKeyRotationEvent(row scanner) (*KeyRotationEvent, error) {
	var event KeyRotationEvent
	if err := row.Scan(
		&event.ID,
		&event.PeerDeviceID,
		&event.OldKeyFingerprint,
		&event.NewKeyFingerprint,
		&event.Decision,
		&event.Timestamp,
	); err != nil {
		return nil, err
	}
	return &event, nil
}

func scanPeerSettings(row scanner) (*PeerSettings, error) {
	var (
		settings           PeerSettings
		autoAccept         int
		notificationsMuted int
		verified           int
	)
	if err := row.Scan(
		&settings.PeerDeviceID,
		&autoAccept,
		&settings.MaxFileSize,
		&settings.DownloadDirectory,
		&settings.CustomName,
		&settings.TrustLevel,
		&notificationsMuted,
		&verified,
	); err != nil {
		return nil, err
	}

	settings.AutoAcceptFiles = autoAccept == 1
	settings.NotificationsMuted = notificationsMuted == 1
	settings.Verified = verified == 1
	return &settings, nil
}
