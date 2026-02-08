package storage

import (
	"errors"
	"testing"
)

func TestPeerCRUD(t *testing.T) {
	store := newTestStore(t)

	lastSeen := nowUnixMilli()
	lastIP := "192.168.1.10"
	lastPort := 9999

	peer := Peer{
		DeviceID:          "peer-1",
		DeviceName:        "Alice",
		Ed25519PublicKey:  "base64-ed25519-pubkey",
		KeyFingerprint:    "deadbeefdeadbeefdeadbeefdeadbeef",
		Status:            peerStatusPending,
		AddedTimestamp:    nowUnixMilli(),
		LastSeenTimestamp: &lastSeen,
		LastKnownIP:       &lastIP,
		LastKnownPort:     &lastPort,
	}

	if err := store.AddPeer(peer); err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	got, err := store.GetPeer(peer.DeviceID)
	if err != nil {
		t.Fatalf("GetPeer failed: %v", err)
	}
	if got.DeviceName != peer.DeviceName {
		t.Fatalf("unexpected device name: got %q want %q", got.DeviceName, peer.DeviceName)
	}
	if got.Status != peer.Status {
		t.Fatalf("unexpected status: got %q want %q", got.Status, peer.Status)
	}
	if got.LastSeenTimestamp == nil || *got.LastSeenTimestamp != lastSeen {
		t.Fatalf("unexpected last_seen_timestamp: got %+v", got.LastSeenTimestamp)
	}

	if err := store.AddPeer(Peer{
		DeviceID:         "peer-2",
		DeviceName:       "Bob",
		Ed25519PublicKey: "base64-ed25519-pubkey-2",
		KeyFingerprint:   "00112233445566778899aabbccddeeff",
	}); err != nil {
		t.Fatalf("AddPeer (second peer) failed: %v", err)
	}

	list, err := store.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(list))
	}

	newSeen := nowUnixMilli()
	if err := store.UpdatePeerStatus(peer.DeviceID, peerStatusOnline, newSeen); err != nil {
		t.Fatalf("UpdatePeerStatus failed: %v", err)
	}

	updated, err := store.GetPeer(peer.DeviceID)
	if err != nil {
		t.Fatalf("GetPeer after update failed: %v", err)
	}
	if updated.Status != peerStatusOnline {
		t.Fatalf("expected status %q, got %q", peerStatusOnline, updated.Status)
	}
	if updated.LastSeenTimestamp == nil || *updated.LastSeenTimestamp != newSeen {
		t.Fatalf("unexpected updated last_seen_timestamp: got %+v", updated.LastSeenTimestamp)
	}

	if err := store.UpdatePeerEndpoint(peer.DeviceID, "10.0.0.8", 7777, newSeen+1); err != nil {
		t.Fatalf("UpdatePeerEndpoint failed: %v", err)
	}
	updatedEndpoint, err := store.GetPeer(peer.DeviceID)
	if err != nil {
		t.Fatalf("GetPeer after endpoint update failed: %v", err)
	}
	if updatedEndpoint.LastKnownIP == nil || *updatedEndpoint.LastKnownIP != "10.0.0.8" {
		t.Fatalf("unexpected last_known_ip: %+v", updatedEndpoint.LastKnownIP)
	}
	if updatedEndpoint.LastKnownPort == nil || *updatedEndpoint.LastKnownPort != 7777 {
		t.Fatalf("unexpected last_known_port: %+v", updatedEndpoint.LastKnownPort)
	}

	if err := store.RemovePeer(peer.DeviceID); err != nil {
		t.Fatalf("RemovePeer failed: %v", err)
	}
	_, err = store.GetPeer(peer.DeviceID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after RemovePeer, got %v", err)
	}
}

func TestUpdatePeerIdentityAndKeyRotationEvents(t *testing.T) {
	store := newTestStore(t)

	now := nowUnixMilli()
	if err := store.AddPeer(Peer{
		DeviceID:         "peer-identity",
		DeviceName:       "Identity Peer",
		Ed25519PublicKey: "old-key",
		KeyFingerprint:   "old-fingerprint",
		Status:           peerStatusOnline,
		AddedTimestamp:   now,
	}); err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	if err := store.UpdatePeerIdentity("peer-identity", "new-key", "new-fingerprint"); err != nil {
		t.Fatalf("UpdatePeerIdentity failed: %v", err)
	}

	updated, err := store.GetPeer("peer-identity")
	if err != nil {
		t.Fatalf("GetPeer failed: %v", err)
	}
	if updated.Ed25519PublicKey != "new-key" {
		t.Fatalf("unexpected updated public key: got %q", updated.Ed25519PublicKey)
	}
	if updated.KeyFingerprint != "new-fingerprint" {
		t.Fatalf("unexpected updated fingerprint: got %q", updated.KeyFingerprint)
	}

	if err := store.RecordKeyRotationEvent(KeyRotationEvent{
		PeerDeviceID:      "peer-identity",
		OldKeyFingerprint: "old-fingerprint",
		NewKeyFingerprint: "new-fingerprint",
		Decision:          KeyRotationDecisionTrusted,
		Timestamp:         now + 1,
	}); err != nil {
		t.Fatalf("RecordKeyRotationEvent trusted failed: %v", err)
	}
	if err := store.RecordKeyRotationEvent(KeyRotationEvent{
		PeerDeviceID:      "peer-identity",
		OldKeyFingerprint: "new-fingerprint",
		NewKeyFingerprint: "bad-fingerprint",
		Decision:          KeyRotationDecisionRejected,
		Timestamp:         now + 2,
	}); err != nil {
		t.Fatalf("RecordKeyRotationEvent rejected failed: %v", err)
	}

	events, err := store.GetRecentKeyRotationEvents("peer-identity", 10)
	if err != nil {
		t.Fatalf("GetRecentKeyRotationEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 key rotation events, got %d", len(events))
	}
	if events[0].Decision != KeyRotationDecisionRejected {
		t.Fatalf("expected newest event decision %q, got %q", KeyRotationDecisionRejected, events[0].Decision)
	}
	if events[1].Decision != KeyRotationDecisionTrusted {
		t.Fatalf("expected older event decision %q, got %q", KeyRotationDecisionTrusted, events[1].Decision)
	}
}

func TestPeerSettingsCRUDAndPeerNameUpdate(t *testing.T) {
	store := newTestStore(t)

	now := nowUnixMilli()
	if err := store.AddPeer(Peer{
		DeviceID:         "peer-settings",
		DeviceName:       "Original Name",
		Ed25519PublicKey: "pub-key",
		KeyFingerprint:   "fingerprint",
		Status:           peerStatusOnline,
		AddedTimestamp:   now,
	}); err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	if err := store.EnsurePeerSettingsExist("peer-settings"); err != nil {
		t.Fatalf("EnsurePeerSettingsExist failed: %v", err)
	}
	defaults, err := store.GetPeerSettings("peer-settings")
	if err != nil {
		t.Fatalf("GetPeerSettings defaults failed: %v", err)
	}
	if defaults.AutoAcceptFiles {
		t.Fatalf("expected default auto_accept_files=false")
	}
	if defaults.MaxFileSize != 0 {
		t.Fatalf("expected default max_file_size=0, got %d", defaults.MaxFileSize)
	}
	if defaults.DownloadDirectory != "" {
		t.Fatalf("expected default download_directory empty, got %q", defaults.DownloadDirectory)
	}
	if defaults.CustomName != "" {
		t.Fatalf("expected default custom_name empty, got %q", defaults.CustomName)
	}
	if defaults.TrustLevel != PeerTrustLevelNormal {
		t.Fatalf("expected default trust_level %q, got %q", PeerTrustLevelNormal, defaults.TrustLevel)
	}
	if defaults.NotificationsMuted {
		t.Fatalf("expected default notifications_muted=false")
	}
	if defaults.Verified {
		t.Fatalf("expected default verified=false")
	}

	if err := store.UpdatePeerSettings(PeerSettings{
		PeerDeviceID:       "peer-settings",
		AutoAcceptFiles:    true,
		MaxFileSize:        12345,
		DownloadDirectory:  "/tmp/downloads",
		CustomName:         "Laptop",
		TrustLevel:         PeerTrustLevelTrusted,
		NotificationsMuted: true,
		Verified:           true,
	}); err != nil {
		t.Fatalf("UpdatePeerSettings failed: %v", err)
	}

	updated, err := store.GetPeerSettings("peer-settings")
	if err != nil {
		t.Fatalf("GetPeerSettings updated failed: %v", err)
	}
	if !updated.AutoAcceptFiles {
		t.Fatalf("expected auto_accept_files=true")
	}
	if updated.MaxFileSize != 12345 {
		t.Fatalf("unexpected max_file_size: %d", updated.MaxFileSize)
	}
	if updated.DownloadDirectory != "/tmp/downloads" {
		t.Fatalf("unexpected download_directory: %q", updated.DownloadDirectory)
	}
	if updated.CustomName != "Laptop" {
		t.Fatalf("unexpected custom_name: %q", updated.CustomName)
	}
	if updated.TrustLevel != PeerTrustLevelTrusted {
		t.Fatalf("unexpected trust_level: %q", updated.TrustLevel)
	}
	if !updated.NotificationsMuted {
		t.Fatalf("expected notifications_muted=true")
	}
	if !updated.Verified {
		t.Fatalf("expected verified=true")
	}

	if err := store.ResetPeerVerified("peer-settings"); err != nil {
		t.Fatalf("ResetPeerVerified failed: %v", err)
	}
	resetSettings, err := store.GetPeerSettings("peer-settings")
	if err != nil {
		t.Fatalf("GetPeerSettings after reset failed: %v", err)
	}
	if resetSettings.Verified {
		t.Fatalf("expected verified=false after reset")
	}

	if err := store.UpdatePeerDeviceName("peer-settings", "Changed Name"); err != nil {
		t.Fatalf("UpdatePeerDeviceName failed: %v", err)
	}
	peer, err := store.GetPeer("peer-settings")
	if err != nil {
		t.Fatalf("GetPeer failed: %v", err)
	}
	if peer.DeviceName != "Changed Name" {
		t.Fatalf("unexpected peer device_name: %q", peer.DeviceName)
	}

	if err := store.UpdatePeerIdentity("peer-settings", "rotated-key", "rotated-fingerprint"); err != nil {
		t.Fatalf("UpdatePeerIdentity failed: %v", err)
	}
	afterRotation, err := store.GetPeerSettings("peer-settings")
	if err != nil {
		t.Fatalf("GetPeerSettings after key rotation failed: %v", err)
	}
	if afterRotation.Verified {
		t.Fatalf("expected verified=false after key rotation")
	}
}
