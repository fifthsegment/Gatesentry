//go:build linux

package gatesentryproxy

import (
	"net"
	"testing"
	"time"
)

// TestTransparentConnectionObservesClientIP verifies that a connection
// arriving at the transparent listener is reported to the device
// observation handler with the real client IP. Routed traffic (exit nodes,
// routers) is the only evidence GateSentry may ever get that such an
// address is active, so observation must happen before any protocol
// handling that may fail early.
func TestTransparentConnectionObservesClientIP(t *testing.T) {
	saved := IProxy
	defer func() { IProxy = saved }()

	observed := make(chan string, 1)
	IProxy = &GSProxy{
		DeviceObservationHandler: func(clientIP string) { observed <- clientIP },
	}

	l := &TransparentProxyListener{ProxyHandler: &ProxyHandler{}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	server, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	// Close the client end so handleConnection fails its initial read and
	// exits early; observation must already have fired.
	client.Close()
	go l.handleConnection(server)

	select {
	case ip := <-observed:
		if ip != "127.0.0.1" {
			t.Fatalf("observed client IP = %q, want 127.0.0.1", ip)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DeviceObservationHandler was not called")
	}
}

// TestTransparentConnectionSkipsObservationWithoutHandler verifies the
// listener stays quiet when no handler is wired (e.g. tests or embedded
// builds), instead of panicking.
func TestTransparentConnectionSkipsObservationWithoutHandler(t *testing.T) {
	saved := IProxy
	defer func() { IProxy = saved }()

	IProxy = &GSProxy{}

	l := &TransparentProxyListener{ProxyHandler: &ProxyHandler{}}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	server, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	client.Close()
	done := make(chan struct{})
	go func() { l.handleConnection(server); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConnection blocked without a handler wired")
	}
}
