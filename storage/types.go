package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrNotFound indicates a requested row does not exist.
	ErrNotFound = errors.New("storage: record not found")
)

const (
	peerStatusOnline  = "online"
	peerStatusOffline = "offline"
	peerStatusPending = "pending"
	peerStatusBlocked = "blocked"
)

const (
	messageContentText  = "text"
	messageContentImage = "image"
	messageContentFile  = "file"
)

const (
	deliveryStatusPending   = "pending"
	deliveryStatusSent      = "sent"
	deliveryStatusDelivered = "delivered"
	deliveryStatusFailed    = "failed"
)

const (
	transferStatusPending  = "pending"
	transferStatusAccepted = "accepted"
	transferStatusRejected = "rejected"
	transferStatusComplete = "complete"
	transferStatusFailed   = "failed"
)

const (
	// KeyRotationDecisionTrusted means a presented replacement key was accepted.
	KeyRotationDecisionTrusted = "trusted"
	// KeyRotationDecisionRejected means a presented replacement key was rejected.
	KeyRotationDecisionRejected = "rejected"
)

const (
	// SecuritySeverityInfo indicates informational security event context.
	SecuritySeverityInfo = "info"
	// SecuritySeverityWarning indicates potentially suspicious behavior.
	SecuritySeverityWarning = "warning"
	// SecuritySeverityCritical indicates serious security failures.
	SecuritySeverityCritical = "critical"
)

const (
	// PeerTrustLevelNormal is the default trust level for known peers.
	PeerTrustLevelNormal = "normal"
	// PeerTrustLevelTrusted marks peers explicitly trusted by the user.
	PeerTrustLevelTrusted = "trusted"
)

// Peer is the SQLite representation of a known remote device.
type Peer struct {
	DeviceID          string
	DeviceName        string
	Ed25519PublicKey  string
	KeyFingerprint    string
	Status            string
	AddedTimestamp    int64
	LastSeenTimestamp *int64
	LastKnownIP       *string
	LastKnownPort     *int
}

// Message is the SQLite representation of a chat message.
type Message struct {
	MessageID         string
	FromDeviceID      string
	ToDeviceID        string
	Content           string
	ContentType       string
	TimestampSent     int64
	TimestampReceived *int64
	IsRead            bool
	DeliveryStatus    string
	Signature         string
}

// FileMetadata is the SQLite representation of file transfer metadata.
type FileMetadata struct {
	FileID            string
	MessageID         string
	FromDeviceID      string
	ToDeviceID        string
	Filename          string
	Filesize          int64
	Filetype          string
	StoredPath        string
	Checksum          string
	TimestampReceived *int64
	TransferStatus    string
}

// KeyRotationEvent tracks one trust/reject decision for a peer key change.
type KeyRotationEvent struct {
	ID                int64
	PeerDeviceID      string
	OldKeyFingerprint string
	NewKeyFingerprint string
	Decision          string
	Timestamp         int64
}

// SecurityEvent stores structured security-relevant runtime events.
type SecurityEvent struct {
	ID           int64
	EventType    string
	PeerDeviceID *string
	Details      string
	Severity     string
	Timestamp    int64
}

// SecurityEventFilter narrows GetSecurityEvents query results.
type SecurityEventFilter struct {
	EventType     string
	PeerDeviceID  string
	Severity      string
	FromTimestamp *int64
	ToTimestamp   *int64
	Limit         int
	Offset        int
}

// PeerSettings stores user-controlled behavior for one known peer.
type PeerSettings struct {
	PeerDeviceID      string
	AutoAcceptFiles   bool
	MaxFileSize       int64
	DownloadDirectory string
	CustomName        string
	TrustLevel        string
}

func validatePeerStatus(status string) error {
	switch status {
	case peerStatusOnline, peerStatusOffline, peerStatusPending, peerStatusBlocked:
		return nil
	default:
		return fmt.Errorf("invalid peer status %q", status)
	}
}

func validateContentType(contentType string) error {
	switch contentType {
	case messageContentText, messageContentImage, messageContentFile:
		return nil
	default:
		return fmt.Errorf("invalid content type %q", contentType)
	}
}

func validateDeliveryStatus(status string) error {
	switch status {
	case deliveryStatusPending, deliveryStatusSent, deliveryStatusDelivered, deliveryStatusFailed:
		return nil
	default:
		return fmt.Errorf("invalid delivery status %q", status)
	}
}

func validateTransferStatus(status string) error {
	switch status {
	case transferStatusPending, transferStatusAccepted, transferStatusRejected, transferStatusComplete, transferStatusFailed:
		return nil
	default:
		return fmt.Errorf("invalid transfer status %q", status)
	}
}

func validateKeyRotationDecision(decision string) error {
	switch decision {
	case KeyRotationDecisionTrusted, KeyRotationDecisionRejected:
		return nil
	default:
		return fmt.Errorf("invalid key rotation decision %q", decision)
	}
}

func validateSecuritySeverity(severity string) error {
	switch severity {
	case SecuritySeverityInfo, SecuritySeverityWarning, SecuritySeverityCritical:
		return nil
	default:
		return fmt.Errorf("invalid security event severity %q", severity)
	}
}

func validatePeerTrustLevel(trustLevel string) error {
	switch trustLevel {
	case PeerTrustLevelNormal, PeerTrustLevelTrusted:
		return nil
	default:
		return fmt.Errorf("invalid peer trust level %q", trustLevel)
	}
}

func nullString(ptr *string) sql.NullString {
	if ptr == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *ptr, Valid: true}
}

func nullInt64(ptr *int64) sql.NullInt64 {
	if ptr == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *ptr, Valid: true}
}

func nullInt64FromInt(ptr *int) sql.NullInt64 {
	if ptr == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*ptr), Valid: true}
}

func stringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	v := ns.String
	return &v
}

func int64Ptr(ni sql.NullInt64) *int64 {
	if !ni.Valid {
		return nil
	}
	v := ni.Int64
	return &v
}

func intPtrFromNullInt64(ni sql.NullInt64) *int {
	if !ni.Valid {
		return nil
	}
	v := int(ni.Int64)
	return &v
}

func nowUnixMilli() int64 {
	return time.Now().UnixMilli()
}
