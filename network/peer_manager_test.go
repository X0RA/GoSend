package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	mathrand "math/rand"
	"net"
	"path/filepath"
	"strconv"
	"strings"
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
	if _, err := a.store.GetPeerSettings("peer-b"); err != nil {
		t.Fatalf("expected peer settings row for peer-b, got error: %v", err)
	}
	if _, err := b.store.GetPeerSettings("peer-a"); err != nil {
		t.Fatalf("expected peer settings row for peer-a, got error: %v", err)
	}
}

func TestConnectDoesNotAutoAddPeer(t *testing.T) {
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

	waitForPeerNotFound(t, a.store, "peer-b", 2*time.Second)
	waitForPeerNotFound(t, b.store, "peer-a", 2*time.Second)
}

func TestUpdateDeviceNameAppliesToFutureProtocolMessages(t *testing.T) {
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

	if err := a.manager.UpdateDeviceName("Renamed Peer A"); err != nil {
		t.Fatalf("UpdateDeviceName failed: %v", err)
	}

	resultCh := make(chan struct {
		accepted bool
		err      error
	}, 1)
	go func() {
		accepted, err := a.manager.SendPeerAddRequest("peer-b")
		resultCh <- struct {
			accepted bool
			err      error
		}{accepted: accepted, err: err}
	}()

	select {
	case req := <-b.manager.PendingAddRequests():
		if req.PeerDeviceName != "Renamed Peer A" {
			t.Fatalf("expected renamed peer device name in add request, got %q", req.PeerDeviceName)
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

	self, err := a.store.GetPeer("peer-a")
	if err != nil {
		t.Fatalf("GetPeer self failed: %v", err)
	}
	if self.DeviceName != "Renamed Peer A" {
		t.Fatalf("expected self device name update to be persisted, got %q", self.DeviceName)
	}
}

func TestPeerAddRejectedDoesNotPersistPeer(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
		approve: func(notification AddRequestNotification) (bool, error) {
			return false, nil
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
	if accepted {
		t.Fatalf("expected add request to be rejected")
	}

	waitForPeerNotFound(t, a.store, "peer-b", 2*time.Second)
	waitForPeerNotFound(t, b.store, "peer-a", 2*time.Second)
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

func TestTrustedKeyChangeUpdatesStoredKeyAndAvoidsReprompt(t *testing.T) {
	bPort := freeTCPPort(t)

	callbackCount := 0
	var callbackMu sync.Mutex

	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
		onKeyChange: func(peerDeviceID, existingPublicKeyBase64, receivedPublicKeyBase64 string) (bool, error) {
			callbackMu.Lock()
			callbackCount++
			callbackMu.Unlock()
			return true, nil
		},
	})
	defer a.stop()

	bIdentityOriginal := generateIdentity(t, "peer-b", "Peer B")
	bStoreDir := t.TempDir()
	bStore, _, err := storage.Open(bStoreDir)
	if err != nil {
		t.Fatalf("open B store: %v", err)
	}
	defer func() {
		_ = bStore.Close()
	}()

	b := newTestManagerWithParts(t, bStore, bIdentityOriginal, "127.0.0.1:"+bPort)
	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	if _, err := addWithAutoApproval(a.manager, "peer-b", b.manager, "peer-a"); err != nil {
		t.Fatalf("initial add flow failed: %v", err)
	}

	originalFingerprint := appcrypto.KeyFingerprint(bIdentityOriginal.Ed25519PublicKey)
	b.stop()

	waitForPeerStatus(t, a.store, "peer-b", peerStatusOffline, 4*time.Second)

	bIdentityRotated := generateIdentity(t, "peer-b", "Peer B")
	bRotated := newTestManagerWithParts(t, bStore, bIdentityRotated, "127.0.0.1:"+bPort)
	defer bRotated.stop()

	waitForPeerStatus(t, a.store, "peer-b", peerStatusOnline, 6*time.Second)
	waitForKeyChangeCallbackCount(t, &callbackMu, &callbackCount, 1, 5*time.Second)

	peer, err := a.store.GetPeer("peer-b")
	if err != nil {
		t.Fatalf("GetPeer after key rotation failed: %v", err)
	}
	rotatedPublicKey := base64.StdEncoding.EncodeToString(bIdentityRotated.Ed25519PublicKey)
	if peer.Ed25519PublicKey != rotatedPublicKey {
		t.Fatalf("expected stored public key to rotate")
	}
	rotatedFingerprint := appcrypto.KeyFingerprint(bIdentityRotated.Ed25519PublicKey)
	if peer.KeyFingerprint != rotatedFingerprint {
		t.Fatalf("expected stored fingerprint %q, got %q", rotatedFingerprint, peer.KeyFingerprint)
	}

	events, err := a.store.GetRecentKeyRotationEvents("peer-b", 10)
	if err != nil {
		t.Fatalf("GetRecentKeyRotationEvents failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected at least one key rotation event")
	}
	if events[0].Decision != storage.KeyRotationDecisionTrusted {
		t.Fatalf("expected trusted key rotation decision, got %q", events[0].Decision)
	}
	if events[0].OldKeyFingerprint != originalFingerprint {
		t.Fatalf("unexpected old key fingerprint: got %q want %q", events[0].OldKeyFingerprint, originalFingerprint)
	}
	if events[0].NewKeyFingerprint != rotatedFingerprint {
		t.Fatalf("unexpected new key fingerprint: got %q want %q", events[0].NewKeyFingerprint, rotatedFingerprint)
	}

	bRotated.stop()
	waitForPeerStatus(t, a.store, "peer-b", peerStatusOffline, 4*time.Second)

	bRotatedAgain := newTestManagerWithParts(t, bStore, bIdentityRotated, "127.0.0.1:"+bPort)
	defer bRotatedAgain.stop()
	waitForPeerStatus(t, a.store, "peer-b", peerStatusOnline, 6*time.Second)

	time.Sleep(300 * time.Millisecond)
	callbackMu.Lock()
	finalCount := callbackCount
	callbackMu.Unlock()
	if finalCount != 1 {
		t.Fatalf("expected no second key-change prompt after trusting rotated key, got %d callbacks", finalCount)
	}
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

	events, err := b.store.GetSecurityEvents(storage.SecurityEventFilter{
		EventType:    securityEventTypeSignatureVerificationFailed,
		PeerDeviceID: "peer-a",
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("GetSecurityEvents failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected signature verification failure security event")
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

func TestPeerAddRequestCooldownDropsRapidDuplicates(t *testing.T) {
	var approveCount int
	var approveMu sync.Mutex

	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID: "peer-b",
		name:     "Peer B",
		approve: func(notification AddRequestNotification) (bool, error) {
			approveMu.Lock()
			approveCount++
			approveMu.Unlock()
			return true, nil
		},
		addRequestCooldown: 5 * time.Second,
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	sendAddRequest := func() {
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
	}

	sendAddRequest()
	waitForCondition(t, 2*time.Second, func() bool {
		approveMu.Lock()
		defer approveMu.Unlock()
		return approveCount == 1
	})

	sendAddRequest()
	time.Sleep(300 * time.Millisecond)

	approveMu.Lock()
	finalApprovals := approveCount
	approveMu.Unlock()
	if finalApprovals != 1 {
		t.Fatalf("expected cooldown to suppress duplicate add request, approvals=%d", finalApprovals)
	}

	events, err := b.store.GetSecurityEvents(storage.SecurityEventFilter{
		EventType:    securityEventTypeConnectionRateLimitTriggered,
		PeerDeviceID: "peer-a",
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("GetSecurityEvents failed: %v", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Details, "peer_add_request_cooldown") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected peer_add_request_cooldown security event")
	}
}

func TestFileRequestRateLimitTriggersAfterBurst(t *testing.T) {
	a := newTestManager(t, testManagerConfig{
		deviceID: "peer-a",
		name:     "Peer A",
	})
	defer a.stop()

	b := newTestManager(t, testManagerConfig{
		deviceID:             "peer-b",
		name:                 "Peer B",
		fileRequestRateLimit: 5,
		fileRequestWindow:    time.Minute,
		approveFile: func(notification FileRequestNotification) (bool, error) {
			return false, nil
		},
	})
	defer b.stop()

	if _, err := a.manager.Connect(b.addr()); err != nil {
		t.Fatalf("A connect B failed: %v", err)
	}
	conn := a.manager.getConnection("peer-b")
	if conn == nil {
		t.Fatalf("expected connection to peer-b")
	}

	for i := 0; i < 6; i++ {
		request := FileRequest{
			Type:         TypeFileRequest,
			FileID:       "burst-file-" + strconv.Itoa(i),
			FromDeviceID: "peer-a",
			ToDeviceID:   "peer-b",
			Filename:     "burst.bin",
			Filesize:     1024,
			Filetype:     "application/octet-stream",
			Checksum:     strings.Repeat("a", 64),
			Sequence:     conn.NextSendSequence(),
			Timestamp:    time.Now().UnixMilli(),
		}
		if err := a.manager.signFileRequest(&request); err != nil {
			t.Fatalf("sign file request failed: %v", err)
		}
		if err := conn.SendMessage(request); err != nil {
			t.Fatalf("send file request failed: %v", err)
		}
	}

	waitForCondition(t, 2*time.Second, func() bool {
		events, err := b.store.GetSecurityEvents(storage.SecurityEventFilter{
			EventType:    securityEventTypeConnectionRateLimitTriggered,
			PeerDeviceID: "peer-a",
			Limit:        20,
		})
		if err != nil {
			return false
		}
		for _, event := range events {
			if strings.Contains(event.Details, "file_request_per_peer") {
				return true
			}
		}
		return false
	})
}

func TestBackoffAppliesJitterWithinBounds(t *testing.T) {
	manager := newTestManager(t, testManagerConfig{
		deviceID:                "peer-jitter",
		name:                    "Peer Jitter",
		reconnectJitterFraction: 0.25,
		randomSource:            mathrand.New(mathrand.NewSource(7)),
	})
	defer manager.stop()

	manager.manager.options.ReconnectBackoff = []time.Duration{100 * time.Millisecond}
	for i := 0; i < 25; i++ {
		delay := manager.manager.backoffForAttempt(i)
		if delay < 75*time.Millisecond || delay > 125*time.Millisecond {
			t.Fatalf("jittered delay out of range: %s", delay)
		}
	}
}

func TestSortEndpointsByHealthPrefersHigherScores(t *testing.T) {
	manager := newTestManager(t, testManagerConfig{
		deviceID: "peer-health",
		name:     "Peer Health",
	})
	defer manager.stop()

	peerID := "peer-target"
	endpointA := "10.0.0.1:9000"
	endpointB := "10.0.0.2:9000"
	endpointC := "10.0.0.3:9000"

	manager.manager.adjustEndpointHealth(peerID, endpointA, 3)
	manager.manager.adjustEndpointHealth(peerID, endpointC, 1)
	sorted := manager.manager.sortEndpointsByHealth(peerID, []string{endpointB, endpointC, endpointA})

	if len(sorted) != 3 {
		t.Fatalf("unexpected sorted endpoint count: %d", len(sorted))
	}
	if sorted[0] != endpointA || sorted[1] != endpointC || sorted[2] != endpointB {
		t.Fatalf("unexpected endpoint order: %v", sorted)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout %s", timeout)
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
	deviceID                  string
	name                      string
	approve                   func(notification AddRequestNotification) (bool, error)
	onKeyChange               KeyChangeDecisionFunc
	approveFile               func(notification FileRequestNotification) (bool, error)
	onFileProgress            func(progress FileProgress)
	filesDir                  string
	maxReceiveFileSize        int64
	fileChunkSize             int
	rekeyInterval             time.Duration
	rekeyAfterBytes           uint64
	addRequestCooldown        time.Duration
	fileRequestRateLimit      int
	fileRequestWindow         time.Duration
	maxReconnectAttempts      int
	reconnectJitterFraction   float64
	randomSource              *mathrand.Rand
	connectionRateLimitPerIP  int
	connectionRateLimitWindow time.Duration
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
		Identity:                  identity,
		Store:                     store,
		ListenAddress:             listenAddress,
		ApproveAddRequest:         cfg.approve,
		OnKeyChange:               cfg.onKeyChange,
		OnFileRequest:             cfg.approveFile,
		OnFileProgress:            cfg.onFileProgress,
		FilesDir:                  filesDir,
		MaxReceiveFileSize:        cfg.maxReceiveFileSize,
		FileChunkSize:             chunkSize,
		FileResponseTimeout:       2 * time.Second,
		MaxChunkRetries:           3,
		ReconnectBackoff:          []time.Duration{0, 50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond},
		MaxReconnectAttempts:      cfg.maxReconnectAttempts,
		ReconnectJitterFraction:   cfg.reconnectJitterFraction,
		RandomSource:              cfg.randomSource,
		AddResponseTimeout:        3 * time.Second,
		AddRequestCooldown:        cfg.addRequestCooldown,
		FileRequestRateLimit:      cfg.fileRequestRateLimit,
		FileRequestWindow:         cfg.fileRequestWindow,
		ConnectionTimeout:         2 * time.Second,
		KeepAliveInterval:         80 * time.Millisecond,
		KeepAliveTimeout:          80 * time.Millisecond,
		FrameReadTimeout:          30 * time.Millisecond,
		ConnectionRateLimitPerIP:  cfg.connectionRateLimitPerIP,
		ConnectionRateLimitWindow: cfg.connectionRateLimitWindow,
		RekeyInterval:             cfg.rekeyInterval,
		RekeyAfterBytes:           cfg.rekeyAfterBytes,
		RekeyResponseTimeout:      2 * time.Second,
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

func waitForPeerNotFound(t *testing.T, store *storage.Store, peerID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := store.GetPeer(peerID)
		if errors.Is(err, storage.ErrNotFound) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	peer, err := store.GetPeer(peerID)
	if err == nil {
		t.Fatalf("timed out waiting for peer %q to stay absent, final status=%q", peerID, peer.Status)
	}
	t.Fatalf("timed out waiting for peer %q to stay absent, final error=%v", peerID, err)
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

func waitForKeyChangeCallbackCount(t *testing.T, mu *sync.Mutex, count *int, expected int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		current := *count
		mu.Unlock()
		if current >= expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	current := *count
	mu.Unlock()
	t.Fatalf("timed out waiting for key-change callback count %d, final=%d", expected, current)
}
