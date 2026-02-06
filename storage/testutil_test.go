package storage

import (
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	dataDir := t.TempDir()
	store, _, err := Open(dataDir)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close test store: %v", err)
		}
	})

	return store
}

func mustAddPeer(t *testing.T, store *Store, deviceID, name string) {
	t.Helper()

	err := store.AddPeer(Peer{
		DeviceID:         deviceID,
		DeviceName:       name,
		Ed25519PublicKey: "base64-public-key-" + deviceID,
		KeyFingerprint:   "fingerprint-" + deviceID,
		Status:           peerStatusPending,
		AddedTimestamp:   nowUnixMilli(),
	})
	if err != nil {
		t.Fatalf("add peer %q: %v", deviceID, err)
	}
}
