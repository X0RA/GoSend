package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

const aes256KeySize = 32

// Encrypt encrypts plaintext with AES-256-GCM and returns ciphertext and IV.
func Encrypt(sessionKey, plaintext []byte) (ciphertext, iv []byte, err error) {
	if len(sessionKey) != aes256KeySize {
		return nil, nil, fmt.Errorf("invalid session key length: got %d want %d", len(sessionKey), aes256KeySize)
	}

	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("create GCM: %w", err)
	}

	iv = make([]byte, aead.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return nil, nil, fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext = aead.Seal(nil, iv, plaintext, nil)
	return ciphertext, iv, nil
}

// Decrypt decrypts AES-256-GCM ciphertext using the provided IV.
func Decrypt(sessionKey, iv, ciphertext []byte) ([]byte, error) {
	if len(sessionKey) != aes256KeySize {
		return nil, fmt.Errorf("invalid session key length: got %d want %d", len(sessionKey), aes256KeySize)
	}
	if len(ciphertext) == 0 {
		return nil, errors.New("ciphertext is required")
	}

	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	if len(iv) != aead.NonceSize() {
		return nil, fmt.Errorf("invalid nonce length: got %d want %d", len(iv), aead.NonceSize())
	}

	plaintext, err := aead.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt ciphertext: %w", err)
	}

	return plaintext, nil
}
