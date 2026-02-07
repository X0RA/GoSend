package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appcrypto "gosend/crypto"
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

func TestInvalidPeerAddRequestSignatureIgnored(t *testing.T) {
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
	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	request := PeerAddRequest{
		Type:           TypePeerAddRequest,
		FromDeviceID:   "peer-a",
		FromDeviceName: "Peer A",
		Timestamp:      time.Now().UnixMilli(),
	}
	if err := a.manager.signPeerAddRequest(&request); err != nil {
		t.Fatalf("sign peer add request failed: %v", err)
	}

	sig, err := base64.StdEncoding.DecodeString(request.Signature)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	sig[0] ^= 0xFF
	request.Signature = base64.StdEncoding.EncodeToString(sig)

	if err := conn.SendMessage(request); err != nil {
		t.Fatalf("send forged peer add request failed: %v", err)
	}

	select {
	case req := <-b.manager.PendingAddRequests():
		t.Fatalf("unexpected pending add request: %+v", req)
	case <-time.After(400 * time.Millisecond):
	}
}

func TestPeerRemoveCannotRemoveSpoofedDeviceID(t *testing.T) {
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

	victimIdentity := generateIdentity(t, "victim-peer", "Victim")
	victimPubKey := base64.StdEncoding.EncodeToString(victimIdentity.Ed25519PublicKey)
	now := time.Now().UnixMilli()
	if err := b.store.AddPeer(storage.Peer{
		DeviceID:         "victim-peer",
		DeviceName:       "Victim",
		Ed25519PublicKey: victimPubKey,
		KeyFingerprint:   appcrypto.KeyFingerprint(victimIdentity.Ed25519PublicKey),
		Status:           peerStatusOnline,
		AddedTimestamp:   now,
	}); err != nil {
		t.Fatalf("seed victim peer failed: %v", err)
	}

	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	remove := PeerRemove{
		Type:         TypePeerRemove,
		FromDeviceID: "victim-peer",
		Timestamp:    time.Now().UnixMilli(),
	}
	if err := a.manager.signPeerRemove(&remove); err != nil {
		t.Fatalf("sign peer remove failed: %v", err)
	}
	if err := conn.SendMessage(remove); err != nil {
		t.Fatalf("send spoofed peer remove failed: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if _, err := b.store.GetPeer("victim-peer"); err != nil {
		t.Fatalf("expected victim peer to remain, got err: %v", err)
	}
}

func TestInboundAddRequestPendingEntryRemovedOnDisconnect(t *testing.T) {
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
	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	request := PeerAddRequest{
		Type:           TypePeerAddRequest,
		FromDeviceID:   "peer-a",
		FromDeviceName: "Peer A",
		Timestamp:      time.Now().UnixMilli(),
	}
	if err := a.manager.signPeerAddRequest(&request); err != nil {
		t.Fatalf("sign peer add request failed: %v", err)
	}
	if err := conn.SendMessage(request); err != nil {
		t.Fatalf("send peer add request failed: %v", err)
	}

	waitForInboundPendingCount(t, b.manager, 1, 2*time.Second)
	a.stop()
	waitForInboundPendingCount(t, b.manager, 0, 2*time.Second)
}

func TestPeerAddResponseMustMatchConnectionPeer(t *testing.T) {
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

	c := newTestManager(t, testManagerConfig{
		deviceID: "peer-c",
		name:     "Peer C",
	})
	defer c.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := a.manager.Connect(c.addr()); err != nil {
		t.Fatalf("A connect C failed: %v", err)
	}

	type addResult struct {
		accepted bool
		err      error
	}
	resultCh := make(chan addResult, 1)
	go func() {
		accepted, err := a.manager.SendPeerAddRequest("peer-c")
		resultCh <- addResult{accepted: accepted, err: err}
	}()

	waitForOutboundAddPending(t, a.manager, "peer-c", 2*time.Second)

	conn := b.manager.getConnection("peer-a")
	if conn == nil {
		t.Fatalf("expected connection from B to A")
	}
	forged := PeerAddResponse{
		Type:       TypePeerAddResponse,
		Status:     "accepted",
		DeviceID:   "peer-c",
		DeviceName: "Peer C",
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := b.manager.signPeerAddResponse(&forged); err != nil {
		t.Fatalf("sign forged peer_add_response failed: %v", err)
	}
	if err := conn.SendMessage(forged); err != nil {
		t.Fatalf("send forged peer_add_response failed: %v", err)
	}

	select {
	case res := <-resultCh:
		if res.accepted {
			t.Fatalf("expected forged response to be rejected")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for add request result")
	}
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
	deviceID       string
	name           string
	approve        func(notification AddRequestNotification) (bool, error)
	approveFile    func(notification FileRequestNotification) (bool, error)
	onFileProgress func(progress FileProgress)
	filesDir       string
	fileChunkSize  int
}

func newTestManager(t *testing.T, cfg testManagerConfig) *testManager {
	t.Helper()

	storeDir := filepath.Join(t.TempDir(), cfg.deviceID)
	store, _, err := storage.Open(storeDir)
	if err != nil {
		t.Fatalf("open store %s: %v", cfg.deviceID, err)
	}

	identity := generateIdentity(t, cfg.deviceID, cfg.name)
	return newTestManagerWithConfig(t, store, identity, "127.0.0.1:0", cfg, true)
}

func newTestManagerWithParts(t *testing.T, store *storage.Store, identity LocalIdentity, listenAddress string) *testManager {
	t.Helper()
	return newTestManagerWithConfig(t, store, identity, listenAddress, testManagerConfig{
		approve: func(notification AddRequestNotification) (bool, error) {
			return true, nil
		},
	}, false)
}

func newTestManagerWithConfig(t *testing.T, store *storage.Store, identity LocalIdentity, listenAddress string, cfg testManagerConfig, ownStore bool) *testManager {
	t.Helper()

	filesDir := cfg.filesDir
	if filesDir == "" {
		filesDir = filepath.Join(t.TempDir(), "files", identity.DeviceID)
	}
	chunkSize := cfg.fileChunkSize
	if chunkSize <= 0 {
		chunkSize = 32 * 1024
	}

	manager, err := NewPeerManager(PeerManagerOptions{
		Identity:            identity,
		Store:               store,
		ListenAddress:       listenAddress,
		ApproveAddRequest:   cfg.approve,
		OnFileRequest:       cfg.approveFile,
		OnFileProgress:      cfg.onFileProgress,
		FilesDir:            filesDir,
		FileChunkSize:       chunkSize,
		FileResponseTimeout: 2 * time.Second,
		MaxChunkRetries:     3,
		ReconnectBackoff:    []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond},
		AddResponseTimeout:  3 * time.Second,
		ConnectionTimeout:   2 * time.Second,
		KeepAliveInterval:   80 * time.Millisecond,
		KeepAliveTimeout:    80 * time.Millisecond,
		FrameReadTimeout:    30 * time.Millisecond,
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

func waitForInboundPendingCount(t *testing.T, manager *PeerManager, expected int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		manager.inboundMu.Lock()
		count := len(manager.inboundAddPending)
		manager.inboundMu.Unlock()
		if count == expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	manager.inboundMu.Lock()
	count := len(manager.inboundAddPending)
	manager.inboundMu.Unlock()
	t.Fatalf("timed out waiting for inbound pending count=%d, final=%d", expected, count)
}

func waitForOutboundAddPending(t *testing.T, manager *PeerManager, peerID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		manager.outboundMu.Lock()
		_, exists := manager.outboundAddResponse[peerID]
		manager.outboundMu.Unlock()
		if exists {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for outbound add response channel for peer %q", peerID)
}
