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

	if err := store.RemovePeer(peer.DeviceID); err != nil {
		t.Fatalf("RemovePeer failed: %v", err)
	}
	_, err = store.GetPeer(peer.DeviceID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after RemovePeer, got %v", err)
	}
}
