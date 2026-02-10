package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	// EventPeerUpserted is emitted when a peer appears or metadata changes.
	EventPeerUpserted EventType = "peer_upserted"
	// EventPeerRemoved is emitted when a previously seen peer disappears.
	EventPeerRemoved EventType = "peer_removed"
)

// EventType identifies peer discovery updates.
type EventType string

// Event carries discovery updates for UI/network consumers.
type Event struct {
	Type EventType
	Peer DiscoveredPeer
}

// DiscoveredPeer contains a discovered LAN endpoint.
type DiscoveredPeer struct {
	DeviceID       string
	DeviceName     string
	DiscoveryToken string
	Version        int
	HostName       string
	Port           int
	Addresses      []string
	LastSeen       time.Time
}

type refreshRequest struct {
	ctx  context.Context
	done chan error
}

// PeerScanner discovers peers with periodic and manual mDNS browse operations.
type PeerScanner struct {
	cfg Config

	browse browseFunc

	mu    sync.RWMutex
	peers map[string]DiscoveredPeer

	events chan Event

	startOnce sync.Once
	stopOnce  sync.Once
	startErr  error

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	refreshRequests chan refreshRequest
}

// NewPeerScanner creates a scanner with config defaults applied.
func NewPeerScanner(config Config) (*PeerScanner, error) {
	cfg := config.withDefaults()
	if err := cfg.validateForScan(); err != nil {
		return nil, err
	}

	browse := cfg.browseFn
	if browse == nil {
		browse = func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			resolver, err := zeroconf.NewResolver(nil)
			if err != nil {
				return err
			}
			return resolver.Browse(ctx, service, domain, entries)
		}
	}

	return &PeerScanner{
		cfg:             cfg,
		browse:          browse,
		peers:           make(map[string]DiscoveredPeer),
		events:          make(chan Event, 128),
		refreshRequests: make(chan refreshRequest),
	}, nil
}

// Start begins background peer scanning.
func (s *PeerScanner) Start() error {
	s.startOnce.Do(func() {
		s.ctx, s.cancel = context.WithCancel(context.Background())
		s.wg.Add(1)
		go s.loop()
	})
	return s.startErr
}

// Stop stops background scanning.
func (s *PeerScanner) Stop() {
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.wg.Wait()
		close(s.events)
	})
}

// Events provides asynchronous discovery updates.
func (s *PeerScanner) Events() <-chan Event {
	return s.events
}

// Refresh triggers an immediate scan.
func (s *PeerScanner) Refresh(ctx context.Context) error {
	if s.ctx == nil {
		return errors.New("peer scanner is not started")
	}

	req := refreshRequest{
		ctx:  ctx,
		done: make(chan error, 1),
	}

	select {
	case s.refreshRequests <- req:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return errors.New("peer scanner is stopped")
	}

	select {
	case err := <-req.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return errors.New("peer scanner is stopped")
	}
}

// ListPeers returns the current in-memory discovered peers snapshot.
func (s *PeerScanner) ListPeers() []DiscoveredPeer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]DiscoveredPeer, 0, len(s.peers))
	for _, peer := range s.peers {
		out = append(out, peer)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].DeviceName == out[j].DeviceName {
			return out[i].DeviceID < out[j].DeviceID
		}
		return out[i].DeviceName < out[j].DeviceName
	})
	return out
}

func (s *PeerScanner) loop() {
	defer s.wg.Done()

	// Prime the available peer list immediately.
	s.runScan(context.Background())

	ticker := time.NewTicker(s.cfg.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runScan(context.Background())
		case req := <-s.refreshRequests:
			req.done <- s.runScan(req.ctx)
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *PeerScanner) runScan(requestCtx context.Context) error {
	scanCtx, cancel := context.WithTimeout(s.ctx, s.cfg.ScanTimeout)
	defer cancel()

	if requestCtx != nil {
		go func() {
			select {
			case <-requestCtx.Done():
				cancel()
			case <-scanCtx.Done():
			}
		}()
	}

	entries := make(chan *zeroconf.ServiceEntry, 32)
	collected := make(map[string]DiscoveredPeer)
	var collectedMu sync.Mutex
	collectorDone := make(chan struct{})

	go func() {
		defer close(collectorDone)
		for {
			select {
			case <-scanCtx.Done():
				return
			case entry, ok := <-entries:
				if !ok {
					return
				}
				if entry == nil {
					continue
				}
				peer, ok := parseEntry(entry, s.cfg)
				if !ok {
					continue
				}
				peer.LastSeen = time.Now()
				collectedMu.Lock()
				collected[peer.DeviceID] = peer
				collectedMu.Unlock()
			}
		}
	}()

	browseErr := s.browse(scanCtx, s.cfg.Service, s.cfg.Domain, entries)
	if browseErr != nil && !errors.Is(browseErr, context.DeadlineExceeded) && !errors.Is(browseErr, context.Canceled) {
		return browseErr
	}

	<-scanCtx.Done()
	<-collectorDone
	collectedMu.Lock()
	next := collected
	collectedMu.Unlock()

	s.applySnapshot(next)

	// A timeout/cancel just means this scan window ended naturally.
	if err := scanCtx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (s *PeerScanner) applySnapshot(next map[string]DiscoveredPeer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.peers
	merged := make(map[string]DiscoveredPeer, len(previous)+len(next))
	for id, peer := range previous {
		merged[id] = peer
	}

	for id, peer := range next {
		old, exists := previous[id]
		if !exists || !peersEqual(old, peer) {
			s.emitEvent(Event{Type: EventPeerUpserted, Peer: peer})
		}
		merged[id] = peer
	}

	now := time.Now()
	for id, peer := range merged {
		if _, exists := next[id]; !exists {
			if !peer.LastSeen.IsZero() && now.Sub(peer.LastSeen) < s.cfg.PeerStaleAfter {
				continue
			}
			delete(merged, id)
			s.emitEvent(Event{Type: EventPeerRemoved, Peer: peer})
		}
	}

	s.peers = merged
}

func (s *PeerScanner) emitEvent(event Event) {
	select {
	case s.events <- event:
	default:
	}
}

func parseEntry(entry *zeroconf.ServiceEntry, cfg Config) (DiscoveredPeer, bool) {
	txt := txtToMap(entry.Text)
	token := strings.TrimSpace(txt[discoveryTokenTXTKey])

	version := 0
	if txt["version"] != "" {
		if parsed, err := strconv.Atoi(txt["version"]); err == nil {
			version = parsed
		}
	}

	addresses := make([]string, 0, len(entry.AddrIPv4)+len(entry.AddrIPv6))
	seen := make(map[string]struct{})
	for _, ip := range append(entry.AddrIPv4, entry.AddrIPv6...) {
		if ip == nil {
			continue
		}
		raw := ip.String()
		if raw == "" {
			continue
		}
		if _, exists := seen[raw]; exists {
			continue
		}
		seen[raw] = struct{}{}
		addresses = append(addresses, raw)
	}
	sort.Strings(addresses)

	name := strings.TrimSpace(unescapeMDNSInstance(entry.Instance))
	if name == "" {
		name = strings.TrimSpace(entry.HostName)
	}
	if name == "" {
		name = "Unknown Peer"
	}

	now := time.Now()
	if cfg.Now != nil {
		now = cfg.Now()
	}
	deviceID, matchedKnownPeer := resolveKnownPeerByToken(token, cfg, now)
	if deviceID == cfg.SelfDeviceID {
		return DiscoveredPeer{}, false
	}
	if !matchedKnownPeer && token != "" && len(cfg.Ed25519PublicKey) > 0 {
		if verifyTokenWithHourSkew(token, cfg.SelfDeviceID, cfg.Ed25519PublicKey, now, 1) {
			return DiscoveredPeer{}, false
		}
	}
	if deviceID == "" {
		deviceID = syntheticUnknownDeviceID(entry, addresses)
	}
	if deviceID == "" {
		return DiscoveredPeer{}, false
	}

	return DiscoveredPeer{
		DeviceID:       deviceID,
		DeviceName:     name,
		DiscoveryToken: token,
		Version:        version,
		HostName:       entry.HostName,
		Port:           entry.Port,
		Addresses:      addresses,
	}, true
}

func unescapeMDNSInstance(instance string) string {
	if strings.IndexByte(instance, '\\') < 0 {
		return instance
	}
	out := make([]byte, 0, len(instance))
	for i := 0; i < len(instance); i++ {
		ch := instance[i]
		if ch == '\\' && i+1 < len(instance) {
			i++
			out = append(out, instance[i])
			continue
		}
		out = append(out, ch)
	}
	return string(out)
}

func resolveKnownPeerByToken(token string, cfg Config, now time.Time) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" || cfg.KnownPeerProvider == nil {
		return "", false
	}

	for _, peer := range cfg.KnownPeerProvider() {
		deviceID := strings.TrimSpace(peer.DeviceID)
		if deviceID == "" {
			continue
		}
		keyBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(peer.Ed25519PublicKey))
		if err != nil || len(keyBytes) == 0 {
			continue
		}
		if verifyTokenWithHourSkew(token, deviceID, keyBytes, now, 1) {
			return deviceID, true
		}
	}

	return "", false
}

func verifyTokenWithHourSkew(token, deviceID string, ed25519PublicKey []byte, now time.Time, hourSkew int) bool {
	for offset := -hourSkew; offset <= hourSkew; offset++ {
		candidate := now.Add(time.Duration(offset) * time.Hour)
		if VerifyDiscoveryToken(token, deviceID, ed25519PublicKey, candidate) {
			return true
		}
	}
	return false
}

func syntheticUnknownDeviceID(entry *zeroconf.ServiceEntry, addresses []string) string {
	builder := strings.Builder{}
	builder.WriteString(strings.TrimSpace(entry.Instance))
	builder.WriteString("|")
	builder.WriteString(strings.TrimSpace(entry.HostName))
	builder.WriteString("|")
	builder.WriteString(strconv.Itoa(entry.Port))
	builder.WriteString("|")
	builder.WriteString(strings.Join(addresses, ","))
	value := strings.TrimSpace(builder.String())
	if value == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(value))
	return "unknown-" + hex.EncodeToString(sum[:8])
}

func txtToMap(text []string) map[string]string {
	out := make(map[string]string, len(text))
	for _, entry := range text {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(parts[1])
	}
	return out
}

func peersEqual(a, b DiscoveredPeer) bool {
	if a.DeviceID != b.DeviceID ||
		a.DeviceName != b.DeviceName ||
		a.DiscoveryToken != b.DiscoveryToken ||
		a.Version != b.Version ||
		a.HostName != b.HostName ||
		a.Port != b.Port ||
		len(a.Addresses) != len(b.Addresses) {
		return false
	}
	for i := range a.Addresses {
		if a.Addresses[i] != b.Addresses[i] {
			return false
		}
	}
	return true
}
