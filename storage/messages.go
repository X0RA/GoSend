package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// SaveMessage inserts a new message row.
func (s *Store) SaveMessage(message Message) error {
	if message.MessageID == "" {
		return errors.New("message_id is required")
	}
	if message.FromDeviceID == "" {
		return errors.New("from_device_id is required")
	}
	if message.ToDeviceID == "" {
		return errors.New("to_device_id is required")
	}
	if message.Content == "" {
		return errors.New("content is required")
	}
	if message.ContentType == "" {
		message.ContentType = messageContentText
	}
	if err := validateContentType(message.ContentType); err != nil {
		return err
	}
	if message.DeliveryStatus == "" {
		message.DeliveryStatus = deliveryStatusPending
	}
	if err := validateDeliveryStatus(message.DeliveryStatus); err != nil {
		return err
	}
	if message.TimestampSent == 0 {
		message.TimestampSent = nowUnixMilli()
	}

	isRead := 0
	if message.IsRead {
		isRead = 1
	}

	_, err := s.db.Exec(
		`INSERT INTO messages (
			message_id,
			from_device_id,
			to_device_id,
			content,
			content_type,
			timestamp_sent,
			timestamp_received,
			is_read,
			delivery_status,
			signature
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		message.MessageID,
		message.FromDeviceID,
		message.ToDeviceID,
		message.Content,
		message.ContentType,
		message.TimestampSent,
		nullInt64(message.TimestampReceived),
		isRead,
		message.DeliveryStatus,
		message.Signature,
	)
	if err != nil {
		return fmt.Errorf("insert message %q: %w", message.MessageID, err)
	}

	return nil
}

// GetMessages returns conversation messages with one peer ordered by sent timestamp.
func (s *Store) GetMessages(peerID string, limit, offset int) ([]Message, error) {
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.Query(
		`SELECT
			message_id,
			from_device_id,
			to_device_id,
			content,
			content_type,
			timestamp_sent,
			timestamp_received,
			is_read,
			delivery_status,
			signature
		FROM messages
		WHERE from_device_id = ? OR to_device_id = ?
		ORDER BY timestamp_sent ASC
		LIMIT ? OFFSET ?`,
		peerID,
		peerID,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages for peer %q: %w", peerID, err)
	}
	defer rows.Close()

	messages := make([]Message, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message row: %w", err)
		}
		messages = append(messages, *message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate message rows: %w", err)
	}

	return messages, nil
}

// MarkDelivered sets delivery_status to delivered for a message ID.
func (s *Store) MarkDelivered(messageID string) error {
	if messageID == "" {
		return errors.New("message_id is required")
	}

	res, err := s.db.Exec(
		`UPDATE messages
		SET delivery_status = ?
		WHERE message_id = ?`,
		deliveryStatusDelivered,
		messageID,
	)
	if err != nil {
		return fmt.Errorf("mark delivered for message %q: %w", messageID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for mark delivered %q: %w", messageID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// UpdateDeliveryStatus updates delivery_status for a message.
func (s *Store) UpdateDeliveryStatus(messageID, status string) error {
	if messageID == "" {
		return errors.New("message_id is required")
	}
	if err := validateDeliveryStatus(status); err != nil {
		return err
	}

	res, err := s.db.Exec(
		`UPDATE messages
		SET delivery_status = ?
		WHERE message_id = ?`,
		status,
		messageID,
	)
	if err != nil {
		return fmt.Errorf("update delivery status for message %q: %w", messageID, err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read rows affected for update delivery status %q: %w", messageID, err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// GetMessageByID fetches one message by message ID.
func (s *Store) GetMessageByID(messageID string) (*Message, error) {
	if messageID == "" {
		return nil, errors.New("message_id is required")
	}

	row := s.db.QueryRow(
		`SELECT
			message_id,
			from_device_id,
			to_device_id,
			content,
			content_type,
			timestamp_sent,
			timestamp_received,
			is_read,
			delivery_status,
			signature
		FROM messages
		WHERE message_id = ?`,
		messageID,
	)

	message, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get message %q: %w", messageID, err)
	}
	return message, nil
}

// GetPendingMessages returns outbound pending messages for a peer.
func (s *Store) GetPendingMessages(peerID string) ([]Message, error) {
	if peerID == "" {
		return nil, errors.New("peer_id is required")
	}

	rows, err := s.db.Query(
		`SELECT
			message_id,
			from_device_id,
			to_device_id,
			content,
			content_type,
			timestamp_sent,
			timestamp_received,
			is_read,
			delivery_status,
			signature
		FROM messages
		WHERE to_device_id = ? AND delivery_status = ?
		ORDER BY timestamp_sent ASC`,
		peerID,
		deliveryStatusPending,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending messages for peer %q: %w", peerID, err)
	}
	defer rows.Close()

	messages := make([]Message, 0)
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending message row: %w", err)
		}
		messages = append(messages, *message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending message rows: %w", err)
	}

	return messages, nil
}

// PruneExpiredQueue marks pending outbound messages older than cutoff as failed.
func (s *Store) PruneExpiredQueue(cutoffTimestamp int64) (int64, error) {
	if cutoffTimestamp <= 0 {
		return 0, errors.New("cutoff timestamp must be > 0")
	}

	res, err := s.db.Exec(
		`UPDATE messages
		SET delivery_status = ?
		WHERE delivery_status = ? AND timestamp_sent < ?`,
		deliveryStatusFailed,
		deliveryStatusPending,
		cutoffTimestamp,
	)
	if err != nil {
		return 0, fmt.Errorf("prune expired message queue: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read rows affected for prune expired queue: %w", err)
	}

	return rowsAffected, nil
}

func scanMessage(row scanner) (*Message, error) {
	var (
		message           Message
		timestampReceived sql.NullInt64
		isRead            int
		signature         sql.NullString
	)

	if err := row.Scan(
		&message.MessageID,
		&message.FromDeviceID,
		&message.ToDeviceID,
		&message.Content,
		&message.ContentType,
		&message.TimestampSent,
		&timestampReceived,
		&isRead,
		&message.DeliveryStatus,
		&signature,
	); err != nil {
		return nil, err
	}

	message.TimestampReceived = int64Ptr(timestampReceived)
	message.IsRead = isRead == 1
	if signature.Valid {
		message.Signature = signature.String
	}

	return &message, nil
}
