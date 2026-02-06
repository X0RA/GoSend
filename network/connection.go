package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrSequenceReplay indicates a non-monotonic sequence value.
	ErrSequenceReplay = errors.New("network: sequence replay detected")
	// ErrPongTimeout indicates keep-alive timed out waiting for pong.
	ErrPongTimeout = errors.New("network: pong timeout")
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

// ConnectionOptions controls runtime behavior of PeerConnection.
type ConnectionOptions struct {
	LocalDeviceID     string
	PeerDeviceID      string
	KeepAliveInterval time.Duration
	KeepAliveTimeout  time.Duration
	FrameReadTimeout  time.Duration
	AutoRespondPing   bool
}

// PeerConnection manages a stateful framed TCP session.
type PeerConnection struct {
	conn net.Conn

	sessionKey []byte

	localDeviceID string
	peerDeviceID  string

	sendSequenceMu sync.Mutex
	sendSequence   uint64
	lastSeenSeq    uint64

	sendMu sync.Mutex

	stateMu sync.RWMutex
	state   ConnectionState

	waitMu       sync.Mutex
	waitingPong  bool
	pongDeadline time.Time

	lastActivity atomic.Int64

	keepAliveInterval time.Duration
	keepAliveTimeout  time.Duration
	frameReadTimeout  time.Duration
	autoRespondPing   bool

	inbound chan []byte

	closeOnce sync.Once
	closed    chan struct{}

	errMu    sync.RWMutex
	closeErr error
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

	pc := &PeerConnection{
		conn:              conn,
		sessionKey:        append([]byte(nil), sessionKey...),
		localDeviceID:     options.LocalDeviceID,
		peerDeviceID:      options.PeerDeviceID,
		keepAliveInterval: interval,
		keepAliveTimeout:  timeout,
		frameReadTimeout:  readTimeout,
		autoRespondPing:   options.AutoRespondPing,
		inbound:           make(chan []byte, 64),
		closed:            make(chan struct{}),
		state:             StateConnecting,
	}

	pc.touchActivity()
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
	return append([]byte(nil), pc.sessionKey...)
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

	pc.sendMu.Lock()
	defer pc.sendMu.Unlock()
	if err := WriteFrame(pc.conn, payload); err != nil {
		pc.closeWithError(fmt.Errorf("write frame: %w", err))
		return err
	}

	pc.touchActivity()
	if msgType, err := DecodeMessageType(payload); err == nil && msgType != TypePing && msgType != TypePong {
		pc.setState(StateReady)
	}
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

		payload, err := ReadFrameWithTimeout(pc.conn, pc.frameReadTimeout)
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
		if len(payload) == 0 {
			continue
		}

		msgType, err := DecodeMessageType(payload)
		if err != nil {
			select {
			case pc.inbound <- payload:
			case <-pc.closed:
			}
			continue
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
