package network

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	appcrypto "gosend/crypto"
	"gosend/storage"
)

const (
	peerStatusOnline  = "online"
	peerStatusOffline = "offline"
	peerStatusPending = "pending"
	peerStatusBlocked = "blocked"

	messageContentTypeText = "text"

	deliveryStatusPending   = "pending"
	deliveryStatusSent      = "sent"
	deliveryStatusDelivered = "delivered"
	deliveryStatusFailed    = "failed"

	maxTimestampSkew      = 5 * time.Minute
	maxQueueAge           = 7 * 24 * time.Hour
	maxQueuedMessages     = 500
	maxQueuedBytesPerPeer = 50 * 1024 * 1024

	defaultFileChunkSize        = 256 * 1024
	defaultFileResponseTimout   = 10 * time.Second
	defaultFileCompleteTimeout  = 5 * time.Minute // receiver may need time to checksum/rename large files
	defaultFileChunkRetries     = 3
	defaultRekeyResponseTimeout = 15 * time.Second
)

var defaultReconnectBackoff = []time.Duration{
	0,
	5 * time.Second,
	15 * time.Second,
	60 * time.Second,
}

// AddRequestNotification is queued when manual approval is required.
type AddRequestNotification struct {
	PeerDeviceID   string
	PeerDeviceName string
}

// FileRequestNotification is emitted when a peer requests sending a file.
type FileRequestNotification struct {
	FileID       string
	FromDeviceID string
	Filename     string
	Filesize     int64
	Filetype     string
	Checksum     string
}

// FileProgress captures transfer progress for one file.
type FileProgress struct {
	FileID            string
	PeerDeviceID      string
	Direction         string
	BytesTransferred  int64
	TotalBytes        int64
	ChunkIndex        int
	TotalChunks       int
	TransferCompleted bool
}

// PeerManagerOptions configures peer lifecycle management.
type PeerManagerOptions struct {
	Identity LocalIdentity
	Store    *storage.Store

	ListenAddress string

	ApproveAddRequest func(AddRequestNotification) (bool, error)
	OnKeyChange       KeyChangeDecisionFunc

	ReconnectBackoff   []time.Duration
	AddResponseTimeout time.Duration

	ConnectionTimeout    time.Duration
	KeepAliveInterval    time.Duration
	KeepAliveTimeout     time.Duration
	FrameReadTimeout     time.Duration
	RekeyInterval        time.Duration
	RekeyAfterBytes      uint64
	RekeyResponseTimeout time.Duration
	AutoRespondPing      *bool

	OnMessageReceived func(storage.Message)
	OnQueueOverflow   func(peerDeviceID string, droppedCount int)

	FilesDir            string
	FileChunkSize       int
	FileResponseTimeout time.Duration
	FileCompleteTimeout time.Duration // timeout waiting for receiver's FileComplete after all chunks sent
	MaxChunkRetries     int
	OnFileRequest       func(FileRequestNotification) (bool, error)
	OnFileProgress      func(FileProgress)
}

// PeerManager manages peer add/remove/disconnect flows plus reconnect behavior.
type PeerManager struct {
	options PeerManagerOptions

	server *Server

	ctx    context.Context
	cancel context.CancelFunc

	wg       sync.WaitGroup
	stopOnce sync.Once

	connMu      sync.RWMutex
	connections map[string]*PeerConnection

	knownKeyMu sync.RWMutex
	knownKeys  map[string]string

	outboundMu          sync.Mutex
	outboundAddPending  map[string]bool
	outboundAddResponse map[string]chan PeerAddResponse

	inboundMu         sync.Mutex
	inboundAddPending map[string]chan bool
	addRequests       chan AddRequestNotification

	reconnectMu      sync.Mutex
	reconnectWorkers map[string]context.CancelFunc

	suppressMu        sync.Mutex
	suppressReconnect map[string]bool

	drainMu      sync.Mutex
	activeDrains map[string]bool

	fileMu                 sync.Mutex
	outboundFileTransfers  map[string]*outboundFileTransfer
	inboundFileTransfers   map[string]*inboundFileTransfer
	outboundFileEventChans map[string]chan fileTransferEvent

	rekeyMu             sync.Mutex
	pendingRekeyWaiters map[string]chan rekeyWaitResult

	errors chan error
}

type rekeyWaitResult struct {
	Response RekeyResponse
	Err      error
}

// NewPeerManager creates a peer manager with validated configuration.
func NewPeerManager(options PeerManagerOptions) (*PeerManager, error) {
	if options.Store == nil {
		return nil, errors.New("store is required")
	}
	if options.Identity.DeviceID == "" {
		return nil, errors.New("identity.device_id is required")
	}
	if options.Identity.DeviceName == "" {
		return nil, errors.New("identity.device_name is required")
	}
	if len(options.Identity.Ed25519PrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("identity.ed25519_private_key is invalid")
	}
	if len(options.Identity.Ed25519PublicKey) != ed25519.PublicKeySize {
		return nil, errors.New("identity.ed25519_public_key is invalid")
	}
	if options.AddResponseTimeout <= 0 {
		options.AddResponseTimeout = 15 * time.Second
	}
	if len(options.ReconnectBackoff) == 0 {
		options.ReconnectBackoff = append([]time.Duration(nil), defaultReconnectBackoff...)
	}
	if options.FileChunkSize <= 0 {
		options.FileChunkSize = defaultFileChunkSize
	}
	if options.FileResponseTimeout <= 0 {
		options.FileResponseTimeout = defaultFileResponseTimout
	}
	if options.FileCompleteTimeout <= 0 {
		options.FileCompleteTimeout = defaultFileCompleteTimeout
	}
	if options.MaxChunkRetries <= 0 {
		options.MaxChunkRetries = defaultFileChunkRetries
	}
	if options.RekeyResponseTimeout <= 0 {
		options.RekeyResponseTimeout = defaultRekeyResponseTimeout
	}
	if options.FilesDir == "" {
		options.FilesDir = "./files"
	}

	manager := &PeerManager{
		options:                options,
		connections:            make(map[string]*PeerConnection),
		knownKeys:              make(map[string]string),
		outboundAddPending:     make(map[string]bool),
		outboundAddResponse:    make(map[string]chan PeerAddResponse),
		inboundAddPending:      make(map[string]chan bool),
		addRequests:            make(chan AddRequestNotification, 64),
		reconnectWorkers:       make(map[string]context.CancelFunc),
		suppressReconnect:      make(map[string]bool),
		activeDrains:           make(map[string]bool),
		outboundFileTransfers:  make(map[string]*outboundFileTransfer),
		inboundFileTransfers:   make(map[string]*inboundFileTransfer),
		outboundFileEventChans: make(map[string]chan fileTransferEvent),
		pendingRekeyWaiters:    make(map[string]chan rekeyWaitResult),
		errors:                 make(chan error, 64),
	}

	return manager, nil
}

// Start begins listening for inbound connections and reconnecting known online peers.
func (m *PeerManager) Start() error {
	if m.ctx != nil {
		return nil
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())

	if err := os.MkdirAll(m.options.FilesDir, 0o700); err != nil {
		return fmt.Errorf("create files dir: %w", err)
	}

	if err := m.ensureLocalPeerRecord(); err != nil {
		return err
	}

	if err := m.loadKnownPeers(); err != nil {
		return err
	}

	server, err := Listen(m.options.ListenAddress, m.handshakeOptionsForServer())
	if err != nil {
		return err
	}
	m.server = server

	m.wg.Add(1)
	go m.serverLoop()

	peers, err := m.options.Store.ListPeers()
	if err == nil {
		for _, peer := range peers {
			if peer.DeviceID == m.options.Identity.DeviceID {
				continue
			}
			if peer.Status == peerStatusOnline {
				m.startReconnect(peer.DeviceID)
			}
		}
	}

	return nil
}

// Stop stops manager, listener, reconnect workers, and active connections.
func (m *PeerManager) Stop() {
	m.stopOnce.Do(func() {
		if m.cancel == nil {
			return
		}

		m.cancel()
		if m.server != nil {
			_ = m.server.Close()
		}

		m.reconnectMu.Lock()
		for _, cancel := range m.reconnectWorkers {
			cancel()
		}
		m.reconnectWorkers = make(map[string]context.CancelFunc)
		m.reconnectMu.Unlock()

		m.connMu.Lock()
		for _, conn := range m.connections {
			_ = conn.Close()
		}
		m.connections = make(map[string]*PeerConnection)
		m.connMu.Unlock()

		m.wg.Wait()
		close(m.addRequests)
		close(m.errors)
	})
}

// Addr returns the listening address.
func (m *PeerManager) Addr() net.Addr {
	if m.server == nil {
		return nil
	}
	return m.server.Addr()
}

// Errors returns asynchronous manager/server errors.
func (m *PeerManager) Errors() <-chan error {
	return m.errors
}

// PendingAddRequests returns queued peer-add requests for manual approval.
func (m *PeerManager) PendingAddRequests() <-chan AddRequestNotification {
	return m.addRequests
}

// ApprovePeerAdd resolves a queued add request.
func (m *PeerManager) ApprovePeerAdd(peerDeviceID string, accept bool) error {
	m.inboundMu.Lock()
	ch, ok := m.inboundAddPending[peerDeviceID]
	if ok {
		delete(m.inboundAddPending, peerDeviceID)
	}
	m.inboundMu.Unlock()
	if !ok {
		return fmt.Errorf("no pending add request for peer %q", peerDeviceID)
	}

	select {
	case ch <- accept:
		return nil
	default:
		return errors.New("add request decision channel is full")
	}
}

// Connect dials and registers an outbound connection.
func (m *PeerManager) Connect(address string) (*PeerConnection, error) {
	if m.ctx == nil {
		return nil, errors.New("peer manager is not started")
	}
	return m.dialAndRegister(address)
}

// SendPeerAddRequest sends peer_add_request and waits for peer_add_response.
func (m *PeerManager) SendPeerAddRequest(peerDeviceID string) (bool, error) {
	conn := m.getConnection(peerDeviceID)
	if conn == nil {
		return false, fmt.Errorf("no active connection for peer %q", peerDeviceID)
	}

	responseCh := make(chan PeerAddResponse, 1)
	m.outboundMu.Lock()
	m.outboundAddPending[peerDeviceID] = true
	m.outboundAddResponse[peerDeviceID] = responseCh
	m.outboundMu.Unlock()
	defer func() {
		m.outboundMu.Lock()
		delete(m.outboundAddPending, peerDeviceID)
		delete(m.outboundAddResponse, peerDeviceID)
		m.outboundMu.Unlock()
	}()

	request := PeerAddRequest{
		Type:           TypePeerAddRequest,
		FromDeviceID:   m.options.Identity.DeviceID,
		FromDeviceName: m.options.Identity.DeviceName,
		Timestamp:      time.Now().UnixMilli(),
	}
	if err := m.signPeerAddRequest(&request); err != nil {
		return false, err
	}

	if err := conn.SendMessage(request); err != nil {
		return false, err
	}

	timer := time.NewTimer(m.options.AddResponseTimeout)
	defer timer.Stop()

	select {
	case response := <-responseCh:
		accepted := stringsEqualFold(response.Status, "accepted")
		if accepted {
			if err := m.persistPeerConnection(conn, peerStatusOnline, true); err != nil {
				return false, err
			}
		}
		return accepted, nil
	case <-timer.C:
		return false, errors.New("timed out waiting for peer_add_response")
	case <-m.ctx.Done():
		return false, errors.New("peer manager stopped")
	}
}

// SendPeerRemove sends peer_remove, removes local peer state, and closes connection.
func (m *PeerManager) SendPeerRemove(peerDeviceID string) error {
	conn := m.getConnection(peerDeviceID)
	var sendErr error
	if conn != nil {
		msg := PeerRemove{
			Type:         TypePeerRemove,
			FromDeviceID: m.options.Identity.DeviceID,
			Timestamp:    time.Now().UnixMilli(),
		}
		_ = m.signPeerRemove(&msg)
		sendErr = conn.SendMessage(msg)
		if sendErr != nil {
			m.reportError(sendErr)
		}
	}

	m.markSuppressReconnect(peerDeviceID)
	m.stopReconnect(peerDeviceID)
	_ = m.options.Store.RemovePeer(peerDeviceID)
	m.removeKnownKey(peerDeviceID)
	if conn != nil {
		if sendErr != nil {
			_ = conn.Close()
		} else {
			go func(c *PeerConnection) {
				timer := time.NewTimer(200 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-c.Done():
				case <-timer.C:
					_ = c.Close()
				case <-m.ctx.Done():
				}
			}(conn)
		}
	}
	return nil
}

// SendPeerDisconnect sends peer_disconnect, closes the session, and marks peer offline.
func (m *PeerManager) SendPeerDisconnect(peerDeviceID string) error {
	conn := m.getConnection(peerDeviceID)
	if conn == nil {
		return fmt.Errorf("no active connection for peer %q", peerDeviceID)
	}

	m.markSuppressReconnect(peerDeviceID)
	if err := conn.Disconnect(); err != nil {
		return err
	}
	_ = m.options.Store.UpdatePeerStatus(peerDeviceID, peerStatusOffline, time.Now().UnixMilli())
	return nil
}

// SendTextMessage sends a text message to a peer or queues it if offline.
func (m *PeerManager) SendTextMessage(peerDeviceID, content string) (string, error) {
	if peerDeviceID == "" {
		return "", errors.New("peer device ID is required")
	}
	if content == "" {
		return "", errors.New("content is required")
	}

	messageID := uuid.NewString()
	return messageID, m.sendTextMessageWithID(peerDeviceID, messageID, content, time.Now().UnixMilli())
}

func (m *PeerManager) serverLoop() {
	defer m.wg.Done()
	for {
		select {
		case conn, ok := <-m.server.Incoming():
			if !ok {
				return
			}
			m.registerConnection(conn)
		case err, ok := <-m.server.Errors():
			if !ok {
				return
			}
			m.reportError(err)
		case <-m.ctx.Done():
			return
		}
	}
}

func (m *PeerManager) registerConnection(conn *PeerConnection) {
	peerID := conn.PeerDeviceID()
	if peerID == "" {
		_ = conn.Close()
		return
	}
	conn.SetRekeyNeededCallback(m.onConnectionRekeyNeeded)

	m.connMu.Lock()
	if existing, exists := m.connections[peerID]; exists && existing != conn {
		_ = existing.Close()
	}
	m.connections[peerID] = conn
	m.connMu.Unlock()

	m.stopReconnect(peerID)
	if err := m.persistPeerConnection(conn, peerStatusOnline, false); err != nil && !errors.Is(err, storage.ErrNotFound) {
		m.reportError(err)
	}

	m.startQueueDrain(peerID, conn)
	m.startOutboundFileTransferDrain(peerID, conn)

	m.wg.Add(1)
	go m.connectionLoop(conn)
}

func (m *PeerManager) connectionLoop(conn *PeerConnection) {
	defer m.wg.Done()

	peerID := conn.PeerDeviceID()
loop:
	for {
		payload, err := conn.ReceiveMessage(m.ctx)
		if err != nil {
			break loop
		}

		msgType, err := DecodeMessageType(payload)
		if err != nil {
			continue
		}

		switch msgType {
		case TypePeerAddRequest:
			var request PeerAddRequest
			if err := json.Unmarshal(payload, &request); err != nil {
				m.reportError(err)
				continue
			}
			m.handlePeerAddRequest(conn, request)
		case TypePeerAddResponse:
			var response PeerAddResponse
			if err := json.Unmarshal(payload, &response); err != nil {
				m.reportError(err)
				continue
			}
			m.handlePeerAddResponse(conn, response)
		case TypePeerRemove:
			var removeMsg PeerRemove
			if err := json.Unmarshal(payload, &removeMsg); err != nil {
				m.reportError(err)
				continue
			}
			if m.handlePeerRemove(conn, removeMsg) {
				break loop
			}
		case TypeMessage:
			var message EncryptedMessage
			if err := json.Unmarshal(payload, &message); err != nil {
				m.reportError(err)
				continue
			}
			m.handleIncomingEncryptedMessage(conn, message)
		case TypeRekeyRequest:
			var request RekeyRequest
			if err := json.Unmarshal(payload, &request); err != nil {
				m.reportError(err)
				continue
			}
			m.handleRekeyRequest(conn, request)
		case TypeRekeyResponse:
			var response RekeyResponse
			if err := json.Unmarshal(payload, &response); err != nil {
				m.reportError(err)
				continue
			}
			m.handleRekeyResponse(conn, response)
		case TypeAck:
			var ack AckMessage
			if err := json.Unmarshal(payload, &ack); err != nil {
				m.reportError(err)
				continue
			}
			m.handleAck(conn, ack)
		case TypeError:
			var msg ErrorMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				m.reportError(err)
				continue
			}
			m.handleProtocolError(msg)
		case TypeFileRequest:
			var request FileRequest
			if err := json.Unmarshal(payload, &request); err != nil {
				m.reportError(err)
				continue
			}
			m.handleFileRequest(conn, request)
		case TypeFileResponse:
			var response FileResponse
			if err := json.Unmarshal(payload, &response); err != nil {
				m.reportError(err)
				continue
			}
			m.handleFileResponse(conn, response)
		case TypeFileData:
			var data FileData
			if err := json.Unmarshal(payload, &data); err != nil {
				m.reportError(err)
				continue
			}
			m.handleFileData(conn, data)
		case TypeFileComplete:
			var complete FileComplete
			if err := json.Unmarshal(payload, &complete); err != nil {
				m.reportError(err)
				continue
			}
			m.handleFileComplete(conn, complete)
		}
	}

	_ = conn.Close()

	m.rekeyMu.Lock()
	if waiter := m.pendingRekeyWaiters[peerID]; waiter != nil {
		delete(m.pendingRekeyWaiters, peerID)
		select {
		case waiter <- rekeyWaitResult{Err: errors.New("connection closed during rekey")}:
		default:
		}
	}
	m.rekeyMu.Unlock()

	m.connMu.Lock()
	current := m.connections[peerID]
	if current == conn {
		delete(m.connections, peerID)
	}
	m.connMu.Unlock()

	if peerID != "" {
		if err := m.options.Store.UpdatePeerStatus(peerID, peerStatusOffline, time.Now().UnixMilli()); err != nil && !errors.Is(err, storage.ErrNotFound) {
			m.reportError(err)
		}
		if m.consumeSuppressReconnect(peerID) {
			return
		}
		if _, err := m.options.Store.GetPeer(peerID); errors.Is(err, storage.ErrNotFound) {
			return
		} else if err != nil {
			m.reportError(err)
			return
		}
		m.startReconnect(peerID)
	}
}

func (m *PeerManager) handlePeerAddRequest(conn *PeerConnection, request PeerAddRequest) {
	peerID := conn.PeerDeviceID()
	if peerID == "" || request.FromDeviceID == "" || request.FromDeviceName == "" || request.Signature == "" {
		return
	}
	if request.FromDeviceID != peerID {
		m.reportError(fmt.Errorf("rejecting peer_add_request: sender mismatch %q != %q", request.FromDeviceID, peerID))
		return
	}
	if !withinTimestampSkew(request.Timestamp) {
		m.reportError(fmt.Errorf("rejecting peer_add_request from %q: timestamp outside skew", peerID))
		return
	}
	if err := m.verifyPeerAddRequest(conn, request); err != nil {
		m.reportError(err)
		_ = m.sendErrorMessage(conn, "invalid_signature", "peer add request signature verification failed", "")
		return
	}

	accept := false
	var err error

	if m.isOutboundAddPending(peerID) {
		// Simultaneous add resolution: lower UUID is initiator, other auto-accepts.
		accept = true
	} else if m.options.ApproveAddRequest != nil {
		accept, err = m.options.ApproveAddRequest(AddRequestNotification{
			PeerDeviceID:   request.FromDeviceID,
			PeerDeviceName: request.FromDeviceName,
		})
		if err != nil {
			m.reportError(err)
			return
		}
	} else {
		decisionCh := make(chan bool, 1)
		m.inboundMu.Lock()
		m.inboundAddPending[peerID] = decisionCh
		m.inboundMu.Unlock()
		defer m.removeInboundAddPendingIfMatch(peerID, decisionCh)

		select {
		case m.addRequests <- AddRequestNotification{
			PeerDeviceID:   peerID,
			PeerDeviceName: request.FromDeviceName,
		}:
		default:
		}

		timer := time.NewTimer(m.options.AddResponseTimeout)
		defer timer.Stop()

		select {
		case accept = <-decisionCh:
		case <-timer.C:
		case <-m.ctx.Done():
			return
		case <-conn.Done():
			return
		}
	}

	status := "rejected"
	if accept {
		status = "accepted"
	}
	response := PeerAddResponse{
		Type:       TypePeerAddResponse,
		Status:     status,
		DeviceID:   m.options.Identity.DeviceID,
		DeviceName: m.options.Identity.DeviceName,
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := m.signPeerAddResponse(&response); err != nil {
		m.reportError(err)
		return
	}
	if err := conn.SendMessage(response); err != nil {
		m.reportError(err)
		return
	}

	if accept {
		if err := m.persistPeerConnection(conn, peerStatusOnline, true); err != nil {
			m.reportError(err)
		}
		return
	}

	m.markSuppressReconnect(peerID)
	_ = conn.Close()
}

func (m *PeerManager) handlePeerAddResponse(conn *PeerConnection, response PeerAddResponse) {
	peerID := conn.PeerDeviceID()
	if peerID == "" || response.DeviceID == "" || response.DeviceName == "" || response.Status == "" || response.Signature == "" {
		return
	}
	if response.DeviceID != peerID {
		m.reportError(fmt.Errorf("rejecting peer_add_response: sender mismatch %q != %q", response.DeviceID, peerID))
		return
	}
	if !withinTimestampSkew(response.Timestamp) {
		m.reportError(fmt.Errorf("rejecting peer_add_response from %q: timestamp outside skew", peerID))
		return
	}
	if err := m.verifyPeerAddResponse(conn, response); err != nil {
		m.reportError(err)
		return
	}
	if !stringsEqualFold(response.Status, "accepted") && !stringsEqualFold(response.Status, "rejected") {
		m.reportError(fmt.Errorf("rejecting peer_add_response from %q: invalid status %q", peerID, response.Status))
		return
	}

	m.outboundMu.Lock()
	ch := m.outboundAddResponse[peerID]
	m.outboundMu.Unlock()
	if ch == nil {
		return
	}

	select {
	case ch <- response:
	default:
	}
}

func (m *PeerManager) handlePeerRemove(conn *PeerConnection, removeMsg PeerRemove) bool {
	peerID := conn.PeerDeviceID()
	if peerID == "" || removeMsg.FromDeviceID == "" || removeMsg.Signature == "" {
		return false
	}
	if removeMsg.FromDeviceID != peerID {
		m.reportError(fmt.Errorf("rejecting peer_remove: sender mismatch %q != %q", removeMsg.FromDeviceID, peerID))
		return false
	}
	if !withinTimestampSkew(removeMsg.Timestamp) {
		m.reportError(fmt.Errorf("rejecting peer_remove from %q: timestamp outside skew", peerID))
		return false
	}
	if err := m.verifyPeerRemove(conn, removeMsg); err != nil {
		m.reportError(err)
		_ = m.sendErrorMessage(conn, "invalid_signature", "peer remove signature verification failed", "")
		return false
	}

	m.markSuppressReconnect(peerID)
	m.stopReconnect(peerID)
	_ = m.options.Store.RemovePeer(peerID)
	m.removeKnownKey(peerID)
	_ = conn.Close()
	return true
}

func (m *PeerManager) sendTextMessageWithID(peerDeviceID, messageID, content string, timestampSent int64) error {
	status := deliveryStatusPending
	signature := ""

	conn := m.getConnection(peerDeviceID)
	if conn != nil && conn.State() != StateDisconnected {
		wireMessage, wireSignature, err := m.buildEncryptedMessage(conn, messageID, content, timestampSent)
		if err != nil {
			return err
		}
		if err := conn.SendMessage(wireMessage); err == nil {
			status = deliveryStatusSent
			signature = wireSignature
		} else {
			m.reportError(fmt.Errorf("send message %q to %q failed, queueing: %w", messageID, peerDeviceID, err))
		}
	}

	message := storage.Message{
		MessageID:      messageID,
		FromDeviceID:   m.options.Identity.DeviceID,
		ToDeviceID:     peerDeviceID,
		Content:        content,
		ContentType:    messageContentTypeText,
		TimestampSent:  timestampSent,
		DeliveryStatus: status,
		Signature:      signature,
	}
	if err := m.options.Store.SaveMessage(message); err != nil {
		return err
	}

	if status == deliveryStatusPending {
		if err := m.enforceQueueLimits(peerDeviceID); err != nil {
			return err
		}
	}
	return nil
}

func (m *PeerManager) handleIncomingEncryptedMessage(conn *PeerConnection, message EncryptedMessage) {
	if message.MessageID == "" || message.FromDeviceID == "" || message.ToDeviceID == "" {
		return
	}
	if message.FromDeviceID != conn.PeerDeviceID() {
		m.reportError(fmt.Errorf("rejecting message %q: sender mismatch %q != %q", message.MessageID, message.FromDeviceID, conn.PeerDeviceID()))
		return
	}
	if message.ToDeviceID != m.options.Identity.DeviceID {
		return
	}
	if !withinTimestampSkew(message.Timestamp) {
		m.reportError(fmt.Errorf("rejecting message %q from %q: timestamp outside skew window", message.MessageID, message.FromDeviceID))
		_ = m.sendErrorMessage(conn, "timestamp_out_of_range", "message timestamp outside allowed skew", message.MessageID)
		return
	}
	if err := conn.ValidateSequence(message.Sequence); err != nil {
		m.reportError(fmt.Errorf("rejecting message %q from %q: %w", message.MessageID, message.FromDeviceID, err))
		return
	}

	seen, err := m.options.Store.HasSeenID(message.MessageID)
	if err != nil {
		m.reportError(err)
		return
	}
	if seen {
		m.reportError(fmt.Errorf("rejecting duplicate message_id %q from %q", message.MessageID, message.FromDeviceID))
		return
	}

	ciphertext, err := base64.StdEncoding.DecodeString(message.EncryptedContent)
	if err != nil {
		m.reportError(err)
		return
	}
	iv, err := base64.StdEncoding.DecodeString(message.IV)
	if err != nil {
		m.reportError(err)
		return
	}
	if err := m.verifyEncryptedMessageSignature(conn, message); err != nil {
		m.reportError(fmt.Errorf("rejecting message %q from %q: invalid signature", message.MessageID, message.FromDeviceID))
		_ = m.sendErrorMessage(conn, "invalid_signature", "message signature verification failed", message.MessageID)
		return
	}

	plaintext, err := appcrypto.Decrypt(conn.SessionKey(), iv, ciphertext)
	if err != nil {
		m.reportError(fmt.Errorf("decrypt message %q: %w", message.MessageID, err))
		_ = m.sendErrorMessage(conn, "decryption_failed", "message decryption failed", message.MessageID)
		return
	}

	receivedAt := time.Now().UnixMilli()
	contentType := message.ContentType
	if contentType == "" {
		contentType = messageContentTypeText
	}

	storedMessage := storage.Message{
		MessageID:         message.MessageID,
		FromDeviceID:      message.FromDeviceID,
		ToDeviceID:        message.ToDeviceID,
		Content:           string(plaintext),
		ContentType:       contentType,
		TimestampSent:     message.Timestamp,
		TimestampReceived: &receivedAt,
		DeliveryStatus:    deliveryStatusDelivered,
		Signature:         message.Signature,
	}
	if err := m.options.Store.SaveMessage(storedMessage); err != nil {
		m.reportError(err)
		return
	}
	if err := m.options.Store.InsertSeenID(message.MessageID, receivedAt); err != nil {
		m.reportError(err)
	}

	if m.options.OnMessageReceived != nil {
		m.options.OnMessageReceived(storedMessage)
	}

	ack := AckMessage{
		Type:         TypeAck,
		MessageID:    message.MessageID,
		FromDeviceID: m.options.Identity.DeviceID,
		Status:       deliveryStatusDelivered,
		Timestamp:    receivedAt,
	}
	if err := m.signAck(&ack); err != nil {
		m.reportError(err)
		return
	}
	if err := conn.SendMessage(ack); err != nil {
		m.reportError(err)
	}
}

func (m *PeerManager) handleAck(conn *PeerConnection, ack AckMessage) {
	if ack.MessageID == "" || ack.Signature == "" {
		return
	}
	peerID := conn.PeerDeviceID()
	if peerID == "" || ack.FromDeviceID == "" {
		return
	}
	if ack.FromDeviceID != peerID {
		m.reportError(fmt.Errorf("rejecting ack %q: sender mismatch %q != %q", ack.MessageID, ack.FromDeviceID, peerID))
		return
	}
	if err := m.verifyAck(conn, ack); err != nil {
		m.reportError(fmt.Errorf("rejecting ack %q from %q: %w", ack.MessageID, peerID, err))
		_ = m.sendErrorMessage(conn, "invalid_signature", "ack signature verification failed", ack.MessageID)
		return
	}

	stored, err := m.options.Store.GetMessageByID(ack.MessageID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			m.reportError(err)
		}
		return
	}
	if stored.FromDeviceID != m.options.Identity.DeviceID || stored.ToDeviceID != peerID {
		m.reportError(fmt.Errorf("rejecting ack %q from %q: ack does not match message route", ack.MessageID, peerID))
		return
	}

	status := ack.Status
	if status == "" {
		status = deliveryStatusDelivered
	}

	err = nil
	if status == deliveryStatusDelivered {
		err = m.options.Store.MarkDelivered(ack.MessageID)
	} else {
		err = m.options.Store.UpdateDeliveryStatus(ack.MessageID, status)
	}
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		m.reportError(err)
	}
}

func (m *PeerManager) handleProtocolError(protocolError ErrorMessage) {
	if protocolError.RelatedMessageID != "" {
		if err := m.options.Store.UpdateDeliveryStatus(protocolError.RelatedMessageID, deliveryStatusFailed); err != nil && !errors.Is(err, storage.ErrNotFound) {
			m.reportError(err)
		}
	}
	m.reportError(fmt.Errorf("peer protocol error [%s]: %s", protocolError.Code, protocolError.Message))
}

func (m *PeerManager) onConnectionRekeyNeeded(conn *PeerConnection) {
	if conn == nil || conn.State() == StateDisconnected {
		return
	}
	peerID := conn.PeerDeviceID()
	if peerID == "" {
		conn.setRekeyInProgress(false)
		return
	}
	// Deterministic initiator rule avoids simultaneous cross-initiated rekeys
	// that could otherwise derive different epoch keys.
	if m.options.Identity.DeviceID > peerID {
		conn.setRekeyInProgress(false)
		return
	}
	if err := m.initiateRekey(conn); err != nil {
		m.reportError(fmt.Errorf("rekey with %q failed: %w", conn.PeerDeviceID(), err))
		_ = conn.Close()
	}
}

func (m *PeerManager) initiateRekey(conn *PeerConnection) error {
	peerID := conn.PeerDeviceID()
	if peerID == "" {
		return errors.New("rekey: peer device ID is required")
	}

	waitCh := make(chan rekeyWaitResult, 1)
	m.rekeyMu.Lock()
	if _, exists := m.pendingRekeyWaiters[peerID]; exists {
		m.rekeyMu.Unlock()
		return nil
	}
	m.pendingRekeyWaiters[peerID] = waitCh
	m.rekeyMu.Unlock()
	defer func() {
		m.rekeyMu.Lock()
		current := m.pendingRekeyWaiters[peerID]
		if current == waitCh {
			delete(m.pendingRekeyWaiters, peerID)
		}
		m.rekeyMu.Unlock()
	}()

	privateKey, publicKey, err := appcrypto.GenerateEphemeralX25519KeyPair()
	if err != nil {
		return err
	}

	targetEpoch := conn.SessionEpoch() + 1
	request := RekeyRequest{
		Type:            TypeRekeyRequest,
		FromDeviceID:    m.options.Identity.DeviceID,
		Epoch:           targetEpoch,
		X25519PublicKey: base64.StdEncoding.EncodeToString(publicKey.Bytes()),
		Timestamp:       time.Now().UnixMilli(),
	}
	if err := m.signRekeyRequest(&request); err != nil {
		return err
	}
	if err := conn.SendMessage(request); err != nil {
		return err
	}

	timeout := m.options.RekeyResponseTimeout
	if timeout <= 0 {
		timeout = defaultRekeyResponseTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-m.ctx.Done():
		return m.ctx.Err()
	case <-conn.Done():
		if err := conn.LastError(); err != nil {
			return err
		}
		return errors.New("connection closed while waiting for rekey response")
	case <-timer.C:
		return context.DeadlineExceeded
	case result := <-waitCh:
		if result.Err != nil {
			return result.Err
		}
		response := result.Response
		if response.Epoch != targetEpoch {
			return fmt.Errorf("rekey: unexpected response epoch %d (expected %d)", response.Epoch, targetEpoch)
		}
		newKey, err := deriveRekeySessionKey(privateKey, response.X25519PublicKey, m.options.Identity.DeviceID, peerID, targetEpoch)
		if err != nil {
			return err
		}
		if err := conn.RotateSessionKey(newKey, targetEpoch); err != nil {
			return err
		}
		return nil
	}
}

func (m *PeerManager) handleRekeyRequest(conn *PeerConnection, request RekeyRequest) {
	peerID := conn.PeerDeviceID()
	if peerID == "" || request.FromDeviceID == "" || request.X25519PublicKey == "" || request.Signature == "" || request.Epoch == 0 {
		_ = conn.Close()
		return
	}
	if request.FromDeviceID != peerID {
		m.reportError(fmt.Errorf("rejecting rekey_request: sender mismatch %q != %q", request.FromDeviceID, peerID))
		_ = conn.Close()
		return
	}
	if !withinTimestampSkew(request.Timestamp) {
		m.reportError(fmt.Errorf("rejecting rekey_request from %q: timestamp outside skew", peerID))
		_ = conn.Close()
		return
	}
	if err := m.verifyRekeyRequest(conn, request); err != nil {
		m.reportError(err)
		_ = conn.Close()
		return
	}

	privateKey, publicKey, err := appcrypto.GenerateEphemeralX25519KeyPair()
	if err != nil {
		m.reportError(err)
		_ = conn.Close()
		return
	}
	newKey, err := deriveRekeySessionKey(privateKey, request.X25519PublicKey, m.options.Identity.DeviceID, peerID, request.Epoch)
	if err != nil {
		m.reportError(err)
		_ = conn.Close()
		return
	}

	response := RekeyResponse{
		Type:            TypeRekeyResponse,
		FromDeviceID:    m.options.Identity.DeviceID,
		Epoch:           request.Epoch,
		X25519PublicKey: base64.StdEncoding.EncodeToString(publicKey.Bytes()),
		Timestamp:       time.Now().UnixMilli(),
	}
	if err := m.signRekeyResponse(&response); err != nil {
		m.reportError(err)
		_ = conn.Close()
		return
	}
	if err := conn.SendMessage(response); err != nil {
		m.reportError(err)
		_ = conn.Close()
		return
	}
	if err := conn.RotateSessionKey(newKey, request.Epoch); err != nil {
		m.reportError(err)
		_ = conn.Close()
	}
}

func (m *PeerManager) handleRekeyResponse(conn *PeerConnection, response RekeyResponse) {
	peerID := conn.PeerDeviceID()
	if peerID == "" || response.FromDeviceID == "" || response.X25519PublicKey == "" || response.Signature == "" || response.Epoch == 0 {
		_ = conn.Close()
		return
	}
	if response.FromDeviceID != peerID {
		m.reportError(fmt.Errorf("rejecting rekey_response: sender mismatch %q != %q", response.FromDeviceID, peerID))
		_ = conn.Close()
		return
	}
	if !withinTimestampSkew(response.Timestamp) {
		m.reportError(fmt.Errorf("rejecting rekey_response from %q: timestamp outside skew", peerID))
		_ = conn.Close()
		return
	}
	if err := m.verifyRekeyResponse(conn, response); err != nil {
		m.reportError(err)
		_ = conn.Close()
		return
	}

	m.rekeyMu.Lock()
	waiter := m.pendingRekeyWaiters[peerID]
	m.rekeyMu.Unlock()
	if waiter == nil {
		return
	}

	select {
	case waiter <- rekeyWaitResult{Response: response}:
	default:
	}
}

func (m *PeerManager) startQueueDrain(peerID string, conn *PeerConnection) {
	if peerID == "" || conn == nil {
		return
	}

	m.drainMu.Lock()
	if m.activeDrains[peerID] {
		m.drainMu.Unlock()
		return
	}
	m.activeDrains[peerID] = true
	m.drainMu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.drainMu.Lock()
			delete(m.activeDrains, peerID)
			m.drainMu.Unlock()
		}()
		if err := m.drainPendingMessages(peerID, conn); err != nil {
			m.reportError(err)
		}
	}()
}

func (m *PeerManager) drainPendingMessages(peerID string, conn *PeerConnection) error {
	if err := m.enforceQueueLimits(peerID); err != nil {
		return err
	}

	pending, err := m.options.Store.GetPendingMessages(peerID)
	if err != nil {
		return err
	}
	for _, pendingMessage := range pending {
		select {
		case <-m.ctx.Done():
			return nil
		default:
		}

		if conn.State() == StateDisconnected {
			return nil
		}
		if err := m.sendPendingMessage(conn, pendingMessage); err != nil {
			return err
		}
	}
	return nil
}

func (m *PeerManager) sendPendingMessage(conn *PeerConnection, message storage.Message) error {
	wireMessage, _, err := m.buildEncryptedMessage(conn, message.MessageID, message.Content, message.TimestampSent)
	if err != nil {
		return err
	}
	if err := conn.SendMessage(wireMessage); err != nil {
		return err
	}

	if err := m.options.Store.UpdateDeliveryStatus(message.MessageID, deliveryStatusSent); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	return nil
}

func (m *PeerManager) enforceQueueLimits(peerID string) error {
	if peerID == "" {
		return nil
	}

	_, err := m.options.Store.PruneExpiredQueue(time.Now().Add(-maxQueueAge).UnixMilli())
	if err != nil {
		return err
	}

	pending, err := m.options.Store.GetPendingMessages(peerID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	var totalBytes int64
	for _, pendingMessage := range pending {
		totalBytes += int64(len(pendingMessage.Content))
	}

	dropped := 0
	for len(pending)-dropped > maxQueuedMessages || totalBytes > maxQueuedBytesPerPeer {
		oldest := pending[dropped]
		if err := m.options.Store.UpdateDeliveryStatus(oldest.MessageID, deliveryStatusFailed); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
		totalBytes -= int64(len(oldest.Content))
		dropped++
	}

	if dropped > 0 && m.options.OnQueueOverflow != nil {
		m.options.OnQueueOverflow(peerID, dropped)
	}

	return nil
}

func (m *PeerManager) buildEncryptedMessage(conn *PeerConnection, messageID, content string, timestamp int64) (EncryptedMessage, string, error) {
	if timestamp == 0 {
		timestamp = time.Now().UnixMilli()
	}

	ciphertext, iv, err := appcrypto.Encrypt(conn.SessionKey(), []byte(content))
	if err != nil {
		return EncryptedMessage{}, "", err
	}

	wire := EncryptedMessage{
		Type:             TypeMessage,
		MessageID:        messageID,
		FromDeviceID:     m.options.Identity.DeviceID,
		ToDeviceID:       conn.PeerDeviceID(),
		ContentType:      messageContentTypeText,
		Sequence:         conn.NextSendSequence(),
		EncryptedContent: base64.StdEncoding.EncodeToString(ciphertext),
		IV:               base64.StdEncoding.EncodeToString(iv),
		Timestamp:        timestamp,
	}
	if err := m.signEncryptedMessage(&wire); err != nil {
		return EncryptedMessage{}, "", err
	}

	return wire, wire.Signature, nil
}

func (m *PeerManager) sendErrorMessage(conn *PeerConnection, code, message, relatedMessageID string) error {
	if conn == nil {
		return nil
	}
	errorMessage := ErrorMessage{
		Type:             TypeError,
		Code:             code,
		Message:          message,
		RelatedMessageID: relatedMessageID,
		Timestamp:        time.Now().UnixMilli(),
	}
	return conn.SendMessage(errorMessage)
}

func (m *PeerManager) startReconnect(peerID string) {
	if peerID == "" {
		return
	}

	m.reconnectMu.Lock()
	if _, exists := m.reconnectWorkers[peerID]; exists {
		m.reconnectMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.reconnectWorkers[peerID] = cancel
	m.reconnectMu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			m.reconnectMu.Lock()
			delete(m.reconnectWorkers, peerID)
			m.reconnectMu.Unlock()
		}()

		attempt := 0
		for {
			delay := m.backoffForAttempt(attempt)
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return
			}

			address, err := m.resolvePeerAddress(peerID)
			if err != nil {
				attempt++
				continue
			}
			conn, err := m.dialAndRegister(address)
			if err != nil {
				attempt++
				continue
			}
			if conn.PeerDeviceID() != peerID {
				_ = conn.Close()
				attempt++
				continue
			}
			return
		}
	}()
}

func (m *PeerManager) stopReconnect(peerID string) {
	m.reconnectMu.Lock()
	cancel, exists := m.reconnectWorkers[peerID]
	if exists {
		delete(m.reconnectWorkers, peerID)
	}
	m.reconnectMu.Unlock()
	if exists {
		cancel()
	}
}

func (m *PeerManager) backoffForAttempt(attempt int) time.Duration {
	backoff := m.options.ReconnectBackoff
	if len(backoff) == 0 {
		return 0
	}
	if attempt < len(backoff) {
		return backoff[attempt]
	}
	return backoff[len(backoff)-1]
}

func (m *PeerManager) resolvePeerAddress(peerID string) (string, error) {
	peer, err := m.options.Store.GetPeer(peerID)
	if err != nil {
		return "", err
	}
	if peer.Status == peerStatusBlocked || peer.Status == peerStatusPending {
		return "", fmt.Errorf("peer %q is not reconnectable with status %q", peerID, peer.Status)
	}
	if peer.LastKnownIP == nil || peer.LastKnownPort == nil {
		return "", fmt.Errorf("peer %q has no known endpoint", peerID)
	}
	return net.JoinHostPort(*peer.LastKnownIP, strconv.Itoa(*peer.LastKnownPort)), nil
}

func (m *PeerManager) dialAndRegister(address string) (*PeerConnection, error) {
	conn, err := Dial(address, HandshakeOptions{
		Identity:            m.options.Identity,
		KnownPeerKeys:       m.knownKeysSnapshot(),
		OnKeyChangeDecision: m.options.OnKeyChange,
		ConnectionTimeout:   m.options.ConnectionTimeout,
		KeepAliveInterval:   m.options.KeepAliveInterval,
		KeepAliveTimeout:    m.options.KeepAliveTimeout,
		FrameReadTimeout:    m.options.FrameReadTimeout,
		RekeyInterval:       m.options.RekeyInterval,
		RekeyAfterBytes:     m.options.RekeyAfterBytes,
		AutoRespondPing:     m.options.AutoRespondPing,
	})
	if err != nil {
		return nil, err
	}

	m.registerConnection(conn)
	return conn, nil
}

func (m *PeerManager) handshakeOptionsForServer() HandshakeOptions {
	return HandshakeOptions{
		Identity:            m.options.Identity,
		KnownPeerKeys:       m.knownKeysSnapshot(),
		OnKeyChangeDecision: m.options.OnKeyChange,
		ConnectionTimeout:   m.options.ConnectionTimeout,
		KeepAliveInterval:   m.options.KeepAliveInterval,
		KeepAliveTimeout:    m.options.KeepAliveTimeout,
		FrameReadTimeout:    m.options.FrameReadTimeout,
		RekeyInterval:       m.options.RekeyInterval,
		RekeyAfterBytes:     m.options.RekeyAfterBytes,
		AutoRespondPing:     m.options.AutoRespondPing,
	}
}

func (m *PeerManager) persistPeerConnection(conn *PeerConnection, status string, createIfMissing bool) error {
	peerID := conn.PeerDeviceID()
	if peerID == "" {
		return errors.New("peer ID is required")
	}

	now := time.Now().UnixMilli()
	endpointIP, endpointPort := remoteEndpoint(conn.RemoteAddr())

	existing, err := m.options.Store.GetPeer(peerID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	if errors.Is(err, storage.ErrNotFound) {
		if !createIfMissing {
			return storage.ErrNotFound
		}

		fingerprint, err := fingerprintFromBase64(conn.PeerPublicKey())
		if err != nil {
			return err
		}

		peer := storage.Peer{
			DeviceID:          peerID,
			DeviceName:        conn.PeerDeviceName(),
			Ed25519PublicKey:  conn.PeerPublicKey(),
			KeyFingerprint:    fingerprint,
			Status:            status,
			AddedTimestamp:    now,
			LastSeenTimestamp: &now,
		}
		if endpointIP != "" && endpointPort > 0 {
			peer.LastKnownIP = &endpointIP
			peer.LastKnownPort = &endpointPort
		}

		if err := m.options.Store.AddPeer(peer); err != nil {
			return err
		}
		m.setKnownKey(peerID, conn.PeerPublicKey())
		return nil
	}

	if err := m.options.Store.UpdatePeerStatus(peerID, status, now); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	if endpointIP != "" && endpointPort > 0 {
		_ = m.options.Store.UpdatePeerEndpoint(peerID, endpointIP, endpointPort, now)
	}
	if existing.Ed25519PublicKey == "" && conn.PeerPublicKey() != "" {
		m.setKnownKey(peerID, conn.PeerPublicKey())
	}
	return nil
}

func (m *PeerManager) loadKnownPeers() error {
	peers, err := m.options.Store.ListPeers()
	if err != nil {
		return err
	}
	for _, peer := range peers {
		if peer.Ed25519PublicKey != "" {
			m.setKnownKey(peer.DeviceID, peer.Ed25519PublicKey)
		}
	}
	return nil
}

func (m *PeerManager) ensureLocalPeerRecord() error {
	_, err := m.options.Store.GetPeer(m.options.Identity.DeviceID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return err
	}

	pubKeyBase64 := base64.StdEncoding.EncodeToString(m.options.Identity.Ed25519PublicKey)
	fingerprint := appcrypto.KeyFingerprint(m.options.Identity.Ed25519PublicKey)
	now := time.Now().UnixMilli()

	localPeer := storage.Peer{
		DeviceID:          m.options.Identity.DeviceID,
		DeviceName:        m.options.Identity.DeviceName,
		Ed25519PublicKey:  pubKeyBase64,
		KeyFingerprint:    fingerprint,
		Status:            peerStatusBlocked,
		AddedTimestamp:    now,
		LastSeenTimestamp: &now,
	}
	return m.options.Store.AddPeer(localPeer)
}

func (m *PeerManager) knownKeysSnapshot() map[string]string {
	m.knownKeyMu.RLock()
	defer m.knownKeyMu.RUnlock()

	out := make(map[string]string, len(m.knownKeys))
	for key, value := range m.knownKeys {
		out[key] = value
	}
	return out
}

func (m *PeerManager) setKnownKey(peerDeviceID, key string) {
	if peerDeviceID == "" || key == "" {
		return
	}
	m.knownKeyMu.Lock()
	m.knownKeys[peerDeviceID] = key
	m.knownKeyMu.Unlock()
}

func (m *PeerManager) removeKnownKey(peerDeviceID string) {
	m.knownKeyMu.Lock()
	delete(m.knownKeys, peerDeviceID)
	m.knownKeyMu.Unlock()
}

func (m *PeerManager) getConnection(peerDeviceID string) *PeerConnection {
	m.connMu.RLock()
	defer m.connMu.RUnlock()
	return m.connections[peerDeviceID]
}

func (m *PeerManager) isOutboundAddPending(peerDeviceID string) bool {
	m.outboundMu.Lock()
	defer m.outboundMu.Unlock()
	return m.outboundAddPending[peerDeviceID]
}

func (m *PeerManager) removeInboundAddPendingIfMatch(peerDeviceID string, decisionCh chan bool) {
	m.inboundMu.Lock()
	current := m.inboundAddPending[peerDeviceID]
	if current == decisionCh {
		delete(m.inboundAddPending, peerDeviceID)
	}
	m.inboundMu.Unlock()
}

func (m *PeerManager) markSuppressReconnect(peerDeviceID string) {
	m.suppressMu.Lock()
	m.suppressReconnect[peerDeviceID] = true
	m.suppressMu.Unlock()
}

func (m *PeerManager) consumeSuppressReconnect(peerDeviceID string) bool {
	m.suppressMu.Lock()
	defer m.suppressMu.Unlock()
	suppress := m.suppressReconnect[peerDeviceID]
	delete(m.suppressReconnect, peerDeviceID)
	return suppress
}

func (m *PeerManager) reportError(err error) {
	if err == nil {
		return
	}
	select {
	case m.errors <- err:
	default:
	}
}

func decodePeerPublicKey(keyBase64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, fmt.Errorf("decode peer Ed25519 public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errors.New("invalid peer Ed25519 public key size")
	}
	return ed25519.PublicKey(raw), nil
}

func withinTimestampSkew(timestamp int64) bool {
	if timestamp == 0 {
		return false
	}
	delta := time.Since(time.UnixMilli(timestamp))
	if delta < 0 {
		delta = -delta
	}
	return delta <= maxTimestampSkew
}

func remoteEndpoint(addr net.Addr) (string, int) {
	if addr == nil {
		return "", 0
	}
	host, portText, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "", 0
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0
	}
	return host, port
}

func fingerprintFromBase64(keyBase64 string) (string, error) {
	publicKeyBytes, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return "", fmt.Errorf("decode peer public key: %w", err)
	}
	if len(publicKeyBytes) != ed25519.PublicKeySize {
		return "", errors.New("invalid peer Ed25519 public key size")
	}
	return appcrypto.KeyFingerprint(ed25519.PublicKey(publicKeyBytes)), nil
}

// NotifyPeerDiscovered is called when mDNS discovers a peer on the network.
// If the peer is a known (added) peer that is not currently connected,
// it updates the stored endpoint and starts a reconnect attempt.
func (m *PeerManager) NotifyPeerDiscovered(deviceID, ip string, port int) {
	if deviceID == "" || deviceID == m.options.Identity.DeviceID {
		return
	}

	peer, err := m.options.Store.GetPeer(deviceID)
	if err != nil {
		return
	}
	if peer.Status == peerStatusBlocked {
		return
	}

	if ip != "" && port > 0 {
		_ = m.options.Store.UpdatePeerEndpoint(deviceID, ip, port, time.Now().UnixMilli())
	}

	conn := m.getConnection(deviceID)
	if conn != nil && conn.State() != StateDisconnected {
		return
	}

	m.startReconnect(deviceID)
}

func stringsEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x := a[i]
		y := b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func (m *PeerManager) signPeerAddRequest(msg *PeerAddRequest) error {
	signable := *msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyPeerAddRequest(conn *PeerConnection, msg PeerAddRequest) error {
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return fmt.Errorf("decode peer add request signature: %w", err)
	}
	signable := msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid peer add request signature")
	}
	return nil
}

func (m *PeerManager) signPeerAddResponse(msg *PeerAddResponse) error {
	signable := *msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyPeerAddResponse(conn *PeerConnection, msg PeerAddResponse) error {
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return fmt.Errorf("decode peer add response signature: %w", err)
	}
	signable := msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid peer add response signature")
	}
	return nil
}

func (m *PeerManager) signPeerRemove(msg *PeerRemove) error {
	signable := *msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyPeerRemove(conn *PeerConnection, msg PeerRemove) error {
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return fmt.Errorf("decode peer remove signature: %w", err)
	}
	signable := msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid peer remove signature")
	}
	return nil
}

func (m *PeerManager) signEncryptedMessage(msg *EncryptedMessage) error {
	signable := *msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) signRekeyRequest(msg *RekeyRequest) error {
	signable := *msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyRekeyRequest(conn *PeerConnection, msg RekeyRequest) error {
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return fmt.Errorf("decode rekey request signature: %w", err)
	}
	signable := msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid rekey request signature")
	}
	return nil
}

func (m *PeerManager) signRekeyResponse(msg *RekeyResponse) error {
	signable := *msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyRekeyResponse(conn *PeerConnection, msg RekeyResponse) error {
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return fmt.Errorf("decode rekey response signature: %w", err)
	}
	signable := msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid rekey response signature")
	}
	return nil
}

func (m *PeerManager) signAck(msg *AckMessage) error {
	signable := *msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	signature, err := appcrypto.Sign(m.options.Identity.Ed25519PrivateKey, raw)
	if err != nil {
		return err
	}
	msg.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func (m *PeerManager) verifyAck(conn *PeerConnection, msg AckMessage) error {
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return fmt.Errorf("decode ack signature: %w", err)
	}
	signable := msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid ack signature")
	}
	return nil
}

func (m *PeerManager) verifyEncryptedMessageSignature(conn *PeerConnection, msg EncryptedMessage) error {
	if msg.Signature == "" {
		return errors.New("message signature is required")
	}
	publicKey, err := decodePeerPublicKey(conn.PeerPublicKey())
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.DecodeString(msg.Signature)
	if err != nil {
		return fmt.Errorf("decode message signature: %w", err)
	}
	signable := msg
	signable.Signature = ""
	raw, err := json.Marshal(signable)
	if err != nil {
		return err
	}
	if !appcrypto.Verify(publicKey, raw, signature) {
		return errors.New("invalid message signature")
	}
	return nil
}
