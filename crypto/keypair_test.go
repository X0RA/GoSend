package crypto

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestEnsureEd25519KeyPairIsStable(t *testing.T) {
	tempDir := t.TempDir()
	privatePath := filepath.Join(tempDir, "ed25519_private.pem")
	publicPath := filepath.Join(tempDir, "ed25519_public.pem")

	firstPrivate, firstPublic, err := EnsureEd25519KeyPair(privatePath, publicPath)
	if err != nil {
		t.Fatalf("first EnsureEd25519KeyPair failed: %v", err)
	}

	secondPrivate, secondPublic, err := EnsureEd25519KeyPair(privatePath, publicPath)
	if err != nil {
		t.Fatalf("second EnsureEd25519KeyPair failed: %v", err)
	}

	if !bytes.Equal(firstPrivate, secondPrivate) {
		t.Fatalf("expected stable private key across runs")
	}
	if !bytes.Equal(firstPublic, secondPublic) {
		t.Fatalf("expected stable public key across runs")
	}
}

func TestEnsureX25519PrivateKeyIsStable(t *testing.T) {
	tempDir := t.TempDir()
	privatePath := filepath.Join(tempDir, "x25519_private.pem")

	firstPrivate, err := EnsureX25519PrivateKey(privatePath)
	if err != nil {
		t.Fatalf("first EnsureX25519PrivateKey failed: %v", err)
	}

	secondPrivate, err := EnsureX25519PrivateKey(privatePath)
	if err != nil {
		t.Fatalf("second EnsureX25519PrivateKey failed: %v", err)
	}

	if !bytes.Equal(firstPrivate.Bytes(), secondPrivate.Bytes()) {
		t.Fatalf("expected stable X25519 private key across runs")
	}
}
