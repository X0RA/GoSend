package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const sessionHKDFSalt = "p2pchat-session-v1"
const sessionKeyBytes = 32

var x25519Curve = ecdh.X25519()

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
	return DeriveSessionKeyWithContext(sharedSecret, deviceIDa, deviceIDb, nil)
}

// DeriveSessionKeyWithContext derives a 32-byte AES session key with additional HKDF info context.
func DeriveSessionKeyWithContext(sharedSecret []byte, deviceIDa, deviceIDb string, context []byte) ([]byte, error) {
	if len(sharedSecret) == 0 {
		return nil, errors.New("shared secret is required")
	}
	if strings.TrimSpace(deviceIDa) == "" || strings.TrimSpace(deviceIDb) == "" {
		return nil, errors.New("both device IDs are required")
	}

	info := sessionInfo(deviceIDa, deviceIDb, context)
	reader := hkdf.New(sha256.New, sharedSecret, []byte(sessionHKDFSalt), info)

	key := make([]byte, sessionKeyBytes)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, fmt.Errorf("derive session key: %w", err)
	}

	return key, nil
}

func sessionInfo(deviceIDa, deviceIDb string, context []byte) []byte {
	ids := []string{deviceIDa, deviceIDb}
	sort.Strings(ids)
	info := ids[0] + "|" + ids[1]
	if len(context) == 0 {
		return []byte(info)
	}
	combined := make([]byte, 0, len(info)+1+len(context))
	combined = append(combined, []byte(info)...)
	combined = append(combined, '|')
	combined = append(combined, context...)
	return combined
}
