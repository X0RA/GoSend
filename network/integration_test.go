package network

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"
)

func TestHandshakeDerivesMatchingSessionKeys(t *testing.T) {
	serverIdentity := testIdentity(t, "server-device", "Server")
	clientIdentity := testIdentity(t, "client-device", "Client")

	server, err := Listen("127.0.0.1:0", HandshakeOptions{
		Identity: serverIdentity,
	})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()

	clientConn, err := Dial(server.Addr().String(), HandshakeOptions{
		Identity: clientIdentity,
	})
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() {
		_ = clientConn.Close()
	}()

	serverConn := waitForServerConn(t, server)
	defer func() {
		_ = serverConn.Close()
	}()

	if !bytes.Equal(clientConn.SessionKey(), serverConn.SessionKey()) {
		t.Fatalf("session keys do not match")
	}
	if clientConn.State() == StateDisconnected {
		t.Fatalf("client should be connected")
	}
	if serverConn.State() == StateDisconnected {
		t.Fatalf("server should be connected")
	}
}

func TestKeepAliveMaintainsIdleConnections(t *testing.T) {
	serverIdentity := testIdentity(t, "server-idle", "Server Idle")
	clientIdentity := testIdentity(t, "client-idle", "Client Idle")

	server, err := Listen("127.0.0.1:0", HandshakeOptions{
		Identity:          serverIdentity,
		KeepAliveInterval: 120 * time.Millisecond,
		KeepAliveTimeout:  120 * time.Millisecond,
		FrameReadTimeout:  40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()

	clientConn, err := Dial(server.Addr().String(), HandshakeOptions{
		Identity:          clientIdentity,
		KeepAliveInterval: 120 * time.Millisecond,
		KeepAliveTimeout:  120 * time.Millisecond,
		FrameReadTimeout:  40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() {
		_ = clientConn.Close()
	}()

	serverConn := waitForServerConn(t, server)
	defer func() {
		_ = serverConn.Close()
	}()

	time.Sleep(600 * time.Millisecond)

	if clientConn.State() == StateDisconnected {
		t.Fatalf("client unexpectedly disconnected during keep-alive idle period")
	}
	if serverConn.State() == StateDisconnected {
		t.Fatalf("server unexpectedly disconnected during keep-alive idle period")
	}
}

func TestDeadConnectionDetectedOnPingTimeout(t *testing.T) {
	serverIdentity := testIdentity(t, "server-timeout", "Server Timeout")
	clientIdentity := testIdentity(t, "client-timeout", "Client Timeout")
	disableAutoPong := false

	server, err := Listen("127.0.0.1:0", HandshakeOptions{
		Identity:          serverIdentity,
		KeepAliveInterval: 500 * time.Millisecond,
		KeepAliveTimeout:  200 * time.Millisecond,
		FrameReadTimeout:  40 * time.Millisecond,
		AutoRespondPing:   &disableAutoPong,
	})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()

	clientConn, err := Dial(server.Addr().String(), HandshakeOptions{
		Identity:          clientIdentity,
		KeepAliveInterval: 80 * time.Millisecond,
		KeepAliveTimeout:  80 * time.Millisecond,
		FrameReadTimeout:  40 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() {
		_ = clientConn.Close()
	}()

	serverConn := waitForServerConn(t, server)
	defer func() {
		_ = serverConn.Close()
	}()

	select {
	case <-clientConn.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("expected client connection to close after ping timeout")
	}

	if !errors.Is(clientConn.LastError(), ErrPongTimeout) {
		t.Fatalf("expected ErrPongTimeout, got %v", clientConn.LastError())
	}
}

func TestKeyChangeDecisionBlocksHandshakeUntilDecision(t *testing.T) {
	serverIdentity := testIdentity(t, "server-keychange", "Server Key")
	clientIdentity := testIdentity(t, "client-keychange", "Client Key")

	var callbackCalled bool
	start := time.Now()

	server, err := Listen("127.0.0.1:0", HandshakeOptions{
		Identity: serverIdentity,
		KnownPeerKeys: map[string]string{
			clientIdentity.DeviceID: "previous-key-material",
		},
		OnKeyChangeDecision: func(peerDeviceID, existingPublicKeyBase64, receivedPublicKeyBase64 string) (bool, error) {
			callbackCalled = true
			time.Sleep(75 * time.Millisecond)
			return true, nil
		},
	})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()

	clientConn, err := Dial(server.Addr().String(), HandshakeOptions{
		Identity: clientIdentity,
	})
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() {
		_ = clientConn.Close()
	}()

	serverConn := waitForServerConn(t, server)
	defer func() {
		_ = serverConn.Close()
	}()

	if !callbackCalled {
		t.Fatalf("expected key change callback to be invoked")
	}
	if elapsed := time.Since(start); elapsed < 75*time.Millisecond {
		t.Fatalf("expected handshake to block on key change decision, completed in %s", elapsed)
	}
}

func waitForServerConn(t *testing.T, server *Server) *PeerConnection {
	t.Helper()
	select {
	case conn := <-server.Incoming():
		return conn
	case err := <-server.Errors():
		t.Fatalf("server error before incoming connection: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for incoming server connection")
	}
	return nil
}

func testIdentity(t *testing.T, deviceID, deviceName string) LocalIdentity {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 keypair: %v", err)
	}
	return LocalIdentity{
		DeviceID:          deviceID,
		DeviceName:        deviceName,
		Ed25519PrivateKey: privateKey,
		Ed25519PublicKey:  publicKey,
	}
}
