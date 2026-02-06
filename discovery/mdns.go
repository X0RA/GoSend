package discovery

import (
	"context"
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
	DefaultScanTimeout = 3 * time.Second
	// DefaultTTL is the intended mDNS record TTL in seconds.
	DefaultTTL = 120
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
	TTL             uint32

	SelfDeviceID   string
	DeviceName     string
	ListeningPort  int
	KeyFingerprint string

	registerFn registerFunc
	browseFn   browseFunc
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
	if out.registerFn == nil {
		out.registerFn = zeroconf.Register
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

	txt := []string{
		"device_id=" + cfg.SelfDeviceID,
		"version=" + strconv.Itoa(cfg.Version),
		"key_fingerprint=" + cfg.KeyFingerprint,
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
