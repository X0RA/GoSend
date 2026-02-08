package network

import (
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gosend/crypto"
)

var (
	// ErrKeyChanged indicates a known peer presented a different public key.
	ErrKeyChanged = errors.New("network: peer public key changed")
)

// KeyChangeDecisionFunc blocks handshake progression until trust decision is made.
type KeyChangeDecisionFunc func(peerDeviceID, existingPublicKeyBase64, receivedPublicKeyBase64 string) (bool, error)

// HandshakeOptions configures handshake verification and connection behavior.
type HandshakeOptions struct {
	Identity            LocalIdentity
	KnownPeerKeys       map[string]string
	OnKeyChangeDecision KeyChangeDecisionFunc

	ConnectionTimeout time.Duration
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
	FrameReadTimeout  time.Duration
	RekeyInterval     time.Duration
	RekeyAfterBytes   uint64
	AutoRespondPing   *bool
}

func (o HandshakeOptions) withDefaults() HandshakeOptions {
	out := o
	if out.ConnectionTimeout <= 0 {
		out.ConnectionTimeout = DefaultConnectionTimeout
	}
	if out.KeepAliveInterval <= 0 {
		out.KeepAliveInterval = DefaultKeepAliveInterval
	}
	if out.KeepAliveTimeout <= 0 {
		out.KeepAliveTimeout = DefaultKeepAliveTimeout
	}
	if out.FrameReadTimeout <= 0 {
		out.FrameReadTimeout = DefaultFrameReadTimeout
	}
	if out.RekeyInterval <= 0 {
		out.RekeyInterval = defaultRekeyInterval
	}
	if out.RekeyAfterBytes == 0 {
		out.RekeyAfterBytes = defaultRekeyAfterBytes
	}
	return out
}

func (o HandshakeOptions) validateIdentity() error {
	if o.Identity.DeviceID == "" {
		return errors.New("local device ID is required")
	}
	if o.Identity.DeviceName == "" {
		return errors.New("local device name is required")
	}
	if len(o.Identity.Ed25519PrivateKey) == 0 {
		return errors.New("local Ed25519 private key is required")
	}
	if len(o.Identity.Ed25519PublicKey) == 0 {
		return errors.New("local Ed25519 public key is required")
	}
	return nil
}

func (o HandshakeOptions) autoRespondPingEnabled() bool {
	if o.AutoRespondPing == nil {
		return true
	}
	return *o.AutoRespondPing
}

func deriveSessionKey(localEphemeralPrivateKey *ecdh.PrivateKey, peerX25519PublicKeyBase64, localDeviceID, peerDeviceID, challengeNonceBase64 string) ([]byte, error) {
	peerPublicRaw, err := base64.StdEncoding.DecodeString(peerX25519PublicKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode peer ephemeral public key: %w", err)
	}
	peerPublicKey, err := crypto.ParseX25519PublicKey(peerPublicRaw)
	if err != nil {
		return nil, err
	}

	sharedSecret, err := crypto.ComputeX25519SharedSecret(localEphemeralPrivateKey, peerPublicKey)
	if err != nil {
		return nil, err
	}

	challengeNonce, err := base64.StdEncoding.DecodeString(challengeNonceBase64)
	if err != nil {
		return nil, fmt.Errorf("decode challenge nonce: %w", err)
	}
	if len(challengeNonce) != 32 {
		return nil, fmt.Errorf("invalid challenge nonce length: got %d want %d", len(challengeNonce), 32)
	}

	return crypto.DeriveSessionKeyWithContext(sharedSecret, localDeviceID, peerDeviceID, challengeNonce)
}

func deriveRekeySessionKey(localEphemeralPrivateKey *ecdh.PrivateKey, peerX25519PublicKeyBase64, localDeviceID, peerDeviceID string, epoch uint64) ([]byte, error) {
	peerPublicRaw, err := base64.StdEncoding.DecodeString(peerX25519PublicKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode peer rekey public key: %w", err)
	}
	peerPublicKey, err := crypto.ParseX25519PublicKey(peerPublicRaw)
	if err != nil {
		return nil, err
	}

	sharedSecret, err := crypto.ComputeX25519SharedSecret(localEphemeralPrivateKey, peerPublicKey)
	if err != nil {
		return nil, err
	}

	rekeyContext := []byte("rekey|" + strconv.FormatUint(epoch, 10))
	return crypto.DeriveSessionKeyWithContext(sharedSecret, localDeviceID, peerDeviceID, rekeyContext)
}

func evaluatePeerKey(deviceID, receivedBase64 string, known map[string]string, decision KeyChangeDecisionFunc) error {
	if known == nil {
		return nil
	}

	existing := known[deviceID]
	if existing == "" || existing == receivedBase64 {
		return nil
	}

	if decision == nil {
		return ErrKeyChanged
	}

	trust, err := decision(deviceID, existing, receivedBase64)
	if err != nil {
		return fmt.Errorf("key change decision for peer %q: %w", deviceID, err)
	}
	if !trust {
		return ErrKeyChanged
	}
	return nil
}

func makeVersionMismatchError(got int64) ErrorMessage {
	return ErrorMessage{
		Type:              TypeError,
		Code:              "version_mismatch",
		Message:           fmt.Sprintf("Unsupported protocol version. Expected %d, got %d.", ProtocolVersion, got),
		SupportedVersions: []int{ProtocolVersion},
		Timestamp:         time.Now().UnixMilli(),
	}
}
