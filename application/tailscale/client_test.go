package tailscale

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientStatusOverUnixSocket(t *testing.T) {
	var calls atomic.Int32
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", request.Method)
		}
		if request.URL.Path != StatusPath {
			t.Errorf("path = %q, want %q", request.URL.Path, StatusPath)
		}
		if request.Host != "local-tailscaled.sock" {
			t.Errorf("Host = %q, want local-tailscaled.sock", request.Host)
		}
		response.Header().Set("Content-Type", "application/json")
		fmt.Fprint(response, `{
			"BackendState":"Running",
			"Self":{"ID":"self-id","HostName":"gateway","DNSName":"gateway.example.ts.net.","TailscaleIPs":["100.64.0.1"]},
			"Peer":{
				"nodekey:not-the-id":{"ID":"node-b","HostName":"phone","DNSName":"phone.example.ts.net.","TailscaleIPs":["100.64.0.3","fd7a:115c:a1e0::3"],"Online":true,"LastSeen":"2026-09-24T10:00:00Z","WoLMACs":["aa:bb:cc:dd:ee:ff"]},
				"another-map-key":{"ID":"node-a","HostName":"laptop","TailscaleIPs":["100.64.0.2"]},
				"missing-id":{"HostName":"ignored"},
				"nil":null
			}
		}`)
	})
	defer closeServer()

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	if status.BackendState != "Running" {
		t.Errorf("BackendState = %q, want Running", status.BackendState)
	}
	if status.Self == nil || status.Self.NodeID != "self-id" || status.Self.Name != "gateway" {
		t.Fatalf("Self = %#v, want self-id gateway", status.Self)
	}
	if status.Self.DNSName != "gateway.example.ts.net" {
		t.Errorf("Self.DNSName = %q, want trailing dot removed", status.Self.DNSName)
	}
	if len(status.Peers) != 2 || status.Peers[0].NodeID != "node-a" || status.Peers[1].NodeID != "node-b" {
		t.Fatalf("Peers = %#v, want stable IDs node-a and node-b", status.Peers)
	}
	peer := status.Peers[1]
	if peer.Name != "phone" || peer.DNSName != "phone.example.ts.net" || !peer.Online {
		t.Errorf("peer = %#v, want decoded phone", peer)
	}
	if len(peer.Addresses) != 2 || len(peer.WoLMACs) != 1 {
		t.Errorf("peer aliases = %#v, want addresses and informational WoL MAC", peer)
	}
}

func TestClientDetectUsesEnvironmentSocket(t *testing.T) {
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{}`)
	})
	defer closeServer()
	t.Setenv(SocketEnv, client.socketPath)

	discovered := NewClient()
	path, ok := discovered.DetectSocket()
	if !ok || path != client.socketPath {
		t.Fatalf("DetectSocket() = %q, %v; want env socket %q", path, ok, client.socketPath)
	}
}

func TestClientDetectRejectsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tailscaled.sock")
	if err := os.WriteFile(path, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := NewClientWithSocket(path)
	if client.Detect() {
		t.Fatal("Detect() = true for regular file")
	}
	if _, err := client.Status(context.Background()); err == nil {
		t.Fatal("Status() error = nil for regular file")
	}
}

func TestClientStatusRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "non-OK status",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(http.StatusForbidden)
				fmt.Fprint(response, "secret detail must not be reflected")
			},
		},
		{
			name: "invalid JSON",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(response, `{not-json`)
			},
		},
		{
			name: "oversized body",
			handler: func(response http.ResponseWriter, _ *http.Request) {
				fmt.Fprint(response, strings.Repeat("x", maxStatusResponseSize+1))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, closeServer := unixTestServer(t, test.handler)
			defer closeServer()
			_, err := client.Status(context.Background())
			if err == nil {
				t.Fatal("Status() error = nil")
			}
			if strings.Contains(err.Error(), "secret detail") {
				t.Fatalf("error leaked response body: %v", err)
			}
		})
	}
}

func TestClientStatusHonorsRequestTimeout(t *testing.T) {
	client, closeServer := unixTestServer(t, func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	})
	defer closeServer()
	client.timeout = 20 * time.Millisecond
	started := time.Now()
	if _, err := client.Status(context.Background()); err == nil {
		t.Fatal("Status() error = nil after request timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Status() took %v, want bounded timeout", elapsed)
	}
}

func TestClientStatusHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	})
	defer closeServer()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.Status(ctx)
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Status() error = nil after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("Status() did not return after cancellation")
	}
}

func TestClientStatusRejectsNilContext(t *testing.T) {
	client := NewClientWithSocket(filepath.Join(t.TempDir(), "unused.sock"))
	if _, err := client.Status(nil); err == nil {
		t.Fatal("Status(nil) error = nil")
	}
}

func unixTestServer(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tailscaled.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}
	server := &http.Server{Handler: handler}
	stopped := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(stopped)
	}()
	closeServer := func() {
		_ = server.Close()
		_ = listener.Close()
		<-stopped
	}
	return NewClientWithSocket(path), closeServer
}

func TestClientEnvironmentSocketFallsBackToStandardCandidates(t *testing.T) {
	oldPaths := defaultSocketPaths
	t.Cleanup(func() { defaultSocketPaths = oldPaths })
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{}`)
	})
	defer closeServer()
	defaultSocketPaths = []string{client.socketPath}
	t.Setenv(SocketEnv, filepath.Join(t.TempDir(), "missing.sock"))

	path, ok := NewClient().DetectSocket()
	if !ok || path != client.socketPath {
		t.Fatalf("DetectSocket() = %q, %v; want standard fallback %q", path, ok, client.socketPath)
	}
}

func TestClientStatusRejectsDuplicateStableIDs(t *testing.T) {
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key-a":{"ID":"node-1","HostName":"first"},"key-b":{"ID":"node-1","HostName":"second"}}}`)
	})
	defer closeServer()
	if _, err := client.Status(context.Background()); err == nil {
		t.Fatal("Status() error = nil for duplicate PeerStatus.ID")
	}
}
