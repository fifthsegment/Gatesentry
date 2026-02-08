package gatesentryDnsServer

import (
	"net"
	"testing"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	"github.com/miekg/dns"
)

// --- Mock dns.ResponseWriter ---

type mockResponseWriter struct {
	msg        *dns.Msg
	localAddr  net.Addr
	remoteAddr net.Addr
	closed     bool
}

func newMockResponseWriter(clientIP string) *mockResponseWriter {
	return &mockResponseWriter{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
		remoteAddr: &net.UDPAddr{IP: net.ParseIP(clientIP), Port: 12345},
	}
}

func (m *mockResponseWriter) LocalAddr() net.Addr  { return m.localAddr }
func (m *mockResponseWriter) RemoteAddr() net.Addr { return m.remoteAddr }
func (m *mockResponseWriter) WriteMsg(msg *dns.Msg) error {
	m.msg = msg
	return nil
}
func (m *mockResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (m *mockResponseWriter) Close() error {
	m.closed = true
	return nil
}
func (m *mockResponseWriter) TsigStatus() error   { return nil }
func (m *mockResponseWriter) TsigTimersOnly(bool) {}
func (m *mockResponseWriter) Hijack()             {}

// --- Test helper to set up server state ---

func setupTestServer(t *testing.T) func() {
	t.Helper()

	// Save original state
	origDeviceStore := deviceStore
	origLogger := logger
	origBlocked := blockedDomains
	origException := exceptionDomains
	origInternal := internalRecords
	origRunning := serverRunning.Load()
	origDDNSEnabled := ddnsEnabled
	origDDNSTSIGRequired := ddnsTSIGRequired

	// Initialize test state
	deviceStore = discovery.NewDeviceStore("local")
	blockedDomains = make(map[string]bool)
	exceptionDomains = make(map[string]bool)
	internalRecords = make(map[string]string)
	serverRunning.Store(true)
	ddnsEnabled = true
	ddnsTSIGRequired = false

	// Create a temp logger
	logger = gatesentryLogger.NewLogger(t.TempDir() + "/test.db")

	// Return cleanup function
	return func() {
		deviceStore = origDeviceStore
		logger = origLogger
		blockedDomains = origBlocked
		exceptionDomains = origException
		internalRecords = origInternal
		serverRunning.Store(origRunning)
		ddnsEnabled = origDDNSEnabled
		ddnsTSIGRequired = origDDNSTSIGRequired
	}
}

// --- isReverseDomain tests ---

func TestIsReverseDomain_IPv4(t *testing.T) {
	if !isReverseDomain("100.1.168.192.in-addr.arpa") {
		t.Error("Expected in-addr.arpa to be reverse domain")
	}
}

func TestIsReverseDomain_IPv6(t *testing.T) {
	if !isReverseDomain("a.4.2.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.7.6.5.4.3.2.1.ip6.arpa") {
		t.Error("Expected ip6.arpa to be reverse domain")
	}
}

func TestIsReverseDomain_Forward(t *testing.T) {
	if isReverseDomain("macmini.local") {
		t.Error("Expected macmini.local to NOT be reverse domain")
	}
	if isReverseDomain("google.com") {
		t.Error("Expected google.com to NOT be reverse domain")
	}
}

// --- handleDNSRequest integration tests ---

func TestHandleDNS_DeviceStoreA(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	// Add a device to the store
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    discovery.SourceManual,
		Sources:   []discovery.DiscoverySource{discovery.SourceManual},
	})

	// Create DNS query for A record
	req := new(dns.Msg)
	req.SetQuestion("macmini.local.", dns.TypeA)

	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}
	a, ok := w.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", w.msg.Answer[0])
	}
	if a.A.String() != "192.168.1.100" {
		t.Errorf("Expected A record 192.168.1.100, got %s", a.A.String())
	}
}

func TestHandleDNS_DeviceStoreAAAA(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"server"},
		IPv6:      "fd00::1234",
		Source:    discovery.SourceManual,
		Sources:   []discovery.DiscoverySource{discovery.SourceManual},
	})

	req := new(dns.Msg)
	req.SetQuestion("server.local.", dns.TypeAAAA)

	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}
	aaaa, ok := w.msg.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("Expected AAAA record, got %T", w.msg.Answer[0])
	}
	if aaaa.AAAA.String() != "fd00::1234" {
		t.Errorf("Expected AAAA record fd00::1234, got %s", aaaa.AAAA.String())
	}
}

func TestHandleDNS_DeviceStorePTR(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    discovery.SourceManual,
		Sources:   []discovery.DiscoverySource{discovery.SourceManual},
	})

	// PTR query for reverse lookup
	req := new(dns.Msg)
	req.SetQuestion("100.1.168.192.in-addr.arpa.", dns.TypePTR)

	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}
	ptr, ok := w.msg.Answer[0].(*dns.PTR)
	if !ok {
		t.Fatalf("Expected PTR record, got %T", w.msg.Answer[0])
	}
	if ptr.Ptr != "macmini.local." {
		t.Errorf("Expected PTR macmini.local., got %s", ptr.Ptr)
	}
}

func TestHandleDNS_DeviceStoreNoMatchFallsThrough(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	// Device store has a device, but we query for a different name
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    discovery.SourceManual,
	})

	// Query for a name NOT in device store but IS in legacy internal records
	internalRecords["oldserver.local"] = "10.0.0.5"

	req := new(dns.Msg)
	req.SetQuestion("oldserver.local.", dns.TypeA)

	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}
	a, ok := w.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", w.msg.Answer[0])
	}
	if a.A.String() != "10.0.0.5" {
		t.Errorf("Expected legacy A record 10.0.0.5, got %s", a.A.String())
	}
}

func TestHandleDNS_BlockedDomain(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	blockedDomains["malware.example.com"] = true

	req := new(dns.Msg)
	req.SetQuestion("malware.example.com.", dns.TypeA)

	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if w.msg.Rcode != dns.RcodeNameError {
		t.Errorf("Expected NXDOMAIN, got %d", w.msg.Rcode)
	}
}

func TestHandleDNS_DeviceStorePriority(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	// Same name in both device store and legacy internal records
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"myserver"},
		IPv4:      "192.168.1.200",
		Source:    discovery.SourceDDNS,
		Sources:   []discovery.DiscoverySource{discovery.SourceDDNS},
	})
	internalRecords["myserver.local"] = "10.0.0.99"

	req := new(dns.Msg)
	req.SetQuestion("myserver.local.", dns.TypeA)

	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}
	a, ok := w.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", w.msg.Answer[0])
	}
	// Device store should take priority over legacy internal records
	if a.A.String() != "192.168.1.200" {
		t.Errorf("Expected device store IP 192.168.1.200, got %s (device store should take priority)", a.A.String())
	}
}

func TestHandleDNS_ServerNotRunning(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	serverRunning.Store(false)

	req := new(dns.Msg)
	req.SetQuestion("macmini.local.", dns.TypeA)

	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	// Server not running should close the connection, not respond
	if w.closed != true {
		t.Error("Expected connection to be closed when server not running")
	}
	if w.msg != nil {
		t.Error("Expected no response when server not running")
	}
}

func TestHandleDNS_DualStack(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	// Device with both IPv4 and IPv6
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"dualstack"},
		IPv4:      "192.168.1.42",
		IPv6:      "fd00::42",
		Source:    discovery.SourceLease,
		Sources:   []discovery.DiscoverySource{discovery.SourceLease},
	})

	// Query A → should get IPv4 only
	req := new(dns.Msg)
	req.SetQuestion("dualstack.local.", dns.TypeA)
	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil || len(w.msg.Answer) != 1 {
		t.Fatal("Expected 1 A record answer")
	}
	if _, ok := w.msg.Answer[0].(*dns.A); !ok {
		t.Error("Expected A record for TypeA query on dual-stack device")
	}

	// Query AAAA → should get IPv6 only
	req2 := new(dns.Msg)
	req2.SetQuestion("dualstack.local.", dns.TypeAAAA)
	w2 := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w2, req2)

	if w2.msg == nil || len(w2.msg.Answer) != 1 {
		t.Fatal("Expected 1 AAAA record answer")
	}
	if _, ok := w2.msg.Answer[0].(*dns.AAAA); !ok {
		t.Error("Expected AAAA record for TypeAAAA query on dual-stack device")
	}
}

func TestHandleDNS_BareHostname(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"printer"},
		IPv4:      "192.168.1.55",
		Source:    discovery.SourceMDNS,
		Sources:   []discovery.DiscoverySource{discovery.SourceMDNS},
	})

	// Query bare hostname without zone suffix
	req := new(dns.Msg)
	req.SetQuestion("printer.", dns.TypeA)
	w := newMockResponseWriter("192.168.1.50")
	handleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("Expected response message")
	}
	// Bare hostname lookup should match via device store's bare-key index
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer for bare hostname, got %d", len(w.msg.Answer))
	}
	a, ok := w.msg.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("Expected A record, got %T", w.msg.Answer[0])
	}
	if a.A.String() != "192.168.1.55" {
		t.Errorf("Expected 192.168.1.55, got %s", a.A.String())
	}
}
