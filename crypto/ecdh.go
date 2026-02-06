package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const x25519PrivatePEMType = "X25519 PRIVATE KEY"

var x25519Curve = ecdh.X25519()

// EnsureX25519PrivateKey loads an X25519 private key from disk, generating it if absent.
func EnsureX25519PrivateKey(path string) (*ecdh.PrivateKey, error) {
	privateKey, err := LoadX25519PrivateKey(path)
	if err == nil {
		return privateKey, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	privateKey, err = GenerateX25519PrivateKey()
	if err != nil {
		return nil, err
	}
	if err := SaveX25519PrivateKey(path, privateKey); err != nil {
		return nil, err
	}

	return privateKey, nil
}

// GenerateX25519PrivateKey creates a new X25519 private key.
func GenerateX25519PrivateKey() (*ecdh.PrivateKey, error) {
	privateKey, err := x25519Curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate X25519 private key: %w", err)
	}
	return privateKey, nil
}

// LoadX25519PrivateKey reads an X25519 private key from PEM.
func LoadX25519PrivateKey(path string) (*ecdh.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read X25519 private key: %w", err)
	}

	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("decode X25519 PEM: no PEM block")
	}
	if block.Type != x25519PrivatePEMType {
		return nil, fmt.Errorf("decode X25519 PEM: unexpected type %q", block.Type)
	}
	if len(block.Bytes) != 32 {
		return nil, fmt.Errorf("decode X25519 PEM: invalid private key size %d", len(block.Bytes))
	}

	privateKey, err := x25519Curve.NewPrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse X25519 private key: %w", err)
	}

	return privateKey, nil
}

// SaveX25519PrivateKey writes an X25519 private key PEM file with 0600 permissions.
func SaveX25519PrivateKey(path string, key *ecdh.PrivateKey) error {
	block := &pem.Block{
		Type:  x25519PrivatePEMType,
		Bytes: key.Bytes(),
	}

	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return fmt.Errorf("write X25519 private key: %w", err)
	}

	return nil
}
