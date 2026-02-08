package network

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	appcrypto "gosend/crypto"
)

var (
	// ErrSequenceReplay indicates a non-monotonic sequence value.
	ErrSequenceReplay = errors.New("network: sequence replay detected")
	// ErrPongTimeout indicates keep-alive timed out waiting for pong.
	ErrPongTimeout = errors.New("network: pong timeout")
	// ErrInvalidSecureFrame indicates an invalid encrypted transport frame.
	ErrInvalidSecureFrame = errors.New("network: invalid secure frame")
)

const (
	defaultRekeyInterval   = time.Hour
	defaultRekeyAfterBytes = uint64(1 << 30) // 1 GiB
)

// ConnectionState represents the lifecycle state of one peer connection.
type ConnectionState string

const (
	StateConnecting    ConnectionState = "CONNECTING"
	StateReady         ConnectionState = "READY"
	StateIdle          ConnectionState = "IDLE"
	StateDisconnecting ConnectionState = "DISCONNECTING"
	StateDisconnected  ConnectionState = "DISCONNECTED"
)

const secureFrameType = "secure_frame"

type secureFrame struct {
	Type       string `json:"type"`
	Epoch      uint64 `json:"epoch,omitempty"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// ConnectionOptions controls runtime behavior of PeerConnection.
type ConnectionOptions struct {
	LocalDeviceID     string
	PeerDeviceID      string
	PeerDeviceName    string
	PeerPublicKey     string
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
	FrameReadTimeout  time.Duration
	AutoRespondPing   bool
	RekeyInterval     time.Duration
	RekeyAfterBytes   uint64
	OnRekeyNeeded     func(*PeerConnection)
}

// PeerConnection manages a stateful framed TCP session.
type PeerConnection struct {
	conn net.Conn

	sessionMu            sync.RWMutex
	sessionKey           []byte
	sessionEpoch         uint64
	previousSessionKey   []byte
	previousSessionEpoch uint64
	hasPreviousSession   bool

	localDeviceID  string
	peerDeviceID   string
	peerDeviceName string
	peerPublicKey  string

	sendSequenceMu sync.Mutex
	sendSequence   uint64
	lastSeenSeq    uint64

	sendMu sync.Mutex

	stateMu sync.RWMutex
	state   ConnectionState

	waitMu       sync.Mutex
	waitingPong  bool
	pongDeadline time.Time

	lastActivity    atomic.Int64
	lastRekeyAt     atomic.Int64
	bytesSinceRekey atomic.Uint64

	keepAliveInterval time.Duration
	keepAliveTimeout  time.Duration
	frameReadTimeout  time.Duration
	autoRespondPing   bool
	rekeyInterval     time.Duration
	rekeyAfterBytes   uint64

	inbound chan []byte

	closeOnce sync.Once
	closed    chan struct{}

	errMu    sync.RWMutex
	closeErr error

	rekeyStateMu    sync.Mutex
	rekeyInProgress bool
	onRekeyNeeded   func(*PeerConnection)
}

func newPeerConnection(conn net.Conn, sessionKey []byte, options ConnectionOptions) *PeerConnection {
	interval := options.KeepAliveInterval
	if interval <= 0 {
		interval = DefaultKeepAliveInterval
	}

	timeout := options.KeepAliveTimeout
	if timeout <= 0 {
		timeout = DefaultKeepAliveTimeout
	}

	readTimeout := options.FrameReadTimeout
	if readTimeout <= 0 {
		readTimeout = DefaultFrameReadTimeout
	}
	rekeyInterval := options.RekeyInterval
	if rekeyInterval <= 0 {
		rekeyInterval = defaultRekeyInterval
	}
	rekeyAfterBytes := options.RekeyAfterBytes
	if rekeyAfterBytes == 0 {
		rekeyAfterBytes = defaultRekeyAfterBytes
	}

	pc := &PeerConnection{
		conn:              conn,
		sessionKey:        append([]byte(nil), sessionKey...),
		sessionEpoch:      0,
		localDeviceID:     options.LocalDeviceID,
		peerDeviceID:      options.PeerDeviceID,
		peerDeviceName:    options.PeerDeviceName,
		peerPublicKey:     options.PeerPublicKey,
		keepAliveInterval: interval,
		keepAliveTimeout:  timeout,
		frameReadTimeout:  readTimeout,
		autoRespondPing:   options.AutoRespondPing,
		rekeyInterval:     rekeyInterval,
		rekeyAfterBytes:   rekeyAfterBytes,
		inbound:           make(chan []byte, 64),
		closed:            make(chan struct{}),
		state:             StateConnecting,
		onRekeyNeeded:     options.OnRekeyNeeded,
	}

	pc.touchActivity()
	pc.lastRekeyAt.Store(time.Now().UnixNano())
	pc.setState(StateReady)
	go pc.readLoop()
	go pc.keepAliveLoop()

	return pc
}

// State returns the current connection state.
func (pc *PeerConnection) State() ConnectionState {
	pc.stateMu.RLock()
	defer pc.stateMu.RUnlock()
	return pc.state
}

// Done is closed when the connection is fully disconnected.
func (pc *PeerConnection) Done() <-chan struct{} {
	return pc.closed
}

// LastError returns the terminal connection error, if any.
func (pc *PeerConnection) LastError() error {
	pc.errMu.RLock()
	defer pc.errMu.RUnlock()
	return pc.closeErr
}

// SessionKey returns a copy of the negotiated session key.
func (pc *PeerConnection) SessionKey() []byte {
	pc.sessionMu.RLock()
	defer pc.sessionMu.RUnlock()
	return append([]byte(nil), pc.sessionKey...)
}

// SessionEpoch returns the current secure-frame epoch.
func (pc *PeerConnection) SessionEpoch() uint64 {
	pc.sessionMu.RLock()
	defer pc.sessionMu.RUnlock()
	return pc.sessionEpoch
}

// PeerDeviceID returns the remote device ID associated with this connection.
func (pc *PeerConnection) PeerDeviceID() string {
	return pc.peerDeviceID
}

// PeerDeviceName returns the remote device name from handshake metadata.
func (pc *PeerConnection) PeerDeviceName() string {
	return pc.peerDeviceName
}

// PeerPublicKey returns the remote Ed25519 public key (base64) from handshake metadata.
func (pc *PeerConnection) PeerPublicKey() string {
	return pc.peerPublicKey
}

// RemoteAddr returns the remote network endpoint.
func (pc *PeerConnection) RemoteAddr() net.Addr {
	return pc.conn.RemoteAddr()
}

// NextSendSequence increments and returns the outbound sequence counter.
func (pc *PeerConnection) NextSendSequence() uint64 {
	pc.sendSequenceMu.Lock()
	defer pc.sendSequenceMu.Unlock()
	pc.sendSequence++
	return pc.sendSequence
}

// ValidateSequence rejects replayed or non-monotonic sequences.
func (pc *PeerConnection) ValidateSequence(sequence uint64) error {
	pc.sendSequenceMu.Lock()
	defer pc.sendSequenceMu.Unlock()

	if sequence <= pc.lastSeenSeq {
		return ErrSequenceReplay
	}
	pc.lastSeenSeq = sequence
	return nil
}

// SendMessage marshals a protocol message and writes it as one frame.
func (pc *PeerConnection) SendMessage(message any) error {
	payload, err := EncodeJSON(message)
	if err != nil {
		return err
	}
	return pc.SendRaw(payload)
}

// SendRaw writes a pre-marshaled payload as one frame.
func (pc *PeerConnection) SendRaw(payload []byte) error {
	if pc.State() == StateDisconnected {
		if err := pc.LastError(); err != nil {
			return err
		}
		return io.EOF
	}

	encryptedPayload, err := pc.encryptPayload(payload)
	if err != nil {
		return err
	}

	pc.sendMu.Lock()
	defer pc.sendMu.Unlock()
	if err := WriteFrame(pc.conn, encryptedPayload); err != nil {
		pc.closeWithError(fmt.Errorf("write frame: %w", err))
		return err
	}

	pc.touchActivity()
	pc.bytesSinceRekey.Add(uint64(len(payload)))
	pc.triggerRekeyIfNeeded()
	if msgType, err := DecodeMessageType(payload); err == nil && msgType != TypePing && msgType != TypePong {
		pc.setState(StateReady)
	}
	return nil
}

// SetRekeyNeededCallback sets the callback invoked when this connection reaches a rekey threshold.
func (pc *PeerConnection) SetRekeyNeededCallback(callback func(*PeerConnection)) {
	pc.rekeyStateMu.Lock()
	pc.onRekeyNeeded = callback
	pc.rekeyStateMu.Unlock()
}

// RotateSessionKey atomically switches the connection to a newly derived session key.
func (pc *PeerConnection) RotateSessionKey(newKey []byte, epoch uint64) error {
	if len(newKey) == 0 {
		return errors.New("rekey: new session key is required")
	}

	pc.sessionMu.Lock()
	currentEpoch := pc.sessionEpoch
	if epoch <= currentEpoch {
		pc.sessionMu.Unlock()
		return fmt.Errorf("rekey: stale epoch %d (current %d)", epoch, currentEpoch)
	}
	if epoch != currentEpoch+1 {
		pc.sessionMu.Unlock()
		return fmt.Errorf("rekey: unexpected epoch %d (expected %d)", epoch, currentEpoch+1)
	}

	pc.previousSessionKey = append([]byte(nil), pc.sessionKey...)
	pc.previousSessionEpoch = currentEpoch
	pc.hasPreviousSession = len(pc.previousSessionKey) > 0

	pc.sessionKey = append([]byte(nil), newKey...)
	pc.sessionEpoch = epoch
	pc.sessionMu.Unlock()

	pc.bytesSinceRekey.Store(0)
	pc.lastRekeyAt.Store(time.Now().UnixNano())
	pc.setRekeyInProgress(false)
	return nil
}

// ReceiveMessage waits for the next non-keepalive inbound protocol frame.
func (pc *PeerConnection) ReceiveMessage(ctx context.Context) ([]byte, error) {
	select {
	case payload := <-pc.inbound:
		return payload, nil
	case <-pc.closed:
		if err := pc.LastError(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Disconnect sends peer_disconnect and closes the connection.
func (pc *PeerConnection) Disconnect() error {
	pc.setState(StateDisconnecting)

	_ = pc.SendMessage(PeerDisconnect{
		Type:         TypePeerDisconnect,
		FromDeviceID: pc.localDeviceID,
		Timestamp:    time.Now().UnixMilli(),
	})

	return pc.Close()
}

// Close terminates the connection.
func (pc *PeerConnection) Close() error {
	pc.closeWithError(nil)
	return nil
}

func (pc *PeerConnection) readLoop() {
	for {
		select {
		case <-pc.closed:
			return
		default:
		}

		framePayload, err := ReadFrameWithTimeout(pc.conn, pc.frameReadTimeout)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				pc.closeWithError(nil)
				return
			}

			pc.closeWithError(fmt.Errorf("read frame: %w", err))
			return
		}

		pc.touchActivity()
		if len(framePayload) == 0 {
			continue
		}

		payload, err := pc.decryptPayload(framePayload)
		if err != nil {
			pc.closeWithError(fmt.Errorf("decrypt frame: %w", err))
			return
		}
		if len(payload) == 0 {
			continue
		}

		msgType, err := DecodeMessageType(payload)
		if err != nil {
			if len(payload) > MaxControlFrameSize {
				pc.closeWithError(ErrFrameTooLarge)
				return
			}
			select {
			case pc.inbound <- payload:
			case <-pc.closed:
			}
			continue
		}
		if msgType != TypeFileData && len(payload) > MaxControlFrameSize {
			pc.closeWithError(ErrFrameTooLarge)
			return
		}

		switch msgType {
		case TypePing:
			pc.setState(StateIdle)
			if pc.autoRespondPing {
				_ = pc.SendMessage(PongMessage{
					Type:         TypePong,
					FromDeviceID: pc.localDeviceID,
					Timestamp:    time.Now().UnixMilli(),
				})
			}
		case TypePong:
			pc.ackPong()
			pc.setState(StateIdle)
		case TypePeerDisconnect:
			pc.setState(StateDisconnecting)
			pc.closeWithError(nil)
			return
		default:
			pc.setState(StateReady)
			select {
			case pc.inbound <- payload:
			case <-pc.closed:
				return
			}
		}
	}
}

func (pc *PeerConnection) keepAliveLoop() {
	checkEvery := pc.keepAliveInterval / 2
	if checkEvery <= 0 {
		checkEvery = pc.keepAliveInterval
	}
	ticker := time.NewTicker(checkEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if pc.State() == StateDisconnected {
				return
			}
			pc.triggerRekeyIfNeeded()

			if pc.waitingPongExpired() {
				pc.closeWithError(ErrPongTimeout)
				return
			}

			idleFor := time.Since(time.Unix(0, pc.lastActivity.Load()))
			if idleFor < pc.keepAliveInterval {
				continue
			}

			if pc.isWaitingPong() {
				continue
			}

			if err := pc.SendMessage(PingMessage{
				Type:         TypePing,
				FromDeviceID: pc.localDeviceID,
				Timestamp:    time.Now().UnixMilli(),
			}); err != nil {
				return
			}
			pc.setWaitingPong(time.Now().Add(pc.keepAliveTimeout))
			pc.setState(StateIdle)
		case <-pc.closed:
			return
		}
	}
}

func (pc *PeerConnection) setState(state ConnectionState) {
	pc.stateMu.Lock()
	defer pc.stateMu.Unlock()
	pc.state = state
}

func (pc *PeerConnection) touchActivity() {
	pc.lastActivity.Store(time.Now().UnixNano())
}

func (pc *PeerConnection) setWaitingPong(deadline time.Time) {
	pc.waitMu.Lock()
	defer pc.waitMu.Unlock()
	pc.waitingPong = true
	pc.pongDeadline = deadline
}

func (pc *PeerConnection) ackPong() {
	pc.waitMu.Lock()
	defer pc.waitMu.Unlock()
	pc.waitingPong = false
	pc.pongDeadline = time.Time{}
}

func (pc *PeerConnection) isWaitingPong() bool {
	pc.waitMu.Lock()
	defer pc.waitMu.Unlock()
	return pc.waitingPong
}

func (pc *PeerConnection) waitingPongExpired() bool {
	pc.waitMu.Lock()
	defer pc.waitMu.Unlock()
	return pc.waitingPong && time.Now().After(pc.pongDeadline)
}

func (pc *PeerConnection) closeWithError(err error) {
	pc.closeOnce.Do(func() {
		pc.errMu.Lock()
		pc.closeErr = err
		pc.errMu.Unlock()
		pc.setRekeyInProgress(false)

		pc.setState(StateDisconnected)
		_ = pc.conn.Close()
		close(pc.closed)
	})
}

func decodeHandshake(payload []byte) (HandshakeMessage, error) {
	var msg HandshakeMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return HandshakeMessage{}, fmt.Errorf("decode handshake: %w", err)
	}
	return msg, nil
}

func decodeHandshakeResponse(payload []byte) (HandshakeResponse, error) {
	var msg HandshakeResponse
	if err := json.Unmarshal(payload, &msg); err != nil {
		return HandshakeResponse{}, fmt.Errorf("decode handshake response: %w", err)
	}
	return msg, nil
}

func (pc *PeerConnection) encryptPayload(payload []byte) ([]byte, error) {
	pc.sessionMu.RLock()
	key := append([]byte(nil), pc.sessionKey...)
	epoch := pc.sessionEpoch
	pc.sessionMu.RUnlock()

	ciphertext, nonce, err := appcrypto.Encrypt(key, payload)
	if err != nil {
		return nil, fmt.Errorf("encrypt payload: %w", err)
	}
	frame := secureFrame{
		Type:       secureFrameType,
		Epoch:      epoch,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}
	return EncodeJSON(frame)
}

func (pc *PeerConnection) decryptPayload(framePayload []byte) ([]byte, error) {
	var frame secureFrame
	if err := json.Unmarshal(framePayload, &frame); err != nil {
		return nil, fmt.Errorf("%w: decode frame: %v", ErrInvalidSecureFrame, err)
	}
	if frame.Type != secureFrameType || frame.Nonce == "" || frame.Ciphertext == "" {
		return nil, ErrInvalidSecureFrame
	}

	nonce, err := base64.StdEncoding.DecodeString(frame.Nonce)
	if err != nil {
		return nil, fmt.Errorf("%w: decode nonce: %v", ErrInvalidSecureFrame, err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(frame.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("%w: decode ciphertext: %v", ErrInvalidSecureFrame, err)
	}

	pc.sessionMu.RLock()
	currentKey := append([]byte(nil), pc.sessionKey...)
	currentEpoch := pc.sessionEpoch
	previousKey := append([]byte(nil), pc.previousSessionKey...)
	previousEpoch := pc.previousSessionEpoch
	hasPrevious := pc.hasPreviousSession
	pc.sessionMu.RUnlock()

	key := currentKey
	switch {
	case frame.Epoch == currentEpoch:
	case hasPrevious && frame.Epoch == previousEpoch:
		key = previousKey
	case frame.Epoch == 0 && currentEpoch == 0:
	default:
		return nil, fmt.Errorf("%w: unsupported epoch %d", ErrInvalidSecureFrame, frame.Epoch)
	}

	plaintext, err := appcrypto.Decrypt(key, nonce, ciphertext)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func (pc *PeerConnection) triggerRekeyIfNeeded() {
	lastRekeyAt := pc.lastRekeyAt.Load()
	rekeyDueByTime := time.Since(time.Unix(0, lastRekeyAt)) >= pc.rekeyInterval
	rekeyDueByBytes := pc.bytesSinceRekey.Load() >= pc.rekeyAfterBytes
	if !rekeyDueByTime && !rekeyDueByBytes {
		return
	}

	pc.rekeyStateMu.Lock()
	if pc.rekeyInProgress || pc.onRekeyNeeded == nil {
		pc.rekeyStateMu.Unlock()
		return
	}
	pc.rekeyInProgress = true
	callback := pc.onRekeyNeeded
	pc.rekeyStateMu.Unlock()

	go callback(pc)
}

func (pc *PeerConnection) setRekeyInProgress(inProgress bool) {
	pc.rekeyStateMu.Lock()
	pc.rekeyInProgress = inProgress
	pc.rekeyStateMu.Unlock()
}
