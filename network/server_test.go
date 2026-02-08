package network

import (
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestServerConnectionRateLimitPerIP(t *testing.T) {
	identity := testIdentity(t, "server-rate-limit", "Server Rate Limit")
	var limitedCount atomic.Int32

	server, err := Listen("127.0.0.1:0", HandshakeOptions{
		Identity:                  identity,
		ConnectionRateLimitPerIP:  2,
		ConnectionRateLimitWindow: 250 * time.Millisecond,
		OnInboundConnectionRateLimit: func(string) {
			limitedCount.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() {
		_ = server.Close()
	}()

	openConns := make([]net.Conn, 0, 4)
	defer func() {
		for _, conn := range openConns {
			_ = conn.Close()
		}
	}()

	for i := 0; i < 2; i++ {
		conn, ok := dialAndExpectHandshakeChallenge(t, server.Addr().String())
		if !ok {
			t.Fatalf("expected allowed connection %d to receive handshake challenge", i+1)
		}
		openConns = append(openConns, conn)
	}

	limitedConn, ok := dialAndExpectHandshakeChallenge(t, server.Addr().String())
	if ok {
		_ = limitedConn.Close()
		t.Fatalf("expected third connection in window to be rate-limited")
	}
	_ = limitedConn.Close()

	if limitedCount.Load() == 0 {
		t.Fatalf("expected connection-rate-limit callback to run at least once")
	}

	time.Sleep(300 * time.Millisecond)

	conn, ok := dialAndExpectHandshakeChallenge(t, server.Addr().String())
	if !ok {
		t.Fatalf("expected connection after window reset to be accepted")
	}
	openConns = append(openConns, conn)
}

func dialAndExpectHandshakeChallenge(t *testing.T, address string) (net.Conn, bool) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	if err := conn.SetDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("set deadline failed: %v", err)
	}

	payload, err := ReadFrameWithTimeout(conn, 500*time.Millisecond)
	if err != nil {
		return conn, false
	}
	var challenge HandshakeChallenge
	if err := json.Unmarshal(payload, &challenge); err != nil {
		return conn, false
	}
	return conn, challenge.Type == TypeHandshakeChallenge && challenge.Nonce != ""
}
