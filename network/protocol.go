package network

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"gosend/crypto"
)

const (
	// ProtocolVersion is the current wire protocol version.
	ProtocolVersion = 1
	// MaxFrameSize is the maximum accepted frame payload size (10 MB).
	MaxFrameSize = 10 * 1024 * 1024
	// DefaultConnectionTimeout bounds TCP dial/handshake duration.
	DefaultConnectionTimeout = 30 * time.Second
	// DefaultKeepAliveInterval sends ping on idle connections.
	DefaultKeepAliveInterval = 60 * time.Second
	// DefaultKeepAliveTimeout waits this long for pong after ping.
	DefaultKeepAliveTimeout = 15 * time.Second
	// DefaultFrameReadTimeout bounds each frame read.
	DefaultFrameReadTimeout = 30 * time.Second
)

const (
	TypeHandshake         = "handshake"
	TypeHandshakeResponse = "handshake_response"
	TypePeerAddRequest    = "peer_add_request"
	TypePeerAddResponse   = "peer_add_response"
	TypePeerRemove        = "peer_remove"
	TypePeerDisconnect    = "peer_disconnect"
	TypePing              = "ping"
	TypePong              = "pong"
	TypeMessage           = "message"
	TypeFileRequest       = "file_request"
	TypeFileResponse      = "file_response"
	TypeFileData          = "file_data"
	TypeFileComplete      = "file_complete"
	TypeAck               = "ack"
	TypeError             = "error"
)

var (
	// ErrFrameTooLarge indicates payload exceeds MaxFrameSize.
	ErrFrameTooLarge = errors.New("network: frame exceeds max size")
	// ErrUnsupportedVersion indicates protocol version mismatch.
	ErrUnsupportedVersion = errors.New("network: unsupported protocol version")
	// ErrInvalidSignature indicates signature verification failed.
	ErrInvalidSignature = errors.New("network: invalid signature")
	// ErrInvalidMessageType indicates the message type is missing or unknown.
	ErrInvalidMessageType = errors.New("network: invalid message type")
)

// LocalIdentity contains local device values required to build handshake messages.
type LocalIdentity struct {
	DeviceID          string
	DeviceName        string
	Ed25519PrivateKey ed25519.PrivateKey
	Ed25519PublicKey  ed25519.PublicKey
}

// Envelope identifies the protocol message type.
type Envelope struct {
	Type string `json:"type"`
}

// HandshakeMessage is the initial connection handshake payload.
type HandshakeMessage struct {
	Type             string `json:"type"`
	DeviceID         string `json:"device_id"`
	DeviceName       string `json:"device_name"`
	Ed25519PublicKey string `json:"ed25519_public_key"`
	X25519PublicKey  string `json:"x25519_public_key"`
	ProtocolVersion  int    `json:"protocol_version"`
	Timestamp        int64  `json:"timestamp"`
	Signature        string `json:"signature"`
}

// HandshakeResponse is returned by the receiving side of the handshake.
type HandshakeResponse struct {
	Type             string `json:"type"`
	DeviceID         string `json:"device_id"`
	DeviceName       string `json:"device_name"`
	Ed25519PublicKey string `json:"ed25519_public_key"`
	X25519PublicKey  string `json:"x25519_public_key"`
	ProtocolVersion  int    `json:"protocol_version"`
	Timestamp        int64  `json:"timestamp"`
	Signature        string `json:"signature"`
}

// PeerAddRequest requests adding a peer.
type PeerAddRequest struct {
	Type           string `json:"type"`
	FromDeviceID   string `json:"from_device_id"`
	FromDeviceName string `json:"from_device_name"`
	Timestamp      int64  `json:"timestamp"`
	Signature      string `json:"signature"`
}

// PeerAddResponse responds to a peer add request.
type PeerAddResponse struct {
	Type       string `json:"type"`
	Status     string `json:"status"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Timestamp  int64  `json:"timestamp"`
	Signature  string `json:"signature"`
}

// PeerRemove notifies peer removal.
type PeerRemove struct {
	Type         string `json:"type"`
	FromDeviceID string `json:"from_device_id"`
	Timestamp    int64  `json:"timestamp"`
	Signature    string `json:"signature"`
}

// PeerDisconnect signals graceful disconnect.
type PeerDisconnect struct {
	Type         string `json:"type"`
	FromDeviceID string `json:"from_device_id"`
	Timestamp    int64  `json:"timestamp"`
}

// PingMessage is a keep-alive ping.
type PingMessage struct {
	Type         string `json:"type"`
	FromDeviceID string `json:"from_device_id"`
	Timestamp    int64  `json:"timestamp"`
}

// PongMessage is a keep-alive pong response.
type PongMessage struct {
	Type         string `json:"type"`
	FromDeviceID string `json:"from_device_id"`
	Timestamp    int64  `json:"timestamp"`
}

// EncryptedMessage is the wire format for encrypted chat payloads.
type EncryptedMessage struct {
	Type             string `json:"type"`
	MessageID        string `json:"message_id"`
	FromDeviceID     string `json:"from_device_id"`
	ToDeviceID       string `json:"to_device_id"`
	ContentType      string `json:"content_type"`
	Sequence         uint64 `json:"sequence"`
	EncryptedContent string `json:"encrypted_content"`
	IV               string `json:"iv"`
	Timestamp        int64  `json:"timestamp"`
	Signature        string `json:"signature"`
}

// FileRequest starts a file transfer.
type FileRequest struct {
	Type         string `json:"type"`
	FileID       string `json:"file_id"`
	FromDeviceID string `json:"from_device_id"`
	ToDeviceID   string `json:"to_device_id"`
	Filename     string `json:"filename"`
	Filesize     int64  `json:"filesize"`
	Filetype     string `json:"filetype"`
	Checksum     string `json:"checksum"`
	Sequence     uint64 `json:"sequence"`
	Timestamp    int64  `json:"timestamp"`
	Signature    string `json:"signature"`
}

// FileResponse accepts or rejects a transfer.
type FileResponse struct {
	Type            string `json:"type"`
	FileID          string `json:"file_id"`
	Status          string `json:"status"`
	FromDeviceID    string `json:"from_device_id"`
	ChunkIndex      int    `json:"chunk_index,omitempty"`
	ResumeFromChunk int    `json:"resume_from_chunk,omitempty"`
	Message         string `json:"message,omitempty"`
	Timestamp       int64  `json:"timestamp"`
	Signature       string `json:"signature"`
}

// FileData contains one encrypted chunk.
type FileData struct {
	Type          string `json:"type"`
	FileID        string `json:"file_id"`
	ChunkIndex    int    `json:"chunk_index"`
	TotalChunks   int    `json:"total_chunks"`
	ChunkSize     int    `json:"chunk_size"`
	EncryptedData string `json:"encrypted_data"`
	IV            string `json:"iv"`
	Timestamp     int64  `json:"timestamp"`
}

// FileComplete indicates transfer completion status.
type FileComplete struct {
	Type      string `json:"type"`
	FileID    string `json:"file_id"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// AckMessage confirms message delivery.
type AckMessage struct {
	Type         string `json:"type"`
	MessageID    string `json:"message_id"`
	FromDeviceID string `json:"from_device_id"`
	Status       string `json:"status"`
	Timestamp    int64  `json:"timestamp"`
}

// ErrorMessage reports protocol errors.
type ErrorMessage struct {
	Type              string `json:"type"`
	Code              string `json:"code"`
	Message           string `json:"message"`
	RelatedMessageID  string `json:"related_message_id,omitempty"`
	SupportedVersions []int  `json:"supported_versions,omitempty"`
	Timestamp         int64  `json:"timestamp"`
}

// EncodeJSON marshals a protocol message to JSON.
func EncodeJSON(message any) ([]byte, error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("marshal protocol message: %w", err)
	}
	return payload, nil
}

// DecodeMessageType extracts the "type" field from a payload.
func DecodeMessageType(payload []byte) (string, error) {
	var envelope Envelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("decode envelope: %w", err)
	}
	if envelope.Type == "" {
		return "", ErrInvalidMessageType
	}
	return envelope.Type, nil
}

// WriteFrame writes one length-prefixed frame.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return ErrFrameTooLarge
	}

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write frame length: %w", err)
	}
	if len(payload) == 0 {
		return nil
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}

	return nil
}

// ReadFrame reads one length-prefixed frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read frame length: %w", err)
	}

	length := binary.BigEndian.Uint32(header)
	if length > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	if length == 0 {
		return []byte{}, nil
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read frame payload: %w", err)
	}

	return payload, nil
}

// ReadFrameWithTimeout reads a frame with an optional read deadline.
func ReadFrameWithTimeout(conn net.Conn, timeout time.Duration) ([]byte, error) {
	if timeout > 0 {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return nil, fmt.Errorf("set read deadline: %w", err)
		}
		defer func() {
			_ = conn.SetReadDeadline(time.Time{})
		}()
	}
	return ReadFrame(conn)
}

func buildHandshakeMessage(identity LocalIdentity, ephemeralPublicKey []byte, msgType string) (HandshakeMessage, error) {
	if len(identity.Ed25519PrivateKey) != ed25519.PrivateKeySize {
		return HandshakeMessage{}, errors.New("invalid local Ed25519 private key")
	}
	if len(identity.Ed25519PublicKey) != ed25519.PublicKeySize {
		return HandshakeMessage{}, errors.New("invalid local Ed25519 public key")
	}

	msg := HandshakeMessage{
		Type:             msgType,
		DeviceID:         identity.DeviceID,
		DeviceName:       identity.DeviceName,
		Ed25519PublicKey: base64.StdEncoding.EncodeToString(identity.Ed25519PublicKey),
		X25519PublicKey:  base64.StdEncoding.EncodeToString(ephemeralPublicKey),
		ProtocolVersion:  ProtocolVersion,
		Timestamp:        time.Now().UnixMilli(),
	}

	signature, err := signHandshake(msg, identity.Ed25519PrivateKey)
	if err != nil {
		return HandshakeMessage{}, err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return msg, nil
}

// BuildHandshakeMessage builds and signs a handshake message.
func BuildHandshakeMessage(identity LocalIdentity, ephemeralPublicKey []byte) (HandshakeMessage, error) {
	return buildHandshakeMessage(identity, ephemeralPublicKey, TypeHandshake)
}

// BuildHandshakeResponse builds and signs a handshake response.
func BuildHandshakeResponse(identity LocalIdentity, ephemeralPublicKey []byte) (HandshakeResponse, error) {
	msg, err := buildHandshakeMessage(identity, ephemeralPublicKey, TypeHandshakeResponse)
	if err != nil {
		return HandshakeResponse{}, err
	}

	return HandshakeResponse{
		Type:             msg.Type,
		DeviceID:         msg.DeviceID,
		DeviceName:       msg.DeviceName,
		Ed25519PublicKey: msg.Ed25519PublicKey,
		X25519PublicKey:  msg.X25519PublicKey,
		ProtocolVersion:  msg.ProtocolVersion,
		Timestamp:        msg.Timestamp,
		Signature:        msg.Signature,
	}, nil
}

// VerifyHandshakeMessage verifies the signature and protocol version for a handshake.
func VerifyHandshakeMessage(msg HandshakeMessage) (ed25519.PublicKey, error) {
	return verifyHandshake(msg)
}

// VerifyHandshakeResponse verifies the signature and protocol version for a response.
func VerifyHandshakeResponse(msg HandshakeResponse) (ed25519.PublicKey, error) {
	return verifyHandshake(HandshakeMessage{
		Type:             msg.Type,
		DeviceID:         msg.DeviceID,
		DeviceName:       msg.DeviceName,
		Ed25519PublicKey: msg.Ed25519PublicKey,
		X25519PublicKey:  msg.X25519PublicKey,
		ProtocolVersion:  msg.ProtocolVersion,
		Timestamp:        msg.Timestamp,
		Signature:        msg.Signature,
	})
}

func verifyHandshake(msg HandshakeMessage) (ed25519.PublicKey, error) {
	if msg.ProtocolVersion != ProtocolVersion {
		return nil, ErrUnsupportedVersion
	}

	publicKeyBytes, err := base64.StdEncoding.DecodeString(msg.Ed25519PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode Ed25519 public key: %w", err)
	}
	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key length")
	}
	publicKey := ed25519.PublicKey(publicKeyBytes)

	signatureBytes, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return nil, fmt.Errorf("decode handshake signature: %w", err)
	}

	signaturePayload := msg
	signaturePayload.Signature = ""
	signable, err := json.Marshal(signaturePayload)
	if err != nil {
		return nil, fmt.Errorf("marshal handshake signable payload: %w", err)
	}
	if !crypto.Verify(publicKey, signable, signatureBytes) {
		return nil, ErrInvalidSignature
	}

	return publicKey, nil
}

func signHandshake(msg HandshakeMessage, privateKey ed25519.PrivateKey) ([]byte, error) {
	signaturePayload := msg
	signaturePayload.Signature = ""
	signable, err := json.Marshal(signaturePayload)
	if err != nil {
		return nil, fmt.Errorf("marshal handshake signable payload: %w", err)
	}

	signature, err := crypto.Sign(privateKey, signable)
	if err != nil {
		return nil, fmt.Errorf("sign handshake payload: %w", err)
	}
	return signature, nil
}
