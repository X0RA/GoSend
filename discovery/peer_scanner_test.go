package discovery

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestPeerScannerFiltersSelfAndManualRefresh(t *testing.T) {
	var browseCalls int32
	now := time.Unix(1_706_000_000, 0).UTC()
	selfPub := deterministicDiscoveryPublicKey("self-device")
	peerOnePub := deterministicDiscoveryPublicKey("peer-1")
	peerTwoPub := deterministicDiscoveryPublicKey("peer-2")

	cfg := Config{
		SelfDeviceID:     "self-device",
		Ed25519PublicKey: selfPub,
		RefreshInterval:  time.Hour,
		ScanTimeout:      35 * time.Millisecond,
		Now:              func() time.Time { return now },
		KnownPeerProvider: knownPeerProvider(map[string][]byte{
			"self-device": selfPub,
			"peer-1":      peerOnePub,
			"peer-2":      peerTwoPub,
		}),
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			call := atomic.AddInt32(&browseCalls, 1)
			entries <- testServiceEntry("self-device", "Self", 9999, "10.0.0.1", selfPub, now)
			entries <- testServiceEntry("peer-1", "Bob", 9998, "10.0.0.2", peerOnePub, now)
			if call >= 2 {
				entries <- testServiceEntry("peer-2", "Carol", 9997, "10.0.0.3", peerTwoPub, now)
			}
			<-ctx.Done()
			return nil
		},
	}

	scanner, err := NewPeerScanner(cfg)
	if err != nil {
		t.Fatalf("NewPeerScanner failed: %v", err)
	}
	if err := scanner.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scanner.Stop()

	waitForCondition(t, time.Second, func() bool {
		peers := scanner.ListPeers()
		return len(peers) == 1 && peers[0].DeviceID == "peer-1"
	})

	if err := scanner.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		peers := scanner.ListPeers()
		return len(peers) == 2
	})
}

func TestPeerScannerBackgroundPollingAndRemovalEvent(t *testing.T) {
	var browseCalls int32
	now := time.Unix(1_706_000_000, 0).UTC()
	selfPub := deterministicDiscoveryPublicKey("self-device")
	peerOnePub := deterministicDiscoveryPublicKey("peer-1")
	peerTwoPub := deterministicDiscoveryPublicKey("peer-2")

	cfg := Config{
		SelfDeviceID:     "self-device",
		Ed25519PublicKey: selfPub,
		RefreshInterval:  40 * time.Millisecond,
		ScanTimeout:      25 * time.Millisecond,
		PeerStaleAfter:   80 * time.Millisecond,
		Now:              func() time.Time { return now },
		KnownPeerProvider: knownPeerProvider(map[string][]byte{
			"self-device": selfPub,
			"peer-1":      peerOnePub,
			"peer-2":      peerTwoPub,
		}),
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			call := atomic.AddInt32(&browseCalls, 1)
			if call == 1 {
				entries <- testServiceEntry("peer-1", "Bob", 9998, "10.0.0.2", peerOnePub, now)
				entries <- testServiceEntry("peer-2", "Carol", 9997, "10.0.0.3", peerTwoPub, now)
			} else {
				entries <- testServiceEntry("peer-2", "Carol", 9997, "10.0.0.3", peerTwoPub, now)
			}
			<-ctx.Done()
			return nil
		},
	}

	scanner, err := NewPeerScanner(cfg)
	if err != nil {
		t.Fatalf("NewPeerScanner failed: %v", err)
	}
	if err := scanner.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scanner.Stop()

	waitForCondition(t, 2*time.Second, func() bool {
		peers := scanner.ListPeers()
		return len(peers) == 1 && peers[0].DeviceID == "peer-2"
	})

	if !waitForEvent(scanner.Events(), EventPeerRemoved, "peer-1", 2*time.Second) {
		t.Fatalf("expected peer removal event for peer-1")
	}
}

func TestPeerScannerRefreshIgnoresDeadlineExceededFromBrowse(t *testing.T) {
	now := time.Unix(1_706_000_000, 0).UTC()
	selfPub := deterministicDiscoveryPublicKey("self-device")
	peerOnePub := deterministicDiscoveryPublicKey("peer-1")

	cfg := Config{
		SelfDeviceID:     "self-device",
		Ed25519PublicKey: selfPub,
		RefreshInterval:  time.Hour,
		ScanTimeout:      35 * time.Millisecond,
		Now:              func() time.Time { return now },
		KnownPeerProvider: knownPeerProvider(map[string][]byte{
			"self-device": selfPub,
			"peer-1":      peerOnePub,
		}),
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			entries <- testServiceEntry("peer-1", "Bob", 9998, "10.0.0.2", peerOnePub, now)
			<-ctx.Done()
			return ctx.Err()
		},
	}

	scanner, err := NewPeerScanner(cfg)
	if err != nil {
		t.Fatalf("NewPeerScanner failed: %v", err)
	}
	if err := scanner.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scanner.Stop()

	if err := scanner.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	waitForCondition(t, time.Second, func() bool {
		peers := scanner.ListPeers()
		return len(peers) == 1 && peers[0].DeviceID == "peer-1"
	})
}

func TestPeerScannerUnknownPeerStillDiscovered(t *testing.T) {
	now := time.Unix(1_706_000_000, 0).UTC()
	selfPub := deterministicDiscoveryPublicKey("self-device")
	unknownPub := deterministicDiscoveryPublicKey("unknown-device")

	cfg := Config{
		SelfDeviceID:     "self-device",
		Ed25519PublicKey: selfPub,
		RefreshInterval:  time.Hour,
		ScanTimeout:      35 * time.Millisecond,
		Now:              func() time.Time { return now },
		KnownPeerProvider: knownPeerProvider(map[string][]byte{
			"self-device": selfPub,
		}),
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			entries <- testServiceEntry("unknown-device", "Unknown", 9996, "10.0.0.9", unknownPub, now)
			<-ctx.Done()
			return nil
		},
	}

	scanner, err := NewPeerScanner(cfg)
	if err != nil {
		t.Fatalf("NewPeerScanner failed: %v", err)
	}
	if err := scanner.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer scanner.Stop()

	waitForCondition(t, time.Second, func() bool {
		peers := scanner.ListPeers()
		return len(peers) == 1 && len(peers[0].Addresses) == 1 && peers[0].DeviceID != "" && peers[0].DeviceID != "unknown-device"
	})
}

func deterministicDiscoveryPublicKey(deviceID string) ed25519.PublicKey {
	seed := sha256.Sum256([]byte("discovery-seed|" + deviceID))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil
	}
	return publicKey
}

func knownPeerProvider(peers map[string][]byte) func() []KnownPeerIdentity {
	return func() []KnownPeerIdentity {
		out := make([]KnownPeerIdentity, 0, len(peers))
		for deviceID, publicKey := range peers {
			out = append(out, KnownPeerIdentity{
				DeviceID:         deviceID,
				Ed25519PublicKey: base64.StdEncoding.EncodeToString(publicKey),
			})
		}
		return out
	}
}

func testServiceEntry(deviceID, instance string, port int, ip string, ed25519PublicKey []byte, at time.Time) *zeroconf.ServiceEntry {
	token := ComputeDiscoveryToken(deviceID, ed25519PublicKey, at)
	return &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: instance,
			Service:  DefaultService,
			Domain:   DefaultDomain,
		},
		HostName: instance + ".local",
		Port:     port,
		Text: []string{
			discoveryTokenTXTKey + "=" + token,
			"version=1",
		},
		AddrIPv4: []net.IP{net.ParseIP(ip)},
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout %s", timeout)
}

func waitForEvent(events <-chan Event, eventType EventType, deviceID string, timeout time.Duration) bool {
	deadline := time.After(timeout)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return false
			}
			if event.Type == eventType && event.Peer.DeviceID == deviceID {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
