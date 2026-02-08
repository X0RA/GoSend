package storage

import (
	"testing"
	"time"
)

func TestLogAndQuerySecurityEvents(t *testing.T) {
	store := newTestStore(t)
	mustAddPeer(t, store, "peer-security", "Security Peer")

	now := nowUnixMilli()
	peerID := "peer-security"

	if err := store.LogSecurityEvent(SecurityEvent{
		EventType:    "replay_rejected",
		PeerDeviceID: &peerID,
		Details:      `{"message_id":"msg-1"}`,
		Severity:     SecuritySeverityWarning,
		Timestamp:    now - 1_000,
	}); err != nil {
		t.Fatalf("LogSecurityEvent replay failed: %v", err)
	}
	if err := store.LogSecurityEvent(SecurityEvent{
		EventType:    "signature_verification_failed",
		PeerDeviceID: &peerID,
		Details:      `{"message_id":"msg-2","message_type":"ack"}`,
		Severity:     SecuritySeverityCritical,
		Timestamp:    now,
	}); err != nil {
		t.Fatalf("LogSecurityEvent signature failed: %v", err)
	}

	all, err := store.GetSecurityEvents(SecurityEventFilter{
		PeerDeviceID: peerID,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("GetSecurityEvents all failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 security events, got %d", len(all))
	}
	if all[0].EventType != "signature_verification_failed" {
		t.Fatalf("expected newest event type signature_verification_failed, got %q", all[0].EventType)
	}
	if all[1].EventType != "replay_rejected" {
		t.Fatalf("expected older event type replay_rejected, got %q", all[1].EventType)
	}

	filtered, err := store.GetSecurityEvents(SecurityEventFilter{
		EventType:    "replay_rejected",
		PeerDeviceID: peerID,
		Severity:     SecuritySeverityWarning,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("GetSecurityEvents filtered failed: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered security event, got %d", len(filtered))
	}
	if filtered[0].Details != `{"message_id":"msg-1"}` {
		t.Fatalf("unexpected filtered event details: %q", filtered[0].Details)
	}
}

func TestSecurityEventRetentionPrunesOldRows(t *testing.T) {
	store := newTestStore(t)
	store.SetSecurityEventRetention(1 * time.Second)

	now := nowUnixMilli()

	if err := store.LogSecurityEvent(SecurityEvent{
		EventType: "old_event",
		Details:   `{"state":"old"}`,
		Severity:  SecuritySeverityInfo,
		Timestamp: now - 10_000,
	}); err != nil {
		t.Fatalf("LogSecurityEvent old_event failed: %v", err)
	}
	if err := store.LogSecurityEvent(SecurityEvent{
		EventType: "new_event",
		Details:   `{"state":"new"}`,
		Severity:  SecuritySeverityInfo,
		Timestamp: now,
	}); err != nil {
		t.Fatalf("LogSecurityEvent new_event failed: %v", err)
	}

	events, err := store.GetSecurityEvents(SecurityEventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("GetSecurityEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event after retention prune, got %d", len(events))
	}
	if events[0].EventType != "new_event" {
		t.Fatalf("expected retained event type new_event, got %q", events[0].EventType)
	}
}
