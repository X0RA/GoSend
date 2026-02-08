package discovery

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	// DefaultService is the mDNS service name without domain suffix.
	DefaultService = "_p2pchat._tcp"
	// DefaultDomain is the mDNS domain.
	DefaultDomain = "local."
	// DefaultVersion is the TXT record protocol version.
	DefaultVersion = 1
	// DefaultRefreshInterval is the background peer discovery interval.
	DefaultRefreshInterval = 10 * time.Second
	// DefaultScanTimeout bounds each discovery scan.
	DefaultScanTimeout = 5 * time.Second
	// DefaultTTL is the intended mDNS record TTL in seconds.
	DefaultTTL = 120
	// DefaultPeerStaleAfter keeps a discovered peer visible across transient missed scans.
	DefaultPeerStaleAfter = 30 * time.Second

	discoveryTokenTXTKey = "discovery_token"
	discoveryTokenScope  = "gosend-discovery-token-v1"
)

type registerFunc func(instance, service, domain string, port int, text []string, ifaces []net.Interface) (*zeroconf.Server, error)
type browseFunc func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error

// Config controls mDNS broadcaster and scanner behavior.
type Config struct {
	Service         string
	Domain          string
	Version         int
	RefreshInterval time.Duration
	ScanTimeout     time.Duration
	PeerStaleAfter  time.Duration
	TTL             uint32

	SelfDeviceID      string
	DeviceName        string
	ListeningPort     int
	KeyFingerprint    string
	Ed25519PublicKey  []byte
	KnownPeerProvider func() []KnownPeerIdentity
	Now               func() time.Time

	registerFn registerFunc
	browseFn   browseFunc
}

// KnownPeerIdentity provides the minimum identity material needed for discovery-token verification.
type KnownPeerIdentity struct {
	DeviceID         string
	Ed25519PublicKey string
}

func (c Config) withDefaults() Config {
	out := c
	if out.Service == "" {
		out.Service = DefaultService
	}
	if out.Domain == "" {
		out.Domain = DefaultDomain
	}
	if out.Version == 0 {
		out.Version = DefaultVersion
	}
	if out.RefreshInterval <= 0 {
		out.RefreshInterval = DefaultRefreshInterval
	}
	if out.ScanTimeout <= 0 {
		out.ScanTimeout = DefaultScanTimeout
	}
	if out.TTL == 0 {
		out.TTL = DefaultTTL
	}
	if out.PeerStaleAfter <= 0 {
		out.PeerStaleAfter = DefaultPeerStaleAfter
		ttlBased := time.Duration(out.TTL) * time.Second * 2
		if ttlBased > out.PeerStaleAfter {
			out.PeerStaleAfter = ttlBased
		}
		if out.RefreshInterval > 0 {
			candidate := out.RefreshInterval * 6
			if candidate > out.PeerStaleAfter {
				out.PeerStaleAfter = candidate
			}
		}
	}
	if out.registerFn == nil {
		out.registerFn = zeroconf.Register
	}
	if out.Now == nil {
		out.Now = time.Now
	}
	return out
}

func (c Config) validateForBroadcast() error {
	if strings.TrimSpace(c.SelfDeviceID) == "" {
		return errors.New("self device ID is required")
	}
	if strings.TrimSpace(c.DeviceName) == "" {
		return errors.New("device name is required")
	}
	if c.ListeningPort <= 0 {
		return errors.New("listening port must be > 0")
	}
	if len(c.Ed25519PublicKey) == 0 {
		return errors.New("ed25519 public key is required")
	}
	return nil
}

func (c Config) validateForScan() error {
	if strings.TrimSpace(c.SelfDeviceID) == "" {
		return errors.New("self device ID is required")
	}
	return nil
}

// Broadcaster advertises local device presence via mDNS.
type Broadcaster struct {
	server *zeroconf.Server
}

// StartBroadcaster registers and starts mDNS broadcast.
func StartBroadcaster(config Config) (*Broadcaster, error) {
	cfg := config.withDefaults()
	if err := cfg.validateForBroadcast(); err != nil {
		return nil, err
	}
	now := time.Now()
	if cfg.Now != nil {
		now = cfg.Now()
	}
	token := ComputeDiscoveryToken(cfg.SelfDeviceID, cfg.Ed25519PublicKey, now)
	if token == "" {
		return nil, errors.New("failed to derive discovery token")
	}

	txt := []string{
		discoveryTokenTXTKey + "=" + token,
		"version=" + strconv.Itoa(cfg.Version),
	}

	server, err := cfg.registerFn(cfg.DeviceName, cfg.Service, cfg.Domain, cfg.ListeningPort, txt, nil)
	if err != nil {
		return nil, fmt.Errorf("register mDNS service: %w", err)
	}

	return &Broadcaster{server: server}, nil
}

// Stop stops mDNS broadcasting.
func (b *Broadcaster) Stop() {
	if b == nil || b.server == nil {
		return
	}
	b.server.Shutdown()
}

// Service coordinates mDNS broadcast and scanning.
type Service struct {
	Broadcaster *Broadcaster
	Scanner     *PeerScanner
}

// Start starts broadcaster and scanner using one config.
func Start(config Config) (*Service, error) {
	cfg := config.withDefaults()

	broadcaster, err := StartBroadcaster(cfg)
	if err != nil {
		return nil, err
	}

	scanner, err := NewPeerScanner(cfg)
	if err != nil {
		broadcaster.Stop()
		return nil, err
	}
	if err := scanner.Start(); err != nil {
		broadcaster.Stop()
		return nil, err
	}

	return &Service{
		Broadcaster: broadcaster,
		Scanner:     scanner,
	}, nil
}

// Stop stops scanner and broadcaster.
func (s *Service) Stop() {
	if s == nil {
		return
	}
	if s.Scanner != nil {
		s.Scanner.Stop()
	}
	if s.Broadcaster != nil {
		s.Broadcaster.Stop()
	}
}

// ComputeDiscoveryToken returns the rotating HMAC token for one device identity at the provided hour.
func ComputeDiscoveryToken(deviceID string, ed25519PublicKey []byte, now time.Time) string {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || len(ed25519PublicKey) == 0 {
		return ""
	}

	hourBucket := now.UTC().Unix() / 3600
	payload := deviceID + "|" + strconv.FormatInt(hourBucket, 10)
	mac := hmac.New(sha256.New, deriveDiscoveryTokenKey(ed25519PublicKey))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyDiscoveryToken verifies a token for one device identity at the provided hour.
func VerifyDiscoveryToken(token, deviceID string, ed25519PublicKey []byte, now time.Time) bool {
	expected := ComputeDiscoveryToken(deviceID, ed25519PublicKey, now)
	if expected == "" || strings.TrimSpace(token) == "" {
		return false
	}
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(token))), []byte(expected))
}

func deriveDiscoveryTokenKey(ed25519PublicKey []byte) []byte {
	material := make([]byte, 0, len(discoveryTokenScope)+len(ed25519PublicKey))
	material = append(material, []byte(discoveryTokenScope)...)
	material = append(material, '|')
	material = append(material, ed25519PublicKey...)
	sum := sha256.Sum256(material)
	return sum[:]
}
