package network

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appcrypto "gosend/crypto"
)

func TestRekeyTriggeredByInterval(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID:        "peer-a",
		name:            "Peer A",
		rekeyInterval:   150 * time.Millisecond,
		rekeyAfterBytes: ^uint64(0),
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:        "peer-b",
		name:            "Peer B",
		rekeyInterval:   150 * time.Millisecond,
		rekeyAfterBytes: ^uint64(0),
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	waitForSessionEpochAtLeast(t, a.manager, "peer-b", 1, 5*time.Second)
	waitForSessionEpochAtLeast(t, b.manager, "peer-a", 1, 5*time.Second)
}

func TestRekeyTriggeredByByteThreshold(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID:        "peer-a",
		name:            "Peer A",
		rekeyInterval:   time.Hour,
		rekeyAfterBytes: 2 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:        "peer-b",
		name:            "Peer B",
		rekeyInterval:   time.Hour,
		rekeyAfterBytes: 2 * 1024,
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	payload := strings.Repeat("x", 512)
	for i := 0; i < 12; i++ {
		if _, err := a.manager.SendTextMessage("peer-b", payload); err != nil {
			t.Fatalf("SendTextMessage failed on iteration %d: %v", i, err)
		}
	}

	waitForSessionEpochAtLeast(t, a.manager, "peer-b", 1, 5*time.Second)
	waitForSessionEpochAtLeast(t, b.manager, "peer-a", 1, 5*time.Second)
}

func TestRekeyWithConcurrentFileTransfer(t *testing.T) {
	aFiles := filepath.Join(t.TempDir(), "a-files")
	bFiles := filepath.Join(t.TempDir(), "b-files")
	sourcePath := createFixtureFile(t, t.TempDir(), "rekey-concurrent.bin", 2*1024*1024)

	a := newTestManager(t, testManagerConfig{
		deviceID:        "peer-a",
		name:            "Peer A",
		filesDir:        aFiles,
		fileChunkSize:   64 * 1024,
		rekeyInterval:   time.Hour,
		rekeyAfterBytes: 512 * 1024,
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:        "peer-b",
		name:            "Peer B",
		filesDir:        bFiles,
		fileChunkSize:   64 * 1024,
		rekeyInterval:   time.Hour,
		rekeyAfterBytes: 512 * 1024,
		approveFile: func(FileRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	fileID, err := a.manager.SendFile("peer-b", sourcePath)
	if err != nil {
		t.Fatalf("SendFile failed: %v", err)
	}
	waitForFileStatus(t, a.store, fileID, "complete", 30*time.Second)
	waitForFileStatus(t, b.store, fileID, "complete", 30*time.Second)

	waitForSessionEpochAtLeast(t, a.manager, "peer-b", 1, 5*time.Second)
	waitForSessionEpochAtLeast(t, b.manager, "peer-a", 1, 5*time.Second)
}

func TestInvalidRekeyRequestDropsConnection(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("peer add flow failed: %v", err)
	}

	targetConn := a.manager.getConnection("peer-b")
	if targetConn == nil {
		t.Fatalf("expected connection from A to B")
	}

	conn := b.manager.getConnection("peer-a")
	if conn == nil {
		t.Fatalf("expected connection from B to A")
	}

	_, publicKey, err := appcrypto.GenerateEphemeralX25519KeyPair()
	if err != nil {
		t.Fatalf("generate ephemeral keypair failed: %v", err)
	}
	request := RekeyRequest{
		Type:            TypeRekeyRequest,
		FromDeviceID:    "peer-b",
		Epoch:           1,
		X25519PublicKey: base64.StdEncoding.EncodeToString(publicKey.Bytes()),
		Timestamp:       time.Now().UnixMilli(),
		// Signature intentionally omitted.
	}
	if err := conn.SendMessage(request); err != nil {
		t.Fatalf("send invalid rekey request failed: %v", err)
	}

	select {
	case <-targetConn.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for original connection to close after invalid rekey request")
	}
}

func waitForSessionEpochAtLeast(t *testing.T, manager *PeerManager, peerID string, minEpoch uint64, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn := manager.getConnection(peerID)
		if conn != nil && conn.State() != StateDisconnected && conn.SessionEpoch() >= minEpoch {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	conn := manager.getConnection(peerID)
	if conn == nil {
		t.Fatalf("timed out waiting for rekey epoch >= %d for peer %q: no connection", minEpoch, peerID)
	}
	t.Fatalf("timed out waiting for rekey epoch >= %d for peer %q, got %d", minEpoch, peerID, conn.SessionEpoch())
}

func waitForConnectionClosed(t *testing.T, manager *PeerManager, peerID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn := manager.getConnection(peerID)
		if conn == nil || conn.State() == StateDisconnected {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	conn := manager.getConnection(peerID)
	if conn == nil {
		return
	}
	t.Fatalf("timed out waiting for peer %q connection to close (state=%s)", peerID, conn.State())
}
