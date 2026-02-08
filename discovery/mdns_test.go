package discovery

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"net"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestStartBroadcasterBuildsExpectedTXTRecords(t *testing.T) {
	var (
		gotInstance string
		gotService  string
		gotDomain   string
		gotPort     int
		gotTXT      []string
	)

	cfg := Config{
		SelfDeviceID:     "device-123",
		DeviceName:       "Alice Laptop",
		ListeningPort:    9999,
		Ed25519PublicKey: deterministicTestPublicKey("device-123"),
		Now:              func() time.Time { return time.Unix(1_706_000_000, 0).UTC() },
		registerFn: func(instance, service, domain string, port int, text []string, ifaces []net.Interface) (*zeroconf.Server, error) {
			gotInstance = instance
			gotService = service
			gotDomain = domain
			gotPort = port
			gotTXT = append([]string(nil), text...)
			return nil, nil
		},
	}

	broadcaster, err := StartBroadcaster(cfg)
	if err != nil {
		t.Fatalf("StartBroadcaster failed: %v", err)
	}
	if broadcaster == nil {
		t.Fatalf("expected broadcaster instance")
	}

	if gotInstance != "Alice Laptop" {
		t.Fatalf("unexpected instance name: %q", gotInstance)
	}
	if gotService != DefaultService {
		t.Fatalf("unexpected service: %q", gotService)
	}
	if gotDomain != DefaultDomain {
		t.Fatalf("unexpected domain: %q", gotDomain)
	}
	if gotPort != 9999 {
		t.Fatalf("unexpected port: %d", gotPort)
	}

	assertContainsTXTPrefix(t, gotTXT, discoveryTokenTXTKey+"=")
	assertContainsTXT(t, gotTXT, "version=1")
	assertNotContainsTXTPrefix(t, gotTXT, "device_id=")
	assertNotContainsTXTPrefix(t, gotTXT, "key_fingerprint=")
}

func TestServiceStartAndStop(t *testing.T) {
	cfg := Config{
		SelfDeviceID:     "self",
		DeviceName:       "Self",
		ListeningPort:    9999,
		Ed25519PublicKey: deterministicTestPublicKey("self"),
		registerFn: func(instance, service, domain string, port int, text []string, ifaces []net.Interface) (*zeroconf.Server, error) {
			return nil, nil
		},
		browseFn: func(ctx context.Context, service, domain string, entries chan<- *zeroconf.ServiceEntry) error {
			<-ctx.Done()
			return nil
		},
	}

	svc, err := Start(cfg)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if svc.Broadcaster == nil || svc.Scanner == nil {
		t.Fatalf("expected broadcaster and scanner")
	}
	svc.Stop()
}

func TestConfigWithDefaultsSetsPeerStaleAfterFromTTL(t *testing.T) {
	cfg := Config{
		RefreshInterval: 10 * time.Second,
	}

	withDefaults := cfg.withDefaults()
	if withDefaults.TTL != DefaultTTL {
		t.Fatalf("expected default TTL %d, got %d", DefaultTTL, withDefaults.TTL)
	}
	if withDefaults.PeerStaleAfter < 2*time.Duration(DefaultTTL)*time.Second {
		t.Fatalf("expected peer stale timeout to be >= 2*TTL, got %s", withDefaults.PeerStaleAfter)
	}
}

func TestComputeDiscoveryTokenRotatesAcrossHourBoundary(t *testing.T) {
	publicKey := deterministicTestPublicKey("token-device")
	deviceID := "token-device"

	base := time.Unix(1_706_000_000, 0).UTC().Truncate(time.Hour)
	sameHour := base.Add(30 * time.Minute)
	nextHour := base.Add(90 * time.Minute)

	first := ComputeDiscoveryToken(deviceID, publicKey, base)
	second := ComputeDiscoveryToken(deviceID, publicKey, sameHour)
	third := ComputeDiscoveryToken(deviceID, publicKey, nextHour)

	if first == "" || second == "" || third == "" {
		t.Fatalf("expected non-empty discovery tokens")
	}
	if first != second {
		t.Fatalf("expected token to remain stable inside one hour bucket")
	}
	if first == third {
		t.Fatalf("expected token to rotate across hour boundary")
	}
	if !VerifyDiscoveryToken(first, deviceID, publicKey, base) {
		t.Fatalf("expected token verification to succeed for matching hour")
	}
	if VerifyDiscoveryToken(first, deviceID, publicKey, nextHour) {
		t.Fatalf("expected token verification to fail for different hour")
	}
}

func assertContainsTXT(t *testing.T, txt []string, expected string) {
	t.Helper()
	for _, v := range txt {
		if v == expected {
			return
		}
	}
	t.Fatalf("missing TXT record %q in %v", expected, txt)
}

func assertContainsTXTPrefix(t *testing.T, txt []string, prefix string) {
	t.Helper()
	for _, value := range txt {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return
		}
	}
	t.Fatalf("missing TXT prefix %q in %v", prefix, txt)
}

func assertNotContainsTXTPrefix(t *testing.T, txt []string, prefix string) {
	t.Helper()
	for _, value := range txt {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			t.Fatalf("unexpected TXT prefix %q found in %v", prefix, txt)
		}
	}
}

func deterministicTestPublicKey(seed string) ed25519.PublicKey {
	sum := sha256.Sum256([]byte("mdns-test-seed|" + seed))
	privateKey := ed25519.NewKeyFromSeed(sum[:])
	publicKey, _ := privateKey.Public().(ed25519.PublicKey)
	return publicKey
}
