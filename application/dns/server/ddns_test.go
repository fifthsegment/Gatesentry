package gatesentryDnsServer

import (
	"fmt"
	"net"
	"testing"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	"github.com/miekg/dns"
)

// --- DDNS-specific mock ResponseWriter ---

// ddnsMockWriter extends mockResponseWriter with a configurable TSIG status.
// This allows testing TSIG validation without going through the wire layer.
type ddnsMockWriter struct {
	msg        *dns.Msg
	tsigErr    error // nil = valid TSIG, non-nil = TSIG verification failed
	localAddr  net.Addr
	remoteAddr net.Addr
}

func newDDNSMockWriter() *ddnsMockWriter {
	return &ddnsMockWriter{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53},
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345},
	}
}

func (m *ddnsMockWriter) LocalAddr() net.Addr         { return m.localAddr }
func (m *ddnsMockWriter) RemoteAddr() net.Addr        { return m.remoteAddr }
func (m *ddnsMockWriter) WriteMsg(msg *dns.Msg) error { m.msg = msg; return nil }
func (m *ddnsMockWriter) Write(b []byte) (int, error) { return len(b), nil }
func (m *ddnsMockWriter) Close() error                { return nil }
func (m *ddnsMockWriter) TsigStatus() error           { return m.tsigErr }
func (m *ddnsMockWriter) TsigTimersOnly(bool)         {}
func (m *ddnsMockWriter) Hijack()                     {}

// --- Test helpers ---

// setupDDNSTestServer initializes test state for DDNS tests.
// Returns a cleanup function that restores all globals.
func setupDDNSTestServer(t *testing.T) func() {
	t.Helper()

	origDeviceStore := deviceStore
	origLogger := logger
	origBlocked := blockedDomains
	origException := exceptionDomains
	origInternal := internalRecords
	origRunning := serverRunning.Load()
	origDDNSEnabled := ddnsEnabled
	origDDNSTSIGRequired := ddnsTSIGRequired

	deviceStore = discovery.NewDeviceStore("local")
	blockedDomains = make(map[string]bool)
	exceptionDomains = make(map[string]bool)
	internalRecords = make(map[string]string)
	serverRunning.Store(true)
	ddnsEnabled = true
	ddnsTSIGRequired = false
	logger = gatesentryLogger.NewLogger(t.TempDir() + "/test.db")

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

// makeUpdateMsg creates a DNS UPDATE message for the given zone.
func makeUpdateMsg(zone string) *dns.Msg {
	m := new(dns.Msg)
	m.SetUpdate(zone + ".")
	return m
}

// addUpdateRR adds a resource record string to the UPDATE section (msg.Ns).
func addUpdateRR(m *dns.Msg, rrStr string) {
	rr, err := dns.NewRR(rrStr)
	if err != nil {
		panic(fmt.Sprintf("bad RR: %s: %v", rrStr, err))
	}
	m.Ns = append(m.Ns, rr)
}

// ==========================================================================
// Unit tests — extractHostname
// ==========================================================================

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		fqdn, zone, expected string
	}{
		{"macmini.local", "local", "macmini"},
		{"printer.jvj28.com", "jvj28.com", "printer"},
		{"sub.host.local", "local", "sub.host"},
		{"local", "local", ""},                // zone itself is not a hostname
		{"macmini.other", "local", ""},        // wrong zone
		{"MACMINI.LOCAL", "local", "macmini"}, // case-insensitive
		{"", "local", ""},
		{"host.local", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.fqdn+"_"+tt.zone, func(t *testing.T) {
			got := extractHostname(tt.fqdn, tt.zone)
			if got != tt.expected {
				t.Errorf("extractHostname(%q, %q) = %q, want %q",
					tt.fqdn, tt.zone, got, tt.expected)
			}
		})
	}
}

// ==========================================================================
// Unit tests — isAuthorizedZone
// ==========================================================================

func TestIsAuthorizedZone(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	if !isAuthorizedZone("local") {
		t.Error("Expected 'local' to be authorized")
	}
	if !isAuthorizedZone("LOCAL") {
		t.Error("Expected case-insensitive match for 'LOCAL'")
	}
	if isAuthorizedZone("evil.com") {
		t.Error("Expected 'evil.com' to not be authorized")
	}
	if isAuthorizedZone("") {
		t.Error("Expected empty string to not be authorized")
	}
}

func TestIsAuthorizedZone_MultiZone(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	deviceStore = discovery.NewDeviceStoreMultiZone("jvj28.com", "local")

	if !isAuthorizedZone("jvj28.com") {
		t.Error("Expected 'jvj28.com' to be authorized")
	}
	if !isAuthorizedZone("local") {
		t.Error("Expected 'local' to be authorized")
	}
	if isAuthorizedZone("other.com") {
		t.Error("Expected 'other.com' to not be authorized")
	}
}

// ==========================================================================
// Unit tests — parseDDNSUpdates
// ==========================================================================

func TestParseDDNSUpdates_Adds(t *testing.T) {
	var rrs []dns.RR

	aRR, _ := dns.NewRR("macmini.local. 300 IN A 192.168.1.100")
	rrs = append(rrs, aRR)

	aaaaRR, _ := dns.NewRR("macmini.local. 300 IN AAAA fd00::24a")
	rrs = append(rrs, aaaaRR)

	adds, deletes := parseDDNSUpdates(rrs, "local")

	if len(adds) != 2 {
		t.Fatalf("Expected 2 adds, got %d", len(adds))
	}
	if len(deletes) != 0 {
		t.Fatalf("Expected 0 deletes, got %d", len(deletes))
	}

	if adds[0].name != "macmini.local" || adds[0].rrtype != dns.TypeA || adds[0].value != "192.168.1.100" {
		t.Errorf("Unexpected first add: %+v", adds[0])
	}
	if adds[1].name != "macmini.local" || adds[1].rrtype != dns.TypeAAAA || adds[1].value != "fd00::24a" {
		t.Errorf("Unexpected second add: %+v", adds[1])
	}
}

func TestParseDDNSUpdates_DeleteAll(t *testing.T) {
	var rrs []dns.RR

	// Delete all A records for a name (ClassANY, specific type)
	rrs = append(rrs, &dns.ANY{
		Hdr: dns.RR_Header{
			Name:   "macmini.local.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassANY,
			Ttl:    0,
		},
	})

	adds, deletes := parseDDNSUpdates(rrs, "local")

	if len(adds) != 0 {
		t.Fatalf("Expected 0 adds, got %d", len(adds))
	}
	if len(deletes) != 1 {
		t.Fatalf("Expected 1 delete, got %d", len(deletes))
	}
	if deletes[0].class != dns.ClassANY || deletes[0].rrtype != dns.TypeA {
		t.Errorf("Expected ClassANY TypeA delete, got class=%d type=%d",
			deletes[0].class, deletes[0].rrtype)
	}
}

func TestParseDDNSUpdates_DeleteSpecific(t *testing.T) {
	var rrs []dns.RR

	// Delete a specific A record (ClassNONE with value)
	rr, _ := dns.NewRR("macmini.local. 0 IN A 192.168.1.100")
	rr.Header().Class = dns.ClassNONE
	rr.Header().Ttl = 0
	rrs = append(rrs, rr)

	adds, deletes := parseDDNSUpdates(rrs, "local")

	if len(adds) != 0 {
		t.Fatalf("Expected 0 adds, got %d", len(adds))
	}
	if len(deletes) != 1 {
		t.Fatalf("Expected 1 delete, got %d", len(deletes))
	}
	if deletes[0].class != dns.ClassNONE || deletes[0].value != "192.168.1.100" {
		t.Errorf("Expected ClassNONE delete with value 192.168.1.100, got class=%d value=%q",
			deletes[0].class, deletes[0].value)
	}
}

func TestParseDDNSUpdates_Mixed(t *testing.T) {
	var rrs []dns.RR

	// Delete old IP
	delRR, _ := dns.NewRR("macmini.local. 0 IN A 192.168.1.100")
	delRR.Header().Class = dns.ClassNONE
	delRR.Header().Ttl = 0
	rrs = append(rrs, delRR)

	// Add new IP
	addRR, _ := dns.NewRR("macmini.local. 300 IN A 192.168.1.101")
	rrs = append(rrs, addRR)

	adds, deletes := parseDDNSUpdates(rrs, "local")

	if len(adds) != 1 || len(deletes) != 1 {
		t.Fatalf("Expected 1 add + 1 delete, got %d adds + %d deletes",
			len(adds), len(deletes))
	}
	if deletes[0].value != "192.168.1.100" {
		t.Errorf("Expected delete of 192.168.1.100, got %q", deletes[0].value)
	}
	if adds[0].value != "192.168.1.101" {
		t.Errorf("Expected add of 192.168.1.101, got %q", adds[0].value)
	}
}

// ==========================================================================
// Unit tests — ddnsMsgAcceptFunc
// ==========================================================================

func TestDDNSMsgAcceptFunc_Query(t *testing.T) {
	// Standard query (opcode 0) — should be accepted
	hdr := dns.Header{Id: 1, Bits: 0, Qdcount: 1}
	if ddnsMsgAcceptFunc(hdr) != dns.MsgAccept {
		t.Error("Expected standard query to be accepted")
	}
}

func TestDDNSMsgAcceptFunc_Update(t *testing.T) {
	// UPDATE (opcode 5) — should be accepted by our custom function
	hdr := dns.Header{Id: 2, Bits: uint16(dns.OpcodeUpdate) << 11, Qdcount: 1}
	if ddnsMsgAcceptFunc(hdr) != dns.MsgAccept {
		t.Error("Expected UPDATE to be accepted")
	}
}

func TestDDNSMsgAcceptFunc_Notify(t *testing.T) {
	// NOTIFY (opcode 4) — accepted by default
	hdr := dns.Header{Id: 3, Bits: uint16(dns.OpcodeNotify) << 11, Qdcount: 1}
	if ddnsMsgAcceptFunc(hdr) != dns.MsgAccept {
		t.Error("Expected NOTIFY to be accepted")
	}
}

// ==========================================================================
// Integration tests — handleDDNSUpdate
// ==========================================================================

func TestHandleDDNSUpdate_AddA(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")

	handleDDNSUpdate(w, m)

	if w.msg == nil {
		t.Fatal("Expected response")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Verify device was created with A record
	records := deviceStore.LookupName("macmini.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(records))
	}
	if records[0].Value != "192.168.1.100" {
		t.Errorf("Expected 192.168.1.100, got %s", records[0].Value)
	}

	// Verify device exists and has DDNS source
	device := deviceStore.FindDeviceByIP("192.168.1.100")
	if device == nil {
		t.Fatal("Expected device to exist")
	}
	if !device.HasSource(discovery.SourceDDNS) {
		t.Error("Expected DDNS source")
	}
	if device.DNSName != "macmini" {
		t.Errorf("Expected DNSName 'macmini', got %q", device.DNSName)
	}

	// Verify PTR record was generated
	ptrRecords := deviceStore.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(ptrRecords) != 1 {
		t.Fatalf("Expected 1 PTR record, got %d", len(ptrRecords))
	}
}

func TestHandleDDNSUpdate_AddAAAA(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "server.local. 300 IN AAAA fd00::1")

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	records := deviceStore.LookupName("server.local", dns.TypeAAAA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 AAAA record, got %d", len(records))
	}
	if records[0].Value != "fd00::1" {
		t.Errorf("Expected fd00::1, got %s", records[0].Value)
	}

	device := deviceStore.FindDeviceByIP("fd00::1")
	if device == nil {
		t.Fatal("Expected device to exist")
	}
}

func TestHandleDDNSUpdate_AddDualStack(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")
	addUpdateRR(m, "macmini.local. 300 IN AAAA fd00::24a")

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	aRecs := deviceStore.LookupName("macmini.local", dns.TypeA)
	aaaaRecs := deviceStore.LookupName("macmini.local", dns.TypeAAAA)
	if len(aRecs) != 1 || len(aaaaRecs) != 1 {
		t.Fatalf("Expected 1 A + 1 AAAA, got %d A + %d AAAA", len(aRecs), len(aaaaRecs))
	}

	// Should be ONE device, not two (second add merges by hostname)
	if deviceStore.DeviceCount() != 1 {
		t.Errorf("Expected 1 device, got %d", deviceStore.DeviceCount())
	}

	device := deviceStore.FindDeviceByIP("192.168.1.100")
	if device == nil {
		t.Fatal("Expected device")
	}
	if device.IPv4 != "192.168.1.100" {
		t.Errorf("Expected IPv4 192.168.1.100, got %s", device.IPv4)
	}
	if device.IPv6 != "fd00::24a" {
		t.Errorf("Expected IPv6 fd00::24a, got %s", device.IPv6)
	}
}

func TestHandleDDNSUpdate_DeleteByName(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	// First create a device
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"oldhost"},
		IPv4:      "192.168.1.50",
		Source:    discovery.SourceDDNS,
		Sources:   []discovery.DiscoverySource{discovery.SourceDDNS},
	})
	if deviceStore.DeviceCount() != 1 {
		t.Fatalf("Expected 1 device before delete, got %d", deviceStore.DeviceCount())
	}

	// Send DELETE (ClassANY, TypeA) — delete all A records for oldhost.local
	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	m.Ns = append(m.Ns, &dns.ANY{
		Hdr: dns.RR_Header{
			Name:   "oldhost.local.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassANY,
			Ttl:    0,
		},
	})

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// A records should be gone
	records := deviceStore.LookupName("oldhost.local", dns.TypeA)
	if len(records) != 0 {
		t.Errorf("Expected 0 A records after delete, got %d", len(records))
	}

	// Device should be removed (non-persistent, no remaining IPs)
	if deviceStore.DeviceCount() != 0 {
		t.Errorf("Expected 0 devices after delete (non-persistent), got %d",
			deviceStore.DeviceCount())
	}
}

func TestHandleDDNSUpdate_DeleteSpecificRR(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	// Create a dual-stack device
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		IPv6:      "fd00::24a",
		Source:    discovery.SourceDDNS,
		Sources:   []discovery.DiscoverySource{discovery.SourceDDNS},
	})

	// Delete only the A record (ClassNONE, specific value)
	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	delRR, _ := dns.NewRR("macmini.local. 0 IN A 192.168.1.100")
	delRR.Header().Class = dns.ClassNONE
	delRR.Header().Ttl = 0
	m.Ns = append(m.Ns, delRR)

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// A record should be gone
	aRecs := deviceStore.LookupName("macmini.local", dns.TypeA)
	if len(aRecs) != 0 {
		t.Errorf("Expected 0 A records, got %d", len(aRecs))
	}

	// AAAA record should still exist
	aaaaRecs := deviceStore.LookupName("macmini.local", dns.TypeAAAA)
	if len(aaaaRecs) != 1 {
		t.Fatalf("Expected 1 AAAA record to survive, got %d", len(aaaaRecs))
	}

	// Device should still exist (has IPv6)
	if deviceStore.DeviceCount() != 1 {
		t.Errorf("Expected 1 device (still has IPv6), got %d", deviceStore.DeviceCount())
	}

	device := deviceStore.FindDeviceByIP("fd00::24a")
	if device == nil {
		t.Fatal("Expected device to still exist")
	}
	if device.IPv4 != "" {
		t.Errorf("Expected IPv4 cleared, got %q", device.IPv4)
	}
}

func TestHandleDDNSUpdate_DeleteThenAdd(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	// Create initial device (DHCP lease assigned 192.168.1.100)
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"laptop"},
		IPv4:      "192.168.1.100",
		Source:    discovery.SourceDDNS,
		Sources:   []discovery.DiscoverySource{discovery.SourceDDNS},
	})

	// DHCP renewal: delete old IP + add new IP in same UPDATE
	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")

	// Delete old A record
	delRR, _ := dns.NewRR("laptop.local. 0 IN A 192.168.1.100")
	delRR.Header().Class = dns.ClassNONE
	delRR.Header().Ttl = 0
	m.Ns = append(m.Ns, delRR)

	// Add new A record
	addUpdateRR(m, "laptop.local. 300 IN A 192.168.1.101")

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Should still be 1 device (same hostname)
	if deviceStore.DeviceCount() != 1 {
		t.Errorf("Expected 1 device after lease renewal, got %d", deviceStore.DeviceCount())
	}

	// New IP should be active
	records := deviceStore.LookupName("laptop.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(records))
	}
	if records[0].Value != "192.168.1.101" {
		t.Errorf("Expected new IP 192.168.1.101, got %s", records[0].Value)
	}

	// Old reverse PTR should be gone, new one present
	oldPTR := deviceStore.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(oldPTR) != 0 {
		t.Errorf("Expected old PTR to be gone, got %d records", len(oldPTR))
	}
	newPTR := deviceStore.LookupReverse("101.1.168.192.in-addr.arpa")
	if len(newPTR) != 1 {
		t.Errorf("Expected new PTR, got %d records", len(newPTR))
	}
}

func TestHandleDDNSUpdate_WrongZone(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	w := newDDNSMockWriter()
	m := makeUpdateMsg("evil.com")
	addUpdateRR(m, "hacker.evil.com. 300 IN A 6.6.6.6")

	handleDDNSUpdate(w, m)

	if w.msg == nil {
		t.Fatal("Expected response")
	}
	if w.msg.Rcode != dns.RcodeNotZone {
		t.Errorf("Expected NOTZONE, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// No device should be created
	if deviceStore.DeviceCount() != 0 {
		t.Errorf("Expected 0 devices, got %d", deviceStore.DeviceCount())
	}
}

func TestHandleDDNSUpdate_Disabled(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	ddnsEnabled = false

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("Expected REFUSED when disabled, got %s", dns.RcodeToString[w.msg.Rcode])
	}
	if deviceStore.DeviceCount() != 0 {
		t.Errorf("Expected 0 devices, got %d", deviceStore.DeviceCount())
	}
}

func TestHandleDDNSUpdate_EmptyZoneSection(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	w := newDDNSMockWriter()
	m := new(dns.Msg)
	m.Opcode = dns.OpcodeUpdate
	// No Question section (zone)

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeFormatError {
		t.Errorf("Expected FORMERR for empty zone, got %s", dns.RcodeToString[w.msg.Rcode])
	}
}

func TestHandleDDNSUpdate_EnrichPassiveDevice(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	// Passive discovery created a nameless device
	deviceStore.ObservePassiveQuery("192.168.1.42")
	if deviceStore.DeviceCount() != 1 {
		t.Fatalf("Expected 1 passive device, got %d", deviceStore.DeviceCount())
	}
	passiveDevice := deviceStore.FindDeviceByIP("192.168.1.42")
	if passiveDevice == nil {
		t.Fatal("Expected passive device")
	}
	originalID := passiveDevice.ID

	// DDNS UPDATE names the device
	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "viviennes-ipad.local. 300 IN A 192.168.1.42")

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Should still be 1 device (enriched, not duplicated)
	if deviceStore.DeviceCount() != 1 {
		t.Errorf("Expected 1 device after enrichment, got %d", deviceStore.DeviceCount())
	}

	device := deviceStore.FindDeviceByIP("192.168.1.42")
	if device == nil {
		t.Fatal("Expected device")
	}
	if device.ID != originalID {
		t.Errorf("Expected same device ID %s, got %s", originalID, device.ID)
	}
	if device.DNSName != "viviennes-ipad" {
		t.Errorf("Expected DNSName 'viviennes-ipad', got %q", device.DNSName)
	}
	if !device.HasSource(discovery.SourceDDNS) {
		t.Error("Expected DDNS source after enrichment")
	}
	if !device.HasSource(discovery.SourcePassive) {
		t.Error("Expected passive source preserved after enrichment")
	}

	// DNS records should now exist
	records := deviceStore.LookupName("viviennes-ipad.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(records))
	}
}

func TestHandleDDNSUpdate_MultiZone(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	// Multi-zone setup
	deviceStore = discovery.NewDeviceStoreMultiZone("jvj28.com", "local")

	// UPDATE targets the primary zone
	w := newDDNSMockWriter()
	m := makeUpdateMsg("jvj28.com")
	addUpdateRR(m, "macmini.jvj28.com. 300 IN A 192.168.1.100")

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// A records should exist in BOTH zones
	primaryRecs := deviceStore.LookupName("macmini.jvj28.com", dns.TypeA)
	if len(primaryRecs) != 1 {
		t.Fatalf("Expected 1 A record in jvj28.com, got %d", len(primaryRecs))
	}
	localRecs := deviceStore.LookupName("macmini.local", dns.TypeA)
	if len(localRecs) != 1 {
		t.Fatalf("Expected 1 A record in local, got %d", len(localRecs))
	}

	// PTR should point to primary zone
	ptrRecs := deviceStore.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(ptrRecs) != 1 {
		t.Fatalf("Expected 1 PTR, got %d", len(ptrRecs))
	}
	if ptrRecs[0].Value != "macmini.jvj28.com" {
		t.Errorf("PTR should target primary zone, got %q", ptrRecs[0].Value)
	}
}

func TestHandleDDNSUpdate_MultiZone_SecondaryZoneUpdate(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	deviceStore = discovery.NewDeviceStoreMultiZone("jvj28.com", "local")

	// UPDATE targets the secondary zone (.local) — should also work
	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "printer.local. 300 IN A 192.168.1.50")

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Records in both zones
	if len(deviceStore.LookupName("printer.jvj28.com", dns.TypeA)) != 1 {
		t.Error("Expected A record in jvj28.com")
	}
	if len(deviceStore.LookupName("printer.local", dns.TypeA)) != 1 {
		t.Error("Expected A record in local")
	}
}

// ==========================================================================
// TSIG tests
// ==========================================================================

func TestHandleDDNSUpdate_TSIGValid(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	ddnsTSIGRequired = true

	w := newDDNSMockWriter()
	w.tsigErr = nil // TSIG verification passed

	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")
	m.SetTsig("dhcp-key.", dns.HmacSHA256, 300, time.Now().Unix())

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected NOERROR with valid TSIG, got %s", dns.RcodeToString[w.msg.Rcode])
	}
	if deviceStore.DeviceCount() != 1 {
		t.Errorf("Expected 1 device, got %d", deviceStore.DeviceCount())
	}
}

func TestHandleDDNSUpdate_TSIGInvalid(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	ddnsTSIGRequired = true

	w := newDDNSMockWriter()
	w.tsigErr = fmt.Errorf("TSIG verification failed") // Simulates bad key

	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")
	m.SetTsig("dhcp-key.", dns.HmacSHA256, 300, time.Now().Unix())

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("Expected REFUSED with invalid TSIG, got %s", dns.RcodeToString[w.msg.Rcode])
	}
	if deviceStore.DeviceCount() != 0 {
		t.Errorf("Expected 0 devices (rejected), got %d", deviceStore.DeviceCount())
	}
}

func TestHandleDDNSUpdate_TSIGMissing_Required(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	ddnsTSIGRequired = true

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")
	// No TSIG on message

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("Expected REFUSED when TSIG required but missing, got %s",
			dns.RcodeToString[w.msg.Rcode])
	}
}

func TestHandleDDNSUpdate_TSIGOptional_NoTSIG(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	ddnsTSIGRequired = false // Default

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")
	// No TSIG on message — should be accepted since not required

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected NOERROR when TSIG optional and absent, got %s",
			dns.RcodeToString[w.msg.Rcode])
	}
	if deviceStore.DeviceCount() != 1 {
		t.Errorf("Expected 1 device, got %d", deviceStore.DeviceCount())
	}
}

func TestHandleDDNSUpdate_TSIGOptional_PresentButInvalid(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	ddnsTSIGRequired = false

	w := newDDNSMockWriter()
	w.tsigErr = fmt.Errorf("bad key") // TSIG present but verification failed

	m := makeUpdateMsg("local")
	addUpdateRR(m, "macmini.local. 300 IN A 192.168.1.100")
	m.SetTsig("bad-key.", dns.HmacSHA256, 300, time.Now().Unix())

	handleDDNSUpdate(w, m)

	// Even though TSIG is optional, a present but invalid TSIG should be rejected
	if w.msg.Rcode != dns.RcodeRefused {
		t.Errorf("Expected REFUSED for present but invalid TSIG, got %s",
			dns.RcodeToString[w.msg.Rcode])
	}
}

// ==========================================================================
// Integration test — UPDATE routing via handleDNSRequest
// ==========================================================================

func TestHandleDNSRequest_RoutesUpdateToDDNS(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	addUpdateRR(m, "router.local. 300 IN A 192.168.1.1")

	// Call the main handler — should dispatch to DDNS
	handleDNSRequest(w, m)

	if w.msg == nil {
		t.Fatal("Expected response")
	}
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR from UPDATE via main handler, got %s",
			dns.RcodeToString[w.msg.Rcode])
	}

	// Verify the device was created (proves UPDATE was handled)
	records := deviceStore.LookupName("router.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(records))
	}
}

func TestHandleDNSRequest_StandardQueryNotAffected(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	// Add a device so a standard query can find it
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    discovery.SourceDDNS,
		Sources:   []discovery.DiscoverySource{discovery.SourceDDNS},
	})

	w := newDDNSMockWriter()
	m := new(dns.Msg)
	m.SetQuestion("macmini.local.", dns.TypeA)

	handleDNSRequest(w, m)

	if w.msg == nil {
		t.Fatal("Expected response")
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("Expected 1 answer, got %d", len(w.msg.Answer))
	}
}

// ==========================================================================
// Persistent device delete protection
// ==========================================================================

func TestHandleDDNSUpdate_PersistentDeviceSurvivesDelete(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	// Create a persistent (manually named) device
	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames:  []string{"nas"},
		IPv4:       "192.168.1.200",
		Source:     discovery.SourceManual,
		Sources:    []discovery.DiscoverySource{discovery.SourceManual},
		ManualName: "Dad's NAS",
		Persistent: true,
	})

	// DDNS DELETE for all A records
	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	m.Ns = append(m.Ns, &dns.ANY{
		Hdr: dns.RR_Header{
			Name:   "nas.local.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassANY,
			Ttl:    0,
		},
	})

	handleDDNSUpdate(w, m)

	if w.msg.Rcode != dns.RcodeSuccess {
		t.Fatalf("Expected NOERROR, got %s", dns.RcodeToString[w.msg.Rcode])
	}

	// Persistent device should survive even with no IPs
	if deviceStore.DeviceCount() != 1 {
		t.Errorf("Expected persistent device to survive, got %d devices",
			deviceStore.DeviceCount())
	}
	device := deviceStore.FindDeviceByHostname("nas")
	if device == nil {
		t.Fatal("Expected persistent device to still exist")
	}
	if device.ManualName != "Dad's NAS" {
		t.Errorf("Expected ManualName preserved, got %q", device.ManualName)
	}
}

// ==========================================================================
// Delete nonexistent name (should silently succeed)
// ==========================================================================

func TestHandleDDNSUpdate_DeleteNonexistent(t *testing.T) {
	cleanup := setupDDNSTestServer(t)
	defer cleanup()

	w := newDDNSMockWriter()
	m := makeUpdateMsg("local")
	m.Ns = append(m.Ns, &dns.ANY{
		Hdr: dns.RR_Header{
			Name:   "doesnotexist.local.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassANY,
			Ttl:    0,
		},
	})

	handleDDNSUpdate(w, m)

	// Should succeed (RFC 2136: no-op for nonexistent names)
	if w.msg.Rcode != dns.RcodeSuccess {
		t.Errorf("Expected NOERROR for nonexistent delete, got %s",
			dns.RcodeToString[w.msg.Rcode])
	}
}
