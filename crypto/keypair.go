package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const (
	ed25519PrivatePEMType = "ED25519 PRIVATE KEY"
	ed25519PublicPEMType  = "ED25519 PUBLIC KEY"
)

// EnsureEd25519KeyPair loads an Ed25519 keypair from disk, generating it on first run.
func EnsureEd25519KeyPair(privatePath, publicPath string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	privateKey, err := LoadEd25519PrivateKey(privatePath)
	if err == nil {
		publicKey := privateKey.Public().(ed25519.PublicKey)

		storedPublic, pubErr := LoadEd25519PublicKey(publicPath)
		if pubErr != nil || !bytes.Equal(storedPublic, publicKey) {
			if err := SaveEd25519PublicKey(publicPath, publicKey); err != nil {
				return nil, nil, err
			}
		}

		return privateKey, publicKey, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate Ed25519 keypair: %w", err)
	}

	if err := SaveEd25519PrivateKey(privatePath, privateKey); err != nil {
		return nil, nil, err
	}
	if err := SaveEd25519PublicKey(publicPath, publicKey); err != nil {
		return nil, nil, err
	}

	return privateKey, publicKey, nil
}

// LoadEd25519PrivateKey loads an Ed25519 private key from a PEM file.
func LoadEd25519PrivateKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Ed25519 private key: %w", err)
	}

	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("decode Ed25519 private PEM: no PEM block")
	}
	if block.Type != ed25519PrivatePEMType {
		return nil, fmt.Errorf("decode Ed25519 private PEM: unexpected type %q", block.Type)
	}
	if len(block.Bytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("decode Ed25519 private PEM: invalid key size %d", len(block.Bytes))
	}

	return ed25519.PrivateKey(block.Bytes), nil
}

// LoadEd25519PublicKey loads an Ed25519 public key from a PEM file.
func LoadEd25519PublicKey(path string) (ed25519.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Ed25519 public key: %w", err)
	}

	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("decode Ed25519 public PEM: no PEM block")
	}
	if block.Type != ed25519PublicPEMType {
		return nil, fmt.Errorf("decode Ed25519 public PEM: unexpected type %q", block.Type)
	}
	if len(block.Bytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("decode Ed25519 public PEM: invalid key size %d", len(block.Bytes))
	}

	return ed25519.PublicKey(block.Bytes), nil
}

// SaveEd25519PrivateKey writes an Ed25519 private key PEM file with 0600 permissions.
func SaveEd25519PrivateKey(path string, key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("save Ed25519 private key: invalid key size %d", len(key))
	}

	block := &pem.Block{
		Type:  ed25519PrivatePEMType,
		Bytes: key,
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		return fmt.Errorf("write Ed25519 private key: %w", err)
	}

	return nil
}

// SaveEd25519PublicKey writes an Ed25519 public key PEM file.
func SaveEd25519PublicKey(path string, key ed25519.PublicKey) error {
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("save Ed25519 public key: invalid key size %d", len(key))
	}

	block := &pem.Block{
		Type:  ed25519PublicPEMType,
		Bytes: key,
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o644); err != nil {
		return fmt.Errorf("write Ed25519 public key: %w", err)
	}

	return nil
}

// KeyFingerprint returns the truncated SHA-256 hex fingerprint of a public key.
func KeyFingerprint(publicKey ed25519.PublicKey) string {
	sum := sha256.Sum256(publicKey)
	return hex.EncodeToString(sum[:16])
}

// FormatFingerprint returns fingerprint text grouped in chunks of 4 uppercase chars.
func FormatFingerprint(fingerprint string) string {
	clean := strings.ToUpper(strings.ReplaceAll(fingerprint, " ", ""))
	if clean == "" {
		return ""
	}

	var b strings.Builder
	for i := 0; i < len(clean); i += 4 {
		if i > 0 {
			b.WriteByte(' ')
		}

		end := i + 4
		if end > len(clean) {
			end = len(clean)
		}
		b.WriteString(clean[i:end])
	}

	return b.String()
}
