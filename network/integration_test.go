package network

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	appcrypto "gosend/crypto"
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

func TestHandshakeReplayRejectedByChallengeNonceMismatch(t *testing.T) {
	serverIdentity := testIdentity(t, "server-replay", "Server Replay")
	clientIdentity := testIdentity(t, "client-replay", "Client Replay")

	server, err := Listen("127.0.0.1:0", HandshakeOptions{
		Identity: serverIdentity,
	})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()

	recordedHandshakePayload, err := func() ([]byte, error) {
		rawConn, err := net.DialTimeout("tcp", server.Addr().String(), 2*time.Second)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = rawConn.Close()
		}()
		if err := rawConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return nil, err
		}

		challengePayload, err := ReadFrameWithTimeout(rawConn, 2*time.Second)
		if err != nil {
			return nil, err
		}
		var challenge HandshakeChallenge
		if err := json.Unmarshal(challengePayload, &challenge); err != nil {
			return nil, err
		}

		_, localEphemeralPublicKey, err := appcrypto.GenerateEphemeralX25519KeyPair()
		if err != nil {
			return nil, err
		}
		handshake, err := BuildHandshakeMessage(clientIdentity, localEphemeralPublicKey.Bytes(), challenge.Nonce)
		if err != nil {
			return nil, err
		}
		payload, err := EncodeJSON(handshake)
		if err != nil {
			return nil, err
		}
		if err := WriteFrame(rawConn, payload); err != nil {
			return nil, err
		}

		responsePayload, err := ReadFrameWithTimeout(rawConn, 2*time.Second)
		if err != nil {
			return nil, err
		}
		msgType, err := DecodeMessageType(responsePayload)
		if err != nil {
			return nil, err
		}
		if msgType != TypeHandshakeResponse {
			return nil, errors.New("expected handshake_response while recording handshake")
		}

		serverConn := waitForServerConn(t, server)
		_ = serverConn.Close()

		return payload, nil
	}()
	if err != nil {
		t.Fatalf("record handshake failed: %v", err)
	}

	rawConn, err := net.DialTimeout("tcp", server.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial replay attempt failed: %v", err)
	}
	defer func() {
		_ = rawConn.Close()
	}()
	if err := rawConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set replay deadline failed: %v", err)
	}

	challengePayload, err := ReadFrameWithTimeout(rawConn, 2*time.Second)
	if err != nil {
		t.Fatalf("read replay challenge failed: %v", err)
	}
	var challenge HandshakeChallenge
	if err := json.Unmarshal(challengePayload, &challenge); err != nil {
		t.Fatalf("decode replay challenge failed: %v", err)
	}

	if err := WriteFrame(rawConn, recordedHandshakePayload); err != nil {
		t.Fatalf("send replayed handshake failed: %v", err)
	}

	responsePayload, err := ReadFrameWithTimeout(rawConn, 2*time.Second)
	if err != nil {
		t.Fatalf("read replay response failed: %v", err)
	}

	msgType, err := DecodeMessageType(responsePayload)
	if err != nil {
		t.Fatalf("decode replay response type failed: %v", err)
	}
	if msgType != TypeError {
		t.Fatalf("expected error response for replay, got %q", msgType)
	}
	var protocolErr ErrorMessage
	if err := json.Unmarshal(responsePayload, &protocolErr); err != nil {
		t.Fatalf("decode replay protocol error failed: %v", err)
	}
	if protocolErr.Code != "invalid_handshake_challenge" {
		t.Fatalf("expected invalid_handshake_challenge, got %q", protocolErr.Code)
	}

	select {
	case unexpected := <-server.Incoming():
		_ = unexpected.Close()
		t.Fatalf("expected replay handshake to be rejected before incoming connection")
	case <-time.After(250 * time.Millisecond):
	}

	if challenge.Nonce == "" {
		t.Fatalf("expected non-empty challenge nonce on replay attempt")
	}
}

func TestOversizedControlFrameDuringHandshakeRejected(t *testing.T) {
	serverIdentity := testIdentity(t, "server-control-limit", "Server Control Limit")

	server, err := Listen("127.0.0.1:0", HandshakeOptions{
		Identity: serverIdentity,
	})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()

	rawConn, err := net.DialTimeout("tcp", server.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer func() {
		_ = rawConn.Close()
	}()

	if _, err := ReadControlFrameWithTimeout(rawConn, 2*time.Second); err != nil {
		t.Fatalf("read handshake challenge failed: %v", err)
	}

	oversized := make([]byte, MaxControlFrameSize+1)
	if err := WriteFrame(rawConn, oversized); err != nil {
		t.Fatalf("send oversized control frame failed: %v", err)
	}

	if _, err := ReadFrameWithTimeout(rawConn, 600*time.Millisecond); err == nil {
		t.Fatalf("expected connection close after oversized control frame")
	}

	select {
	case conn := <-server.Incoming():
		_ = conn.Close()
		t.Fatalf("expected handshake to be rejected before incoming connection")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestOversizedControlPayloadDisconnectsConnection(t *testing.T) {
	serverIdentity := testIdentity(t, "server-control-loop", "Server Control Loop")
	clientIdentity := testIdentity(t, "client-control-loop", "Client Control Loop")

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

	oversizedControlPayload := []byte(`{"type":"ping","from_device_id":"client-control-loop","timestamp":1,"padding":"` + strings.Repeat("x", MaxControlFrameSize) + `"}`)
	if len(oversizedControlPayload) <= MaxControlFrameSize {
		t.Fatalf("expected oversized control payload, got %d bytes", len(oversizedControlPayload))
	}

	if err := clientConn.SendRaw(oversizedControlPayload); err != nil {
		t.Fatalf("send oversized control payload failed: %v", err)
	}

	select {
	case <-serverConn.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("expected server connection to close after oversized control payload")
	}

	if !errors.Is(serverConn.LastError(), ErrFrameTooLarge) {
		t.Fatalf("expected ErrFrameTooLarge, got %v", serverConn.LastError())
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
