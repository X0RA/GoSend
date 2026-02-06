package discovery

import (
	"context"
	"net"
	"testing"

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
		SelfDeviceID:   "device-123",
		DeviceName:     "Alice Laptop",
		ListeningPort:  9999,
		KeyFingerprint: "AABBCCDDEEFF0011",
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

	assertContainsTXT(t, gotTXT, "device_id=device-123")
	assertContainsTXT(t, gotTXT, "version=1")
	assertContainsTXT(t, gotTXT, "key_fingerprint=AABBCCDDEEFF0011")
}

func TestServiceStartAndStop(t *testing.T) {
	cfg := Config{
		SelfDeviceID:   "self",
		DeviceName:     "Self",
		ListeningPort:  9999,
		KeyFingerprint: "ff00",
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

func assertContainsTXT(t *testing.T, txt []string, expected string) {
	t.Helper()
	for _, v := range txt {
		if v == expected {
			return
		}
	}
	t.Fatalf("missing TXT record %q in %v", expected, txt)
}
