package crypto

import (
	"bytes"
	"testing"
)

func TestSessionKeyDerivationMatchesAcrossPeers(t *testing.T) {
	alicePrivate, alicePublic, err := GenerateEphemeralX25519KeyPair()
	if err != nil {
		t.Fatalf("generate alice ephemeral keypair: %v", err)
	}
	bobPrivate, bobPublic, err := GenerateEphemeralX25519KeyPair()
	if err != nil {
		t.Fatalf("generate bob ephemeral keypair: %v", err)
	}

	aliceShared, err := ComputeX25519SharedSecret(alicePrivate, bobPublic)
	if err != nil {
		t.Fatalf("compute alice shared secret: %v", err)
	}
	bobShared, err := ComputeX25519SharedSecret(bobPrivate, alicePublic)
	if err != nil {
		t.Fatalf("compute bob shared secret: %v", err)
	}

	if !bytes.Equal(aliceShared, bobShared) {
		t.Fatalf("expected matching shared secrets")
	}

	aliceSessionKey, err := DeriveSessionKey(aliceShared, "alice-device", "bob-device")
	if err != nil {
		t.Fatalf("derive alice session key: %v", err)
	}
	bobSessionKey, err := DeriveSessionKey(bobShared, "bob-device", "alice-device")
	if err != nil {
		t.Fatalf("derive bob session key: %v", err)
	}

	if len(aliceSessionKey) != 32 {
		t.Fatalf("expected 32-byte session key, got %d", len(aliceSessionKey))
	}
	if !bytes.Equal(aliceSessionKey, bobSessionKey) {
		t.Fatalf("expected matching session keys")
	}
}
