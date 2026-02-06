package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gosend/storage"
)

func TestPeerAddFlowWithApprovalQueue(t *testing.T) {
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

	connA, err := a.manager.Connect(b.addr())
	if err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if connA.PeerDeviceID() != "peer-b" {
		t.Fatalf("expected peer-b ID, got %q", connA.PeerDeviceID())
	}

	type result struct {
		accepted bool
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		accepted, err := a.manager.SendPeerAddRequest("peer-b")
		resultCh <- result{accepted: accepted, err: err}
	}()

	select {
	case req := <-b.manager.PendingAddRequests():
		if req.PeerDeviceID != "peer-a" {
			t.Fatalf("unexpected pending add request peer ID: %q", req.PeerDeviceID)
		}
		if err := b.manager.ApprovePeerAdd(req.PeerDeviceID, true); err != nil {
			t.Fatalf("ApprovePeerAdd failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for pending add request")
	}

	res := <-resultCh
	if res.err != nil {
		t.Fatalf("SendPeerAddRequest failed: %v", res.err)
	}
	if !res.accepted {
		t.Fatalf("expected add request to be accepted")
	}

	waitForPeerStatus(t, a.store, "peer-b", peerStatusOnline, 2*time.Second)
	waitForPeerStatus(t, b.store, "peer-a", peerStatusOnline, 2*time.Second)
}

func TestReconnectAfterPeerRestart(t *testing.T) {
	bPort := freeTCPPort(t)

	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	bIdentity := generateIdentity(t, "peer-b", "Peer B")
	bStoreDir := t.TempDir()
	bStore, _, err := storage.Open(bStoreDir)
	if err != nil {
		t.Fatalf("open B store: %v", err)
	}
	defer func() {
		_ = bStore.Close()
	}()

	b := newTestManagerWithParts(t, bStore, bIdentity, "127.0.0.1:"+bPort)
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("initial add flow failed: %v", err)
	}

	// Simulate B app restart.
	b.stop()

	waitForPeerStatus(t, a.store, "peer-b", peerStatusOffline, 4*time.Second)

	b2 := newTestManagerWithParts(t, bStore, bIdentity, "127.0.0.1:"+bPort)
	defer b2.stop()

	waitForPeerStatus(t, a.store, "peer-b", peerStatusOnline, 5*time.Second)
}

func TestPeerRemoveCleansUpBothSides(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
		approve: func(notification AddRequestNotification) (bool, error) {
			return true, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	accepted, err := a.manager.SendPeerAddRequest("peer-b")
	if err != nil {
		t.Fatalf("SendPeerAddRequest failed: %v", err)
	}
	if !accepted {
		t.Fatalf("expected accepted add")
	}

	if err := a.manager.SendPeerRemove("peer-b"); err != nil {
		t.Fatalf("SendPeerRemove failed: %v", err)
	}

	waitForPeerRemoved(t, a.store, "peer-b", 3*time.Second)
	waitForPeerRemoved(t, b.store, "peer-a", 3*time.Second)
}

func TestSimultaneousAddResolution(t *testing.T) {
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

	type addResult struct {
		accepted bool
		err      error
	}
	var wg sync.WaitGroup
	wg.Add(2)

	resA := make(chan addResult, 1)
	resB := make(chan addResult, 1)

	go func() {
		defer wg.Done()
		accepted, err := a.manager.SendPeerAddRequest("peer-b")
		resA <- addResult{accepted: accepted, err: err}
	}()
	go func() {
		defer wg.Done()
		accepted, err := b.manager.SendPeerAddRequest("peer-a")
		resB <- addResult{accepted: accepted, err: err}
	}()

	wg.Wait()

	aResult := <-resA
	bResult := <-resB

	if aResult.err != nil || bResult.err != nil {
		t.Fatalf("simultaneous add errors: a=%v b=%v", aResult.err, bResult.err)
	}
	if !aResult.accepted || !bResult.accepted {
		t.Fatalf("expected both simultaneous add requests to resolve as accepted")
	}

	waitForPeerStatus(t, a.store, "peer-b", peerStatusOnline, 2*time.Second)
	waitForPeerStatus(t, b.store, "peer-a", peerStatusOnline, 2*time.Second)
}

type testManager struct {
	manager  *PeerManager
	store    *storage.Store
	ownStore bool
}

func (m *testManager) addr() string {
	addr := m.manager.Addr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

func (m *testManager) stop() {
	if m == nil {
		return
	}
	if m.manager != nil {
		m.manager.Stop()
	}
	if m.ownStore && m.store != nil {
		_ = m.store.Close()
	}
}

type testManagerConfig struct {
	deviceID string
	name     string
	approve  func(notification AddRequestNotification) (bool, error)
}

func newTestManager(t *testing.T, cfg testManagerConfig) *testManager {
	t.Helper()

	storeDir := filepath.Join(t.TempDir(), cfg.deviceID)
	store, _, err := storage.Open(storeDir)
	if err != nil {
		t.Fatalf("open store %s: %v", cfg.deviceID, err)
	}

	identity := generateIdentity(t, cfg.deviceID, cfg.name)
	return newTestManagerWithConfig(t, store, identity, "127.0.0.1:0", cfg.approve, true)
}

func newTestManagerWithParts(t *testing.T, store *storage.Store, identity LocalIdentity, listenAddress string) *testManager {
	t.Helper()
	return newTestManagerWithConfig(t, store, identity, listenAddress, func(notification AddRequestNotification) (bool, error) {
		return true, nil
	}, false)
}

func newTestManagerWithConfig(t *testing.T, store *storage.Store, identity LocalIdentity, listenAddress string, approve func(notification AddRequestNotification) (bool, error), ownStore bool) *testManager {
	t.Helper()

	manager, err := NewPeerManager(PeerManagerOptions{
		Identity:           identity,
		Store:              store,
		ListenAddress:      listenAddress,
		ApproveAddRequest:  approve,
		ReconnectBackoff:   []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond},
		AddResponseTimeout: 3 * time.Second,
		ConnectionTimeout:  2 * time.Second,
		KeepAliveInterval:  80 * time.Millisecond,
		KeepAliveTimeout:   80 * time.Millisecond,
		FrameReadTimeout:   30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewPeerManager failed: %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	return &testManager{
		manager:  manager,
		store:    store,
		ownStore: ownStore,
	}
}

func generateIdentity(t *testing.T, deviceID, name string) LocalIdentity {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return LocalIdentity{
		DeviceID:          deviceID,
		DeviceName:        name,
		Ed25519PrivateKey: privateKey,
		Ed25519PublicKey:  publicKey,
	}
}

func addWithAutoApproval(requester *PeerManager, peerID string, approver *PeerManager, expectedRequesterID string) (bool, error) {
	resultCh := make(chan struct {
		accepted bool
		err      error
	}, 1)

	go func() {
		accepted, err := requester.SendPeerAddRequest(peerID)
		resultCh <- struct {
			accepted bool
			err      error
		}{accepted: accepted, err: err}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for {
		select {
		case req := <-approver.PendingAddRequests():
			if req.PeerDeviceID == expectedRequesterID {
				_ = approver.ApprovePeerAdd(req.PeerDeviceID, true)
			}
		case result := <-resultCh:
			return result.accepted, result.err
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

func waitForPeerStatus(t *testing.T, store *storage.Store, peerID, expectedStatus string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		peer, err := store.GetPeer(peerID)
		if err == nil && peer.Status == expectedStatus {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	peer, err := store.GetPeer(peerID)
	if err != nil {
		t.Fatalf("timed out waiting for peer %q status=%q, final error=%v", peerID, expectedStatus, err)
	}
	t.Fatalf("timed out waiting for peer %q status=%q, final=%q", peerID, expectedStatus, peer.Status)
}

func waitForPeerRemoved(t *testing.T, store *storage.Store, peerID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := store.GetPeer(peerID)
		if errors.Is(err, storage.ErrNotFound) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for peer %q removal", peerID)
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	defer func() {
		_ = ln.Close()
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host/port: %v", err)
	}
	return port
}
