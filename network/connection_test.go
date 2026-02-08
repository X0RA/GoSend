package network

import (
	"bytes"
	"context"
	"encoding/base64"
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
