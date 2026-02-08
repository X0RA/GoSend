package network

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"testing"
	"time"

	appcrypto "gosend/crypto"
)

func TestRotateSessionKeyKeepsPreviousEpochDecryptable(t *testing.T) {
	oldKey := bytes.Repeat([]byte{0x11}, 32)
	newKey := bytes.Repeat([]byte{0x22}, 32)

	localConn, remoteConn := net.Pipe()
	defer func() {
		_ = remoteConn.Close()
	}()

	pc := newPeerConnection(localConn, oldKey, ConnectionOptions{
		LocalDeviceID:     "local",
		PeerDeviceID:      "peer",
		PeerDeviceName:    "Peer",
		PeerPublicKey:     "unused",
		KeepAliveInterval: time.Hour,
		KeepAliveTimeout:  time.Hour,
		FrameReadTimeout:  250 * time.Millisecond,
	})
	defer func() {
		_ = pc.Close()
	}()

	if err := pc.RotateSessionKey(newKey, 1); err != nil {
		t.Fatalf("RotateSessionKey failed: %v", err)
	}

	payload := []byte(`{"type":"ack","message_id":"m1","from_device_id":"peer","status":"delivered","timestamp":1,"signature":"sig"}`)
	ciphertext, nonce, err := appcrypto.Encrypt(oldKey, payload)
	if err != nil {
		t.Fatalf("encrypt payload with old key failed: %v", err)
	}

	framePayload, err := EncodeJSON(secureFrame{
		Type:       secureFrameType,
		Epoch:      0,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		t.Fatalf("EncodeJSON secureFrame failed: %v", err)
	}

	if err := WriteFrame(remoteConn, framePayload); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := pc.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("ReceiveMessage failed: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch after rekey transition")
	}
}

func TestConnectionClosesAfterThreeConsecutiveMalformedFrames(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, 32)
	localConn, remoteConn := net.Pipe()

	pc := newPeerConnection(localConn, key, ConnectionOptions{
		LocalDeviceID:     "local",
		PeerDeviceID:      "peer",
		PeerDeviceName:    "Peer",
		PeerPublicKey:     "unused",
		KeepAliveInterval: time.Hour,
		KeepAliveTimeout:  time.Hour,
		FrameReadTimeout:  100 * time.Millisecond,
	})
	defer func() {
		_ = remoteConn.Close()
		_ = pc.Close()
	}()

	for attempt := 1; attempt <= 2; attempt++ {
		if err := WriteFrame(remoteConn, []byte("{malformed")); err != nil {
			t.Fatalf("write malformed frame %d failed: %v", attempt, err)
		}
		select {
		case <-pc.Done():
			t.Fatalf("connection closed too early after %d malformed frames", attempt)
		case <-time.After(80 * time.Millisecond):
		}
	}

	if err := WriteFrame(remoteConn, []byte("{malformed")); err != nil {
		t.Fatalf("write malformed frame 3 failed: %v", err)
	}
	select {
	case <-pc.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("expected connection to close after third malformed frame")
	}
}

func TestConnectionMalformedCounterResetsAfterValidFrame(t *testing.T) {
	key := bytes.Repeat([]byte{0x44}, 32)
	localConn, remoteConn := net.Pipe()

	pc := newPeerConnection(localConn, key, ConnectionOptions{
		LocalDeviceID:     "local",
		PeerDeviceID:      "peer",
		PeerDeviceName:    "Peer",
		PeerPublicKey:     "unused",
		KeepAliveInterval: time.Hour,
		KeepAliveTimeout:  time.Hour,
		FrameReadTimeout:  100 * time.Millisecond,
	})
	defer func() {
		_ = remoteConn.Close()
		_ = pc.Close()
	}()

	for i := 0; i < 2; i++ {
		if err := WriteFrame(remoteConn, []byte("bad-frame")); err != nil {
			t.Fatalf("write malformed frame failed: %v", err)
		}
	}

	validPayload, err := buildSecureTestFramePayload(key, PingMessage{
		Type:         TypePing,
		FromDeviceID: "peer",
		Timestamp:    time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("build valid secure frame failed: %v", err)
	}
	if err := WriteFrame(remoteConn, validPayload); err != nil {
		t.Fatalf("write valid frame failed: %v", err)
	}

	select {
	case <-pc.Done():
		t.Fatalf("connection should remain open after valid frame reset")
	case <-time.After(120 * time.Millisecond):
	}

	for i := 0; i < 2; i++ {
		if err := WriteFrame(remoteConn, []byte("bad-frame")); err != nil {
			t.Fatalf("write malformed frame after reset failed: %v", err)
		}
	}
	select {
	case <-pc.Done():
		t.Fatalf("connection closed before third malformed frame after reset")
	case <-time.After(80 * time.Millisecond):
	}

	if err := WriteFrame(remoteConn, []byte("bad-frame")); err != nil {
		t.Fatalf("write final malformed frame failed: %v", err)
	}
	select {
	case <-pc.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("expected connection close after third malformed frame after reset")
	}
}

func buildSecureTestFramePayload(sessionKey []byte, message any) ([]byte, error) {
	plaintext, err := EncodeJSON(message)
	if err != nil {
		return nil, err
	}
	ciphertext, nonce, err := appcrypto.Encrypt(sessionKey, plaintext)
	if err != nil {
		return nil, err
	}

	frame := secureFrame{
		Type:       secureFrameType,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	return json.Marshal(frame)
}
