package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const x25519PrivatePEMType = "X25519 PRIVATE KEY"
const sessionHKDFSalt = "p2pchat-session-v1"
const sessionKeyBytes = 32

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

// GenerateEphemeralX25519KeyPair creates a non-persistent X25519 keypair for one session.
func GenerateEphemeralX25519KeyPair() (*ecdh.PrivateKey, *ecdh.PublicKey, error) {
	privateKey, err := GenerateX25519PrivateKey()
	if err != nil {
		return nil, nil, err
	}
	return privateKey, privateKey.PublicKey(), nil
}

// ParseX25519PublicKey parses raw X25519 public key bytes.
func ParseX25519PublicKey(raw []byte) (*ecdh.PublicKey, error) {
	publicKey, err := x25519Curve.NewPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse X25519 public key: %w", err)
	}
	return publicKey, nil
}

// ComputeX25519SharedSecret computes the ECDH shared secret for two peers.
func ComputeX25519SharedSecret(privateKey *ecdh.PrivateKey, peerPublicKey *ecdh.PublicKey) ([]byte, error) {
	if privateKey == nil {
		return nil, errors.New("private key is required")
	}
	if peerPublicKey == nil {
		return nil, errors.New("peer public key is required")
	}

	secret, err := privateKey.ECDH(peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("compute X25519 shared secret: %w", err)
	}

	return secret, nil
}

// DeriveSessionKey derives a 32-byte AES session key from an X25519 shared secret.
func DeriveSessionKey(sharedSecret []byte, deviceIDa, deviceIDb string) ([]byte, error) {
	if len(sharedSecret) == 0 {
		return nil, errors.New("shared secret is required")
	}
	if strings.TrimSpace(deviceIDa) == "" || strings.TrimSpace(deviceIDb) == "" {
		return nil, errors.New("both device IDs are required")
	}

	info := sessionInfo(deviceIDa, deviceIDb)
	reader := hkdf.New(sha256.New, sharedSecret, []byte(sessionHKDFSalt), info)

	key := make([]byte, sessionKeyBytes)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive session key: %w", err)
	}

	return key, nil
}

func sessionInfo(deviceIDa, deviceIDb string) []byte {
	ids := []string{deviceIDa, deviceIDb}
	sort.Strings(ids)
	return []byte(ids[0] + "|" + ids[1])
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
