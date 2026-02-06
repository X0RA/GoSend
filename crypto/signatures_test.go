package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestSignatureValidity(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 keypair: %v", err)
	}

	data := []byte("signed payload")
	signature, err := Sign(privateKey, data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}
	if !Verify(publicKey, data, signature) {
		t.Fatalf("expected signature verification to succeed")
	}
}

func TestSignatureTamperingRejected(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate Ed25519 keypair: %v", err)
	}

	originalData := []byte("message to protect")
	signature, err := Sign(privateKey, originalData)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	tamperedData := []byte("message to protect!")
	if Verify(publicKey, tamperedData, signature) {
		t.Fatalf("expected signature verification to fail for tampered data")
	}
}
