package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	sessionKey := make([]byte, 32)
	if _, err := rand.Read(sessionKey); err != nil {
		t.Fatalf("generate session key: %v", err)
	}

	plaintext := []byte(`{"type":"text","content":"hello world"}`)

	ciphertext, iv, err := Encrypt(sessionKey, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if len(iv) != 12 {
		t.Fatalf("expected 12-byte IV, got %d", len(iv))
	}
	if len(ciphertext) == 0 {
		t.Fatalf("expected non-empty ciphertext")
	}

	decrypted, err := Decrypt(sessionKey, iv, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted plaintext does not match original")
	}
}
