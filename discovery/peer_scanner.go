package discovery

import (
	"context"
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
	KeyFingerprint string
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
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			return nil, err
		}
		browse = resolver.Browse
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
			case entry := <-entries:
				if entry == nil {
					continue
				}
				peer, ok := parseEntry(entry, s.cfg.SelfDeviceID)
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
	if browseErr != nil {
		return browseErr
	}

	<-scanCtx.Done()
	<-collectorDone
	collectedMu.Lock()
	next := collected
	collectedMu.Unlock()

	s.applySnapshot(next)

	// A timeout just means this scan window ended naturally.
	if err := scanCtx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (s *PeerScanner) applySnapshot(next map[string]DiscoveredPeer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.peers
	s.peers = next

	for id, peer := range next {
		old, exists := previous[id]
		if !exists || !peersEqual(old, peer) {
			s.emitEvent(Event{Type: EventPeerUpserted, Peer: peer})
		}
	}

	for id, peer := range previous {
		if _, exists := next[id]; !exists {
			s.emitEvent(Event{Type: EventPeerRemoved, Peer: peer})
		}
	}
}

func (s *PeerScanner) emitEvent(event Event) {
	select {
	case s.events <- event:
	default:
	}
}

func parseEntry(entry *zeroconf.ServiceEntry, selfDeviceID string) (DiscoveredPeer, bool) {
	txt := txtToMap(entry.Text)

	deviceID := strings.TrimSpace(txt["device_id"])
	if deviceID == "" || deviceID == selfDeviceID {
		return DiscoveredPeer{}, false
	}

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

	name := strings.TrimSpace(entry.Instance)
	if name == "" {
		name = strings.TrimSpace(entry.HostName)
	}
	if name == "" {
		name = deviceID
	}

	return DiscoveredPeer{
		DeviceID:       deviceID,
		DeviceName:     name,
		KeyFingerprint: strings.TrimSpace(txt["key_fingerprint"]),
		Version:        version,
		HostName:       entry.HostName,
		Port:           entry.Port,
		Addresses:      addresses,
	}, true
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
		a.KeyFingerprint != b.KeyFingerprint ||
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
