package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SetSecurityEventRetention configures automatic security-event pruning horizon.
func (s *Store) SetSecurityEventRetention(retention time.Duration) {
	if retention <= 0 {
		retention = DefaultSecurityEventRetention
	}
	s.securityEventRetention = retention
}

// LogSecurityEvent inserts a structured security event and applies retention pruning.
func (s *Store) LogSecurityEvent(event SecurityEvent) error {
	if strings.TrimSpace(event.EventType) == "" {
		return errors.New("event_type is required")
	}
	if event.Severity == "" {
		event.Severity = SecuritySeverityInfo
	}
	if err := validateSecuritySeverity(event.Severity); err != nil {
		return err
	}
	if event.Details == "" {
		event.Details = "{}"
	}
	if !json.Valid([]byte(event.Details)) {
		return errors.New("details must be valid JSON text")
	}
	if event.Timestamp == 0 {
		event.Timestamp = nowUnixMilli()
	}

	var peerDeviceID *string
	if event.PeerDeviceID != nil {
		trimmed := strings.TrimSpace(*event.PeerDeviceID)
		if trimmed != "" {
			peerDeviceID = &trimmed
		}
	}

	_, err := s.db.Exec(
		`INSERT INTO security_events (
			event_type,
			peer_device_id,
			details,
			severity,
			timestamp
		) VALUES (?, ?, ?, ?, ?)`,
		event.EventType,
		nullString(peerDeviceID),
		event.Details,
		event.Severity,
		event.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert security event %q: %w", event.EventType, err)
	}

	if s.securityEventRetention > 0 {
		cutoff := time.Now().Add(-s.securityEventRetention).UnixMilli()
		if _, err := s.PruneSecurityEvents(cutoff); err != nil {
			return fmt.Errorf("prune security events: %w", err)
		}
	}

	return nil
}

// GetSecurityEvents returns recent security events with optional filtering.
func (s *Store) GetSecurityEvents(filter SecurityEventFilter) ([]SecurityEvent, error) {
	if filter.Severity != "" {
		if err := validateSecuritySeverity(filter.Severity); err != nil {
			return nil, err
		}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := strings.Builder{}
	query.WriteString(`SELECT
		id,
		event_type,
		peer_device_id,
		details,
		severity,
		timestamp
	FROM security_events`)

	where := make([]string, 0, 5)
	args := make([]any, 0, 7)

	if filter.EventType != "" {
		where = append(where, "event_type = ?")
		args = append(args, filter.EventType)
	}
	if filter.PeerDeviceID != "" {
		where = append(where, "peer_device_id = ?")
		args = append(args, filter.PeerDeviceID)
	}
	if filter.Severity != "" {
		where = append(where, "severity = ?")
		args = append(args, filter.Severity)
	}
	if filter.FromTimestamp != nil {
		where = append(where, "timestamp >= ?")
		args = append(args, *filter.FromTimestamp)
	}
	if filter.ToTimestamp != nil {
		where = append(where, "timestamp <= ?")
		args = append(args, *filter.ToTimestamp)
	}

	if len(where) > 0 {
		query.WriteString(" WHERE ")
		query.WriteString(strings.Join(where, " AND "))
	}
	query.WriteString(" ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

	rows, err := s.db.Query(query.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("get security events: %w", err)
	}
	defer rows.Close()

	events := make([]SecurityEvent, 0)
	for rows.Next() {
		event, err := scanSecurityEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("scan security event row: %w", err)
		}
		events = append(events, *event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate security event rows: %w", err)
	}

	return events, nil
}

// PruneSecurityEvents removes security events older than cutoffTimestamp.
func (s *Store) PruneSecurityEvents(cutoffTimestamp int64) (int64, error) {
	if cutoffTimestamp <= 0 {
		return 0, errors.New("cutoff timestamp must be > 0")
	}

	res, err := s.db.Exec(`DELETE FROM security_events WHERE timestamp < ?`, cutoffTimestamp)
	if err != nil {
		return 0, fmt.Errorf("prune security events: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read rows affected for security event prune: %w", err)
	}

	return rowsAffected, nil
}

func scanSecurityEvent(row scanner) (*SecurityEvent, error) {
	var (
		event        SecurityEvent
		peerDeviceID sql.NullString
	)
	if err := row.Scan(
		&event.ID,
		&event.EventType,
		&peerDeviceID,
		&event.Details,
		&event.Severity,
		&event.Timestamp,
	); err != nil {
		return nil, err
	}

	event.PeerDeviceID = stringPtr(peerDeviceID)
	return &event, nil
}
