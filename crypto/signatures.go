package crypto

import (
	"crypto/ed25519"
	"errors"
	"fmt"
)

// Sign signs data using an Ed25519 private key.
func Sign(privateKey ed25519.PrivateKey, data []byte) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid Ed25519 private key length: got %d want %d", len(privateKey), ed25519.PrivateKeySize)
	}
	if len(data) == 0 {
		return nil, errors.New("data is required")
	}

	return ed25519.Sign(privateKey, data), nil
}

// Verify verifies an Ed25519 signature.
func Verify(publicKey ed25519.PublicKey, data, signature []byte) bool {
	if len(publicKey) != ed25519.PublicKeySize {
		return false
	}
	if len(data) == 0 {
		return false
	}
	if len(signature) != ed25519.SignatureSize {
		return false
	}

	return ed25519.Verify(publicKey, data, signature)
}
