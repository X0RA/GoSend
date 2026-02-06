package network

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"gosend/crypto"
)

// Dial connects to a peer, performs handshake, and returns a ready PeerConnection.
func Dial(address string, options HandshakeOptions) (*PeerConnection, error) {
	opts := options.withDefaults()
	if err := opts.validateIdentity(); err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", address, opts.ConnectionTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial %q: %w", address, err)
	}

	if err := conn.SetDeadline(time.Now().Add(opts.ConnectionTimeout)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("set handshake deadline: %w", err)
	}

	localEphemeralPrivateKey, localEphemeralPublicKey, err := crypto.GenerateEphemeralX25519KeyPair()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	handshake, err := BuildHandshakeMessage(opts.Identity, localEphemeralPublicKey.Bytes())
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	payload, err := EncodeJSON(handshake)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := WriteFrame(conn, payload); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send handshake: %w", err)
	}

	responsePayload, err := ReadFrameWithTimeout(conn, opts.ConnectionTimeout)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read handshake response: %w", err)
	}

	msgType, err := DecodeMessageType(responsePayload)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if msgType == TypeError {
		remoteErr := ErrorMessage{}
		if err := json.Unmarshal(responsePayload, &remoteErr); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("decode remote error response: %w", err)
		}
		_ = conn.Close()
		return nil, fmt.Errorf("remote error [%s]: %s", remoteErr.Code, remoteErr.Message)
	}
	if msgType != TypeHandshakeResponse {
		_ = conn.Close()
		return nil, fmt.Errorf("expected %q, got %q", TypeHandshakeResponse, msgType)
	}

	response, err := decodeHandshakeResponse(responsePayload)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err := VerifyHandshakeResponse(response); err != nil {
		_ = conn.Close()
		if errors.Is(err, ErrUnsupportedVersion) {
			return nil, err
		}
		return nil, fmt.Errorf("verify handshake response: %w", err)
	}

	if err := evaluatePeerKey(response.DeviceID, response.Ed25519PublicKey, opts.KnownPeerKeys, opts.OnKeyChangeDecision); err != nil {
		_ = conn.Close()
		return nil, err
	}

	sessionKey, err := deriveSessionKey(localEphemeralPrivateKey, response.X25519PublicKey, opts.Identity.DeviceID, response.DeviceID)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("clear handshake deadline: %w", err)
	}

	connection := newPeerConnection(conn, sessionKey, ConnectionOptions{
		LocalDeviceID:     opts.Identity.DeviceID,
		PeerDeviceID:      response.DeviceID,
		PeerDeviceName:    response.DeviceName,
		PeerPublicKey:     response.Ed25519PublicKey,
		KeepAliveInterval: opts.KeepAliveInterval,
		KeepAliveTimeout:  opts.KeepAliveTimeout,
		FrameReadTimeout:  opts.FrameReadTimeout,
		AutoRespondPing:   opts.autoRespondPingEnabled(),
	})

	return connection, nil
}
