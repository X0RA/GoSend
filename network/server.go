package network

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"gosend/crypto"
)

const (
	defaultConnectionRateLimitPerIP  = 5
	defaultConnectionRateLimitWindow = time.Minute
)

type inboundConnectionRate struct {
	windowStart time.Time
	count       int
	lastSeen    time.Time
}

// Server accepts inbound TCP sessions and upgrades them to PeerConnection.
type Server struct {
	listener net.Listener
	options  HandshakeOptions

	incoming chan *PeerConnection
	errs     chan error

	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup

	connectionRates      map[string]inboundConnectionRate
	lastRateLimiterPrune time.Time
}

// Listen starts a TCP listener and handshake accept loop.
func Listen(address string, options HandshakeOptions) (*Server, error) {
	opts := options.withDefaults()
	if err := opts.validateIdentity(); err != nil {
		return nil, err
	}

	if address == "" {
		address = ":0"
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", address, err)
	}

	server := &Server{
		listener:        listener,
		options:         opts,
		incoming:        make(chan *PeerConnection, 16),
		errs:            make(chan error, 16),
		closed:          make(chan struct{}),
		connectionRates: make(map[string]inboundConnectionRate),
	}

	server.wg.Add(1)
	go server.acceptLoop()
	return server, nil
}

// Addr returns the listening address.
func (s *Server) Addr() net.Addr {
	return s.listener.Addr()
}

// Incoming returns accepted and handshaked peer connections.
func (s *Server) Incoming() <-chan *PeerConnection {
	return s.incoming
}

// Errors returns asynchronous server errors.
func (s *Server) Errors() <-chan error {
	return s.errs
}

// Close stops accepting and closes all server channels.
func (s *Server) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		close(s.closed)
		closeErr = s.listener.Close()
		s.wg.Wait()
		close(s.incoming)
		close(s.errs)
	})
	return closeErr
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closed:
				return
			default:
			}

			select {
			case s.errs <- fmt.Errorf("accept connection: %w", err):
			default:
			}
			continue
		}

		allowed, remoteIP := s.allowInboundConnection(conn.RemoteAddr())
		if !allowed {
			if s.options.OnInboundConnectionRateLimit != nil {
				s.options.OnInboundConnectionRateLimit(remoteIP)
			}
			_ = conn.Close()
			continue
		}

		s.wg.Add(1)
		go s.handleInboundConn(conn)
	}
}

func (s *Server) handleInboundConn(conn net.Conn) {
	defer s.wg.Done()

	closeConn := true
	defer func() {
		if closeConn {
			_ = conn.Close()
		}
	}()

	if err := conn.SetDeadline(time.Now().Add(s.options.ConnectionTimeout)); err != nil {
		s.reportError(fmt.Errorf("set handshake deadline: %w", err))
		return
	}

	nonce, err := generateHandshakeChallengeNonce()
	if err != nil {
		s.reportError(fmt.Errorf("generate handshake challenge nonce: %w", err))
		return
	}
	challengePayload, err := EncodeJSON(HandshakeChallenge{
		Type:  TypeHandshakeChallenge,
		Nonce: nonce,
	})
	if err != nil {
		s.reportError(err)
		return
	}
	if err := WriteFrame(conn, challengePayload); err != nil {
		s.reportError(fmt.Errorf("write handshake challenge: %w", err))
		return
	}

	handshakePayload, err := ReadControlFrameWithTimeout(conn, s.options.ConnectionTimeout)
	if err != nil {
		s.reportError(fmt.Errorf("read handshake: %w", err))
		return
	}

	msgType, err := DecodeMessageType(handshakePayload)
	if err != nil {
		s.reportError(err)
		return
	}
	if msgType != TypeHandshake {
		_ = s.sendError(conn, ErrorMessage{
			Type:      TypeError,
			Code:      "unknown_type",
			Message:   fmt.Sprintf("Expected %q, got %q", TypeHandshake, msgType),
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	handshake, err := decodeHandshake(handshakePayload)
	if err != nil {
		s.reportError(err)
		return
	}

	if handshake.ProtocolVersion != ProtocolVersion {
		_ = s.sendError(conn, makeVersionMismatchError(int64(handshake.ProtocolVersion)))
		return
	}
	if handshake.ChallengeNonce != nonce {
		_ = s.sendError(conn, ErrorMessage{
			Type:      TypeError,
			Code:      "invalid_handshake_challenge",
			Message:   "Handshake challenge nonce mismatch.",
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	if _, err := VerifyHandshakeMessage(handshake); err != nil {
		s.reportError(fmt.Errorf("verify handshake: %w", err))
		return
	}

	if err := evaluatePeerKey(
		handshake.DeviceID,
		handshake.Ed25519PublicKey,
		s.options.KnownPeerKeys,
		s.options.KnownPeerKeyLookup,
		s.options.OnKeyChangeDecision,
	); err != nil {
		_ = s.sendError(conn, ErrorMessage{
			Type:      TypeError,
			Code:      "key_changed",
			Message:   err.Error(),
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	localEphemeralPrivateKey, localEphemeralPublicKey, err := crypto.GenerateEphemeralX25519KeyPair()
	if err != nil {
		s.reportError(err)
		return
	}

	sessionKey, err := deriveSessionKey(localEphemeralPrivateKey, handshake.X25519PublicKey, s.options.Identity.DeviceID, handshake.DeviceID, handshake.ChallengeNonce)
	if err != nil {
		s.reportError(err)
		return
	}

	response, err := BuildHandshakeResponse(s.options.Identity, localEphemeralPublicKey.Bytes())
	if err != nil {
		s.reportError(err)
		return
	}
	responsePayload, err := EncodeJSON(response)
	if err != nil {
		s.reportError(err)
		return
	}
	if err := WriteFrame(conn, responsePayload); err != nil {
		s.reportError(fmt.Errorf("write handshake response: %w", err))
		return
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		s.reportError(fmt.Errorf("clear handshake deadline: %w", err))
		return
	}

	peerConnection := newPeerConnection(conn, sessionKey, ConnectionOptions{
		LocalDeviceID:     s.options.Identity.DeviceID,
		PeerDeviceID:      handshake.DeviceID,
		PeerDeviceName:    handshake.DeviceName,
		PeerPublicKey:     handshake.Ed25519PublicKey,
		KeepAliveInterval: s.options.KeepAliveInterval,
		KeepAliveTimeout:  s.options.KeepAliveTimeout,
		FrameReadTimeout:  s.options.FrameReadTimeout,
		RekeyInterval:     s.options.RekeyInterval,
		RekeyAfterBytes:   s.options.RekeyAfterBytes,
		AutoRespondPing:   s.options.autoRespondPingEnabled(),
	})

	closeConn = false
	select {
	case s.incoming <- peerConnection:
	case <-s.closed:
		_ = peerConnection.Close()
	}
}

func (s *Server) sendError(conn net.Conn, message ErrorMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return WriteFrame(conn, payload)
}

func (s *Server) reportError(err error) {
	if err == nil {
		return
	}

	// Accept loop shutdown produces expected net.ErrClosed errors.
	if errors.Is(err, net.ErrClosed) {
		return
	}

	select {
	case s.errs <- err:
	default:
	}
}

func generateHandshakeChallengeNonce() (string, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(nonce), nil
}

func (s *Server) allowInboundConnection(addr net.Addr) (bool, string) {
	limit := s.options.ConnectionRateLimitPerIP
	window := s.options.ConnectionRateLimitWindow
	if limit <= 0 || window <= 0 {
		return true, ""
	}

	ip := remoteIPFromAddr(addr)
	if ip == "" {
		return true, ""
	}

	now := time.Now()
	s.pruneConnectionRateEntries(now, window*4)

	entry := s.connectionRates[ip]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= window {
		entry.windowStart = now
		entry.count = 0
	}
	entry.lastSeen = now

	if entry.count >= limit {
		s.connectionRates[ip] = entry
		return false, ip
	}

	entry.count++
	s.connectionRates[ip] = entry
	return true, ip
}

func (s *Server) pruneConnectionRateEntries(now time.Time, maxAge time.Duration) {
	if len(s.connectionRates) == 0 || maxAge <= 0 {
		return
	}
	if !s.lastRateLimiterPrune.IsZero() && now.Sub(s.lastRateLimiterPrune) < time.Second {
		return
	}

	for ip, entry := range s.connectionRates {
		if entry.lastSeen.IsZero() || now.Sub(entry.lastSeen) > maxAge {
			delete(s.connectionRates, ip)
		}
	}
	s.lastRateLimiterPrune = now
}

func remoteIPFromAddr(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return tcpAddr.IP.String()
	}

	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	return host
}
