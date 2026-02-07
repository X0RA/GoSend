package discovery

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestPeerScannerFiltersSelfAndManualRefresh(t *testing.T) {
	var browseCalls int32
	cfg := Config{
		SelfDeviceID:    "self-device",
		RefreshInterval: time.Hour,
		ScanTimeout:     35 * time.Millisecond,
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			call := atomic.AddInt32(&browseCalls, 1)
			entries <- testServiceEntry("self-device", "Self", 9999, "10.0.0.1")
			entries <- testServiceEntry("peer-1", "Bob", 9998, "10.0.0.2")
			if call >= 2 {
				entries <- testServiceEntry("peer-2", "Carol", 9997, "10.0.0.3")
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
	cfg := Config{
		SelfDeviceID:    "self-device",
		RefreshInterval: 40 * time.Millisecond,
		ScanTimeout:     25 * time.Millisecond,
		PeerStaleAfter:  80 * time.Millisecond,
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			call := atomic.AddInt32(&browseCalls, 1)
			if call == 1 {
				entries <- testServiceEntry("peer-1", "Bob", 9998, "10.0.0.2")
				entries <- testServiceEntry("peer-2", "Carol", 9997, "10.0.0.3")
			} else {
				entries <- testServiceEntry("peer-2", "Carol", 9997, "10.0.0.3")
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
	cfg := Config{
		SelfDeviceID:    "self-device",
		RefreshInterval: time.Hour,
		ScanTimeout:     35 * time.Millisecond,
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			entries <- testServiceEntry("peer-1", "Bob", 9998, "10.0.0.2")
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

func testServiceEntry(deviceID, instance string, port int, ip string) *zeroconf.ServiceEntry {
	return &zeroconf.ServiceEntry{
		ServiceRecord: zeroconf.ServiceRecord{
			Instance: instance,
			Service:  DefaultService,
			Domain:   DefaultDomain,
		},
		HostName: instance + ".local",
		Port:     port,
		Text: []string{
			"device_id=" + deviceID,
			"version=1",
			"key_fingerprint=fingerprint-" + deviceID,
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
