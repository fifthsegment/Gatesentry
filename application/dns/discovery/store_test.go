package discovery

import (
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// --- SanitizeDNSName tests ---

func TestSanitizeDNSName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MacMini", "macmini"},
		{"Vivienne's iPad", "vivienne-s-ipad"},
		{"my--host", "my-host"},
		{"  UPPER CASE  ", "upper-case"},
		{"simple", "simple"},
		{"with.dots.in.name", "with-dots-in-name"},
		{"under_score", "under-score"},
		{"---leading-trailing---", "leading-trailing"},
		{"", ""},
		{"  ", ""},
		{"a", "a"},
		{"Ring-Doorbell-Pro", "ring-doorbell-pro"},
		{"JacquelnsiPhone", "jacquelnsiphone"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeDNSName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeDNSName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeDNSNameMaxLength(t *testing.T) {
	long := strings.Repeat("a", 100)
	result := SanitizeDNSName(long)
	if len(result) > 63 {
		t.Errorf("SanitizeDNSName should truncate to 63 chars, got %d", len(result))
	}
}

// --- reverseIPv4 tests ---

func TestReverseIPv4(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.100", "100.1.168.192.in-addr.arpa"},
		{"10.0.0.1", "1.0.0.10.in-addr.arpa"},
		{"invalid", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := reverseIPv4(tt.input)
			if result != tt.expected {
				t.Errorf("reverseIPv4(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// --- reverseIPv6 tests ---

func TestReverseIPv6(t *testing.T) {
	result := reverseIPv6("fd00:1234:5678::24a")
	if result == "" {
		t.Fatal("reverseIPv6 returned empty for valid IPv6")
	}
	if !strings.HasSuffix(result, ".ip6.arpa") {
		t.Errorf("reverseIPv6 should end with .ip6.arpa, got %q", result)
	}
	// fd00:1234:5678::24a expands to fd00:1234:5678:0000:0000:0000:0000:024a
	// last nibble reversed: a.4.2.0
	if !strings.HasPrefix(result, "a.4.2.0.") {
		t.Errorf("reverseIPv6 should start with a.4.2.0., got %q", result)
	}
}

func TestReverseIPv6Invalid(t *testing.T) {
	result := reverseIPv6("not-an-ip")
	if result != "" {
		t.Errorf("reverseIPv6(invalid) should be empty, got %q", result)
	}
}

// --- Device.GetDisplayName tests ---

func TestDeviceGetDisplayName(t *testing.T) {
	// ManualName takes priority
	d := &Device{ManualName: "My Device", Hostnames: []string{"host1"}}
	if d.GetDisplayName() != "My Device" {
		t.Errorf("Expected ManualName, got %q", d.GetDisplayName())
	}

	// DisplayName next
	d = &Device{DisplayName: "Display", Hostnames: []string{"host1"}}
	if d.GetDisplayName() != "Display" {
		t.Errorf("Expected DisplayName, got %q", d.GetDisplayName())
	}

	// Hostname next
	d = &Device{Hostnames: []string{"host1"}}
	if d.GetDisplayName() != "host1" {
		t.Errorf("Expected hostname, got %q", d.GetDisplayName())
	}

	// mDNS name next
	d = &Device{MDNSNames: []string{"printer._http._tcp"}}
	if d.GetDisplayName() != "printer._http._tcp" {
		t.Errorf("Expected mDNS name, got %q", d.GetDisplayName())
	}

	// MAC fallback
	d = &Device{MACs: []string{"aa:bb:cc:dd:ee:ff"}}
	if d.GetDisplayName() != "Unknown (aa:bb:cc:dd:ee:ff)" {
		t.Errorf("Expected MAC fallback, got %q", d.GetDisplayName())
	}

	// IPv4 fallback
	d = &Device{IPv4: "192.168.1.1"}
	if d.GetDisplayName() != "Unknown (192.168.1.1)" {
		t.Errorf("Expected IPv4 fallback, got %q", d.GetDisplayName())
	}

	// Ultimate fallback
	d = &Device{}
	if d.GetDisplayName() != "Unknown" {
		t.Errorf("Expected Unknown, got %q", d.GetDisplayName())
	}
}

// --- Device.AddSource tests ---

func TestDeviceAddSource(t *testing.T) {
	d := &Device{}
	d.AddSource(SourcePassive)
	d.AddSource(SourceMDNS)
	d.AddSource(SourcePassive) // duplicate

	if len(d.Sources) != 2 {
		t.Errorf("Expected 2 sources, got %d", len(d.Sources))
	}
	if !d.HasSource(SourcePassive) || !d.HasSource(SourceMDNS) {
		t.Error("Missing expected source")
	}
}

// --- DnsRecord.ToRR tests ---

func TestDnsRecordToRR_A(t *testing.T) {
	rec := DnsRecord{Name: "macmini.local", Type: dns.TypeA, Value: "192.168.1.100", TTL: 60}
	rr := rec.ToRR()
	if rr == nil {
		t.Fatal("ToRR returned nil")
	}
	a, ok := rr.(*dns.A)
	if !ok {
		t.Fatal("Expected *dns.A")
	}
	if a.A.String() != "192.168.1.100" {
		t.Errorf("Expected 192.168.1.100, got %s", a.A.String())
	}
	if a.Hdr.Name != "macmini.local." {
		t.Errorf("Expected macmini.local., got %s", a.Hdr.Name)
	}
}

func TestDnsRecordToRR_AAAA(t *testing.T) {
	rec := DnsRecord{Name: "macmini.local", Type: dns.TypeAAAA, Value: "fd00:1234:5678::24a", TTL: 60}
	rr := rec.ToRR()
	if rr == nil {
		t.Fatal("ToRR returned nil")
	}
	aaaa, ok := rr.(*dns.AAAA)
	if !ok {
		t.Fatal("Expected *dns.AAAA")
	}
	if aaaa.AAAA == nil {
		t.Fatal("AAAA address is nil")
	}
}

func TestDnsRecordToRR_PTR(t *testing.T) {
	rec := DnsRecord{Name: "100.1.168.192.in-addr.arpa", Type: dns.TypePTR, Value: "macmini.local", TTL: 60}
	rr := rec.ToRR()
	if rr == nil {
		t.Fatal("ToRR returned nil")
	}
	ptr, ok := rr.(*dns.PTR)
	if !ok {
		t.Fatal("Expected *dns.PTR")
	}
	if ptr.Ptr != "macmini.local." {
		t.Errorf("Expected macmini.local., got %s", ptr.Ptr)
	}
}

// --- DeviceStore tests ---

func TestNewDeviceStore(t *testing.T) {
	ds := NewDeviceStore("local")
	if ds.Zone() != "local" {
		t.Errorf("Expected zone 'local', got %q", ds.Zone())
	}
	if ds.DeviceCount() != 0 {
		t.Errorf("Expected 0 devices, got %d", ds.DeviceCount())
	}
}

func TestNewDeviceStoreDefaultZone(t *testing.T) {
	ds := NewDeviceStore("")
	if ds.Zone() != "local" {
		t.Errorf("Expected default zone 'local', got %q", ds.Zone())
	}
}

func TestUpsertDevice_NewDevice(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		Hostnames: []string{"MacMini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	id := ds.UpsertDevice(device)
	if id == "" {
		t.Fatal("UpsertDevice returned empty ID")
	}
	if ds.DeviceCount() != 1 {
		t.Errorf("Expected 1 device, got %d", ds.DeviceCount())
	}

	// Should generate A record
	records := ds.LookupName("macmini.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(records))
	}
	if records[0].Value != "192.168.1.100" {
		t.Errorf("Expected 192.168.1.100, got %s", records[0].Value)
	}

	// Should generate PTR record
	ptrRecords := ds.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(ptrRecords) != 1 {
		t.Fatalf("Expected 1 PTR record, got %d", len(ptrRecords))
	}
}

func TestUpsertDevice_WithIPv6(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		Hostnames: []string{"MacMini"},
		IPv4:      "192.168.1.100",
		IPv6:      "fd00:1234:5678::24a",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	// A record
	aRecords := ds.LookupName("macmini.local", dns.TypeA)
	if len(aRecords) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(aRecords))
	}

	// AAAA record
	aaaaRecords := ds.LookupName("macmini.local", dns.TypeAAAA)
	if len(aaaaRecords) != 1 {
		t.Fatalf("Expected 1 AAAA record, got %d", len(aaaaRecords))
	}
	if aaaaRecords[0].Value != "fd00:1234:5678::24a" {
		t.Errorf("Expected fd00:1234:5678::24a, got %s", aaaaRecords[0].Value)
	}

	// Both reverse PTR records
	ipv4ptr := ds.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(ipv4ptr) != 1 {
		t.Fatalf("Expected 1 IPv4 PTR record, got %d", len(ipv4ptr))
	}
	ipv6ptr := ds.LookupReverse(reverseIPv6("fd00:1234:5678::24a"))
	if len(ipv6ptr) != 1 {
		t.Fatalf("Expected 1 IPv6 PTR record, got %d", len(ipv6ptr))
	}
}

func TestUpsertDevice_MergeOnUpdate(t *testing.T) {
	ds := NewDeviceStore("local")

	// First upsert — from passive discovery
	device := &Device{
		ID:      "dev-123",
		IPv4:    "192.168.1.42",
		MACs:    []string{"aa:bb:cc:dd:ee:ff"},
		Source:  SourcePassive,
		Sources: []DiscoverySource{SourcePassive},
	}
	ds.UpsertDevice(device)

	// Second upsert — from mDNS (adds hostname)
	update := &Device{
		ID:        "dev-123",
		Hostnames: []string{"Viviennes-iPad"},
		IPv4:      "192.168.1.42",
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	}
	ds.UpsertDevice(update)

	// Should have merged sources
	d := ds.GetDevice("dev-123")
	if d == nil {
		t.Fatal("Device not found")
	}
	if !d.HasSource(SourcePassive) || !d.HasSource(SourceMDNS) {
		t.Error("Sources not merged")
	}
	// Should have both hostname and MAC
	if len(d.Hostnames) != 1 || d.Hostnames[0] != "Viviennes-iPad" {
		t.Errorf("Hostname not set: %v", d.Hostnames)
	}
	if len(d.MACs) != 1 || d.MACs[0] != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC not preserved: %v", d.MACs)
	}
	// Should now have DNS records (has hostname + IP)
	records := ds.LookupName("viviennes-ipad.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 A record after merge, got %d", len(records))
	}
}

func TestUpsertDevice_ManualNamePreserved(t *testing.T) {
	ds := NewDeviceStore("local")

	// User names a device
	device := &Device{
		ID:         "dev-456",
		ManualName: "Dad's Printer",
		IPv4:       "192.168.1.50",
		Source:     SourceManual,
		Sources:    []DiscoverySource{SourceManual},
		Persistent: true,
	}
	ds.UpsertDevice(device)

	// mDNS discovers the same device (matched by ID)
	update := &Device{
		ID:        "dev-456",
		Hostnames: []string{"HP-Printer"},
		IPv4:      "192.168.1.51", // IP changed!
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	}
	ds.UpsertDevice(update)

	d := ds.GetDevice("dev-456")
	if d.ManualName != "Dad's Printer" {
		t.Errorf("ManualName should be preserved, got %q", d.ManualName)
	}
	if d.GetDisplayName() != "Dad's Printer" {
		t.Errorf("DisplayName should prefer ManualName, got %q", d.GetDisplayName())
	}
	if !d.Persistent {
		t.Error("Persistent flag should be preserved")
	}
}

func TestUpdateDeviceIP(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		ID:        "dev-ip",
		Hostnames: []string{"laptop"},
		IPv4:      "192.168.1.10",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	// DHCP renews — new IP
	ds.UpdateDeviceIP("dev-ip", "192.168.1.20", "")

	// Old record gone, new record present
	oldRecords := ds.LookupName("laptop.local", dns.TypeA)
	if len(oldRecords) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(oldRecords))
	}
	if oldRecords[0].Value != "192.168.1.20" {
		t.Errorf("Expected new IP 192.168.1.20, got %s", oldRecords[0].Value)
	}

	// Old PTR gone, new PTR present
	oldPTR := ds.LookupReverse("10.1.168.192.in-addr.arpa")
	if len(oldPTR) != 0 {
		t.Error("Old PTR should be gone")
	}
	newPTR := ds.LookupReverse("20.1.168.192.in-addr.arpa")
	if len(newPTR) != 1 {
		t.Error("New PTR should exist")
	}
}

func TestRemoveDevice(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		ID:        "dev-rm",
		Hostnames: []string{"temporary"},
		IPv4:      "192.168.1.99",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
	}
	ds.UpsertDevice(device)
	if ds.DeviceCount() != 1 {
		t.Fatal("Device not added")
	}

	ds.RemoveDevice("dev-rm")
	if ds.DeviceCount() != 0 {
		t.Error("Device not removed")
	}
	records := ds.LookupName("temporary.local", dns.TypeA)
	if len(records) != 0 {
		t.Error("DNS records should be cleaned up")
	}
}

func TestFindDeviceByHostname(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		Hostnames: []string{"MyLaptop"},
		IPv4:      "192.168.1.10",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	d := ds.FindDeviceByHostname("mylaptop") // case-insensitive
	if d == nil {
		t.Fatal("Device not found by hostname")
	}
	if d.IPv4 != "192.168.1.10" {
		t.Errorf("Wrong device found, IPv4=%s", d.IPv4)
	}
}

func TestFindDeviceByMAC(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		Hostnames: []string{"printer"},
		IPv4:      "192.168.1.50",
		MACs:      []string{"AA:BB:CC:DD:EE:FF"},
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	}
	ds.UpsertDevice(device)

	d := ds.FindDeviceByMAC("aa:bb:cc:dd:ee:ff") // case-insensitive
	if d == nil {
		t.Fatal("Device not found by MAC")
	}
}

func TestFindDeviceByIP(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		IPv4:    "192.168.1.105",
		Source:  SourcePassive,
		Sources: []DiscoverySource{SourcePassive},
	}
	ds.UpsertDevice(device)

	d := ds.FindDeviceByIP("192.168.1.105")
	if d == nil {
		t.Fatal("Device not found by IP")
	}
}

func TestMarkOffline(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		ID:       "dev-offline",
		IPv4:     "192.168.1.10",
		Source:   SourcePassive,
		Sources:  []DiscoverySource{SourcePassive},
		LastSeen: time.Now().Add(-10 * time.Minute),
	}
	// Bypass UpsertDevice's auto LastSeen by setting directly
	ds.mu.Lock()
	device.ID = "dev-offline"
	device.Online = true
	ds.devices[device.ID] = device
	ds.mu.Unlock()

	ds.MarkOffline(5 * time.Minute)

	d := ds.GetDevice("dev-offline")
	if d.Online {
		t.Error("Device should be offline")
	}
}

func TestImportLegacyRecords(t *testing.T) {
	ds := NewDeviceStore("local")
	legacy := map[string]string{
		"nas":      "192.168.1.200",
		"printer":  "192.168.1.50",
		"ipv6host": "fd00::1",
	}
	count := ds.ImportLegacyRecords(legacy)
	if count != 3 {
		t.Errorf("Expected 3 imported, got %d", count)
	}
	if ds.DeviceCount() != 3 {
		t.Errorf("Expected 3 devices, got %d", ds.DeviceCount())
	}

	// Check A record for nas
	records := ds.LookupName("nas.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 A record for nas, got %d", len(records))
	}
	if records[0].Value != "192.168.1.200" {
		t.Errorf("Expected 192.168.1.200, got %s", records[0].Value)
	}

	// Check AAAA record for ipv6host
	records = ds.LookupName("ipv6host.local", dns.TypeAAAA)
	if len(records) != 1 {
		t.Fatalf("Expected 1 AAAA record for ipv6host, got %d", len(records))
	}

	// All should be persistent and manual
	d := ds.FindDeviceByHostname("nas")
	if d == nil {
		t.Fatal("nas not found")
	}
	if !d.Persistent {
		t.Error("Legacy imports should be persistent")
	}
	if d.Source != SourceManual {
		t.Errorf("Legacy imports should be SourceManual, got %s", d.Source)
	}
}

func TestBareHostnameLookup(t *testing.T) {
	ds := NewDeviceStore("local")
	device := &Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	// Lookup by bare hostname (without .local)
	records := ds.LookupName("macmini", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected bare hostname lookup to work, got %d records", len(records))
	}

	// Lookup by FQDN
	records = ds.LookupName("macmini.local", dns.TypeA)
	if len(records) != 1 {
		t.Fatalf("Expected FQDN lookup to work, got %d records", len(records))
	}
}

func TestGetAllDevices(t *testing.T) {
	ds := NewDeviceStore("local")
	for i := 0; i < 5; i++ {
		ds.UpsertDevice(&Device{
			Hostnames: []string{SanitizeDNSName("device-" + string(rune('a'+i)))},
			IPv4:      "192.168.1." + string(rune('1'+i)),
			Source:    SourcePassive,
			Sources:   []DiscoverySource{SourcePassive},
		})
	}
	all := ds.GetAllDevices()
	if len(all) != 5 {
		t.Errorf("Expected 5 devices, got %d", len(all))
	}
}

// --- mergeStringSlice tests ---

func TestMergeStringSlice(t *testing.T) {
	a := []string{"Foo", "Bar"}
	b := []string{"bar", "Baz"} // "bar" is duplicate of "Bar" (case-insensitive)
	result := mergeStringSlice(a, b)
	if len(result) != 3 {
		t.Errorf("Expected 3 items, got %d: %v", len(result), result)
	}
}

func TestMergeStringSliceEmpty(t *testing.T) {
	result := mergeStringSlice(nil, nil)
	if len(result) != 0 {
		t.Errorf("Expected 0 items, got %d", len(result))
	}
}

// --- Concurrent access test ---

func TestConcurrentAccess(t *testing.T) {
	ds := NewDeviceStore("local")

	// Writer goroutine
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			ds.UpsertDevice(&Device{
				Hostnames: []string{"concurrent-test"},
				IPv4:      "192.168.1.1",
				Source:    SourcePassive,
				Sources:   []DiscoverySource{SourcePassive},
			})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			ds.LookupName("concurrent-test.local", dns.TypeA)
			ds.FindDeviceByIP("192.168.1.1")
			ds.GetAllDevices()
		}
		done <- true
	}()

	<-done
	<-done
}

// ==========================================================================
// Multi-Zone tests
// ==========================================================================

func TestNewDeviceStoreMultiZone(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")

	zones := ds.Zones()
	if len(zones) != 2 {
		t.Fatalf("Expected 2 zones, got %d: %v", len(zones), zones)
	}
	if zones[0] != "jvj28.com" {
		t.Errorf("Expected primary zone 'jvj28.com', got %q", zones[0])
	}
	if zones[1] != "local" {
		t.Errorf("Expected secondary zone 'local', got %q", zones[1])
	}
	// Zone() returns the primary
	if ds.Zone() != "jvj28.com" {
		t.Errorf("Zone() should return primary zone, got %q", ds.Zone())
	}
}

func TestNewDeviceStoreMultiZone_Empty(t *testing.T) {
	ds := NewDeviceStoreMultiZone()
	if ds.Zone() != "local" {
		t.Errorf("Expected default zone 'local', got %q", ds.Zone())
	}
}

func TestNewDeviceStoreMultiZone_FiltersEmpty(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "", "  ", "local")
	zones := ds.Zones()
	if len(zones) != 2 {
		t.Fatalf("Expected 2 zones after filtering, got %d: %v", len(zones), zones)
	}
}

func TestMultiZone_RecordsGeneratedForAllZones(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")

	device := &Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		IPv6:      "fd00:1234:5678::24a",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	// A record should exist for BOTH zones
	aRecordsPrimary := ds.LookupName("macmini.jvj28.com", dns.TypeA)
	if len(aRecordsPrimary) != 1 {
		t.Fatalf("Expected 1 A record for macmini.jvj28.com, got %d", len(aRecordsPrimary))
	}
	if aRecordsPrimary[0].Value != "192.168.1.100" {
		t.Errorf("Expected 192.168.1.100, got %s", aRecordsPrimary[0].Value)
	}

	aRecordsLocal := ds.LookupName("macmini.local", dns.TypeA)
	if len(aRecordsLocal) != 1 {
		t.Fatalf("Expected 1 A record for macmini.local, got %d", len(aRecordsLocal))
	}
	if aRecordsLocal[0].Value != "192.168.1.100" {
		t.Errorf("Expected 192.168.1.100, got %s", aRecordsLocal[0].Value)
	}

	// AAAA record should exist for BOTH zones
	aaaaP := ds.LookupName("macmini.jvj28.com", dns.TypeAAAA)
	if len(aaaaP) != 1 {
		t.Fatalf("Expected 1 AAAA record for jvj28.com, got %d", len(aaaaP))
	}
	aaaaL := ds.LookupName("macmini.local", dns.TypeAAAA)
	if len(aaaaL) != 1 {
		t.Fatalf("Expected 1 AAAA record for local, got %d", len(aaaaL))
	}

	// Bare hostname should also work
	aBare := ds.LookupName("macmini", dns.TypeA)
	if len(aBare) != 1 {
		t.Fatalf("Expected 1 A record for bare hostname, got %d", len(aBare))
	}
}

func TestMultiZone_PTRPointsToPrimaryZone(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")

	device := &Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	// PTR should point to the PRIMARY zone (jvj28.com), not local
	ptrRecords := ds.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(ptrRecords) != 1 {
		t.Fatalf("Expected 1 PTR record, got %d", len(ptrRecords))
	}
	if ptrRecords[0].Value != "macmini.jvj28.com" {
		t.Errorf("PTR should point to primary zone: expected 'macmini.jvj28.com', got %q",
			ptrRecords[0].Value)
	}
}

func TestMultiZone_PTRIPv6PointsToPrimary(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")

	device := &Device{
		Hostnames: []string{"server"},
		IPv6:      "fd00::1",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	rev := reverseIPv6("fd00::1")
	ptrRecords := ds.LookupReverse(rev)
	if len(ptrRecords) != 1 {
		t.Fatalf("Expected 1 IPv6 PTR record, got %d", len(ptrRecords))
	}
	if ptrRecords[0].Value != "server.jvj28.com" {
		t.Errorf("IPv6 PTR should point to primary zone, got %q", ptrRecords[0].Value)
	}
}

func TestAddZone(t *testing.T) {
	ds := NewDeviceStore("local")

	// Add a device first
	device := &Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	// Initially only .local records exist
	beforeRecords := ds.LookupName("macmini.jvj28.com", dns.TypeA)
	if len(beforeRecords) != 0 {
		t.Fatalf("Expected 0 records for jvj28.com before AddZone, got %d", len(beforeRecords))
	}

	// Add the new zone
	ds.AddZone("jvj28.com")

	zones := ds.Zones()
	if len(zones) != 2 {
		t.Fatalf("Expected 2 zones, got %d", len(zones))
	}

	// Now both zones should have records
	afterLocal := ds.LookupName("macmini.local", dns.TypeA)
	if len(afterLocal) != 1 {
		t.Fatalf("Expected 1 A record for .local, got %d", len(afterLocal))
	}
	afterCustom := ds.LookupName("macmini.jvj28.com", dns.TypeA)
	if len(afterCustom) != 1 {
		t.Fatalf("Expected 1 A record for .jvj28.com after AddZone, got %d", len(afterCustom))
	}
}

func TestAddZone_NoDuplicate(t *testing.T) {
	ds := NewDeviceStore("local")
	ds.AddZone("local") // duplicate — should be ignored
	ds.AddZone("LOCAL") // case-insensitive duplicate

	zones := ds.Zones()
	if len(zones) != 1 {
		t.Errorf("Expected 1 zone (no duplicates), got %d: %v", len(zones), zones)
	}
}

func TestAddZone_EmptyIgnored(t *testing.T) {
	ds := NewDeviceStore("local")
	ds.AddZone("")
	ds.AddZone("  ")

	zones := ds.Zones()
	if len(zones) != 1 {
		t.Errorf("Expected 1 zone (empty ignored), got %d: %v", len(zones), zones)
	}
}

func TestSetZones(t *testing.T) {
	ds := NewDeviceStore("local")

	device := &Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	// Switch to a completely different zone set
	ds.SetZones([]string{"home.arpa", "jvj28.com"})

	zones := ds.Zones()
	if len(zones) != 2 {
		t.Fatalf("Expected 2 zones, got %d", len(zones))
	}
	if zones[0] != "home.arpa" {
		t.Errorf("Expected primary 'home.arpa', got %q", zones[0])
	}

	// Old .local records should be gone
	oldRecords := ds.LookupName("macmini.local", dns.TypeA)
	if len(oldRecords) != 0 {
		t.Errorf("Expected 0 records for old zone .local, got %d", len(oldRecords))
	}

	// New zones should have records
	newRecords := ds.LookupName("macmini.home.arpa", dns.TypeA)
	if len(newRecords) != 1 {
		t.Fatalf("Expected 1 A record for home.arpa, got %d", len(newRecords))
	}
	customRecords := ds.LookupName("macmini.jvj28.com", dns.TypeA)
	if len(customRecords) != 1 {
		t.Fatalf("Expected 1 A record for jvj28.com, got %d", len(customRecords))
	}
}

func TestSetZones_EmptyFallsBackToLocal(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")
	ds.SetZones([]string{}) // empty → should default to "local"

	if ds.Zone() != "local" {
		t.Errorf("Expected fallback to 'local', got %q", ds.Zone())
	}
}

func TestMultiZone_MultipleDevices(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")

	ds.UpsertDevice(&Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})
	ds.UpsertDevice(&Device{
		Hostnames: []string{"printer"},
		IPv4:      "192.168.1.50",
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	})

	// Each device should have records in both zones
	if len(ds.LookupName("macmini.jvj28.com", dns.TypeA)) != 1 {
		t.Error("Expected macmini A record in jvj28.com")
	}
	if len(ds.LookupName("macmini.local", dns.TypeA)) != 1 {
		t.Error("Expected macmini A record in local")
	}
	if len(ds.LookupName("printer.jvj28.com", dns.TypeA)) != 1 {
		t.Error("Expected printer A record in jvj28.com")
	}
	if len(ds.LookupName("printer.local", dns.TypeA)) != 1 {
		t.Error("Expected printer A record in local")
	}

	// Total record count: 2 devices × 2 zones × 1 A record + 2 devices × 1 PTR + bare hostname aliases
	// The exact count depends on implementation — just verify > single-zone count
	if ds.RecordCount() < 6 {
		t.Errorf("Expected at least 6 records (2 devices × 2 zones + PTRs), got %d", ds.RecordCount())
	}
}

func TestMultiZone_BackwardCompat_SingleZone(t *testing.T) {
	// NewDeviceStore("local") should behave exactly as before
	ds := NewDeviceStore("local")

	device := &Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	ds.UpsertDevice(device)

	zones := ds.Zones()
	if len(zones) != 1 || zones[0] != "local" {
		t.Errorf("Expected single zone [local], got %v", zones)
	}

	aRecords := ds.LookupName("macmini.local", dns.TypeA)
	if len(aRecords) != 1 {
		t.Fatalf("Expected 1 A record, got %d", len(aRecords))
	}

	ptrRecords := ds.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(ptrRecords) != 1 {
		t.Fatalf("Expected 1 PTR record, got %d", len(ptrRecords))
	}
	if ptrRecords[0].Value != "macmini.local" {
		t.Errorf("PTR should point to macmini.local, got %q", ptrRecords[0].Value)
	}
}

func TestMultiZone_CustomDomainAsPrimary(t *testing.T) {
	// Simulate user's real setup: jvj28.com as primary, local as secondary
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")

	// mDNS discovers an iPad
	device := &Device{
		Hostnames: []string{"Viviennes-iPad"},
		MDNSNames: []string{"Vivienne's iPad"},
		IPv4:      "192.168.1.42",
		IPv6:      "fd00::1a3",
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	}
	ds.UpsertDevice(device)

	// User queries viviennes-ipad.jvj28.com — works
	r1 := ds.LookupName("viviennes-ipad.jvj28.com", dns.TypeA)
	if len(r1) == 0 {
		t.Error("Expected A record for viviennes-ipad.jvj28.com")
	}

	// Apple device queries viviennes-ipad.local — also works
	r2 := ds.LookupName("viviennes-ipad.local", dns.TypeA)
	if len(r2) == 0 {
		t.Error("Expected A record for viviennes-ipad.local")
	}

	// AAAA works for both
	r3 := ds.LookupName("viviennes-ipad.jvj28.com", dns.TypeAAAA)
	if len(r3) == 0 {
		t.Error("Expected AAAA record for viviennes-ipad.jvj28.com")
	}
	r4 := ds.LookupName("viviennes-ipad.local", dns.TypeAAAA)
	if len(r4) == 0 {
		t.Error("Expected AAAA record for viviennes-ipad.local")
	}

	// Reverse PTR points to the primary domain (jvj28.com)
	ptr := ds.LookupReverse("42.1.168.192.in-addr.arpa")
	if len(ptr) == 0 {
		t.Fatal("Expected PTR record")
	}
	if ptr[0].Value != "viviennes-ipad.jvj28.com" {
		t.Errorf("PTR should target primary zone: expected 'viviennes-ipad.jvj28.com', got %q",
			ptr[0].Value)
	}
}

// ==========================================================================
// PTR → Primary Domain Round-Trip Tests
// ==========================================================================
// These tests verify the full cycle:
//   forward lookup → extract IP → build reverse arpa name → PTR → primary FQDN
// This catches any inconsistency between forward and reverse indexes.

func TestPTR_RoundTrip_IPv4_SingleZone(t *testing.T) {
	ds := NewDeviceStore("local")
	ds.UpsertDevice(&Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	// Step 1: Forward lookup
	aRecords := ds.LookupName("macmini.local", dns.TypeA)
	if len(aRecords) != 1 {
		t.Fatalf("Forward lookup failed: expected 1 A record, got %d", len(aRecords))
	}
	ip := aRecords[0].Value

	// Step 2: Build reverse name from the IP returned
	rev := reverseIPv4(ip)
	if rev == "" {
		t.Fatalf("reverseIPv4(%q) returned empty", ip)
	}

	// Step 3: PTR lookup
	ptrRecords := ds.LookupReverse(rev)
	if len(ptrRecords) != 1 {
		t.Fatalf("Reverse lookup failed: expected 1 PTR record for %s, got %d", rev, len(ptrRecords))
	}

	// Step 4: PTR target must be the primary zone FQDN
	if ptrRecords[0].Value != "macmini.local" {
		t.Errorf("PTR round-trip: expected 'macmini.local', got %q", ptrRecords[0].Value)
	}

	// Step 5: Verify the PTR target resolves back to the same IP
	backRecords := ds.LookupName(ptrRecords[0].Value, dns.TypeA)
	if len(backRecords) != 1 || backRecords[0].Value != ip {
		t.Errorf("PTR target %q does not resolve back to %s", ptrRecords[0].Value, ip)
	}
}

func TestPTR_RoundTrip_IPv4_MultiZone(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")
	ds.UpsertDevice(&Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	// Forward lookups work for BOTH zones
	for _, zone := range []string{"jvj28.com", "local"} {
		fqdn := "macmini." + zone
		recs := ds.LookupName(fqdn, dns.TypeA)
		if len(recs) != 1 {
			t.Fatalf("Forward lookup %s: expected 1 A record, got %d", fqdn, len(recs))
		}
		if recs[0].Value != "192.168.1.100" {
			t.Errorf("Forward lookup %s: expected 192.168.1.100, got %s", fqdn, recs[0].Value)
		}
	}

	// Reverse lookup from the IP
	rev := reverseIPv4("192.168.1.100")
	ptrRecords := ds.LookupReverse(rev)
	if len(ptrRecords) != 1 {
		t.Fatalf("Expected exactly 1 PTR record, got %d", len(ptrRecords))
	}

	// PTR MUST point to the PRIMARY zone (jvj28.com), never .local
	if ptrRecords[0].Value != "macmini.jvj28.com" {
		t.Errorf("PTR round-trip: expected 'macmini.jvj28.com' (primary), got %q",
			ptrRecords[0].Value)
	}

	// The PTR target must resolve back to the same IP
	backRecords := ds.LookupName(ptrRecords[0].Value, dns.TypeA)
	if len(backRecords) != 1 || backRecords[0].Value != "192.168.1.100" {
		t.Error("PTR target does not resolve back to 192.168.1.100")
	}
}

func TestPTR_RoundTrip_IPv6_MultiZone(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")
	ds.UpsertDevice(&Device{
		Hostnames: []string{"fileserver"},
		IPv6:      "fd00:1234:5678::24a",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	// Forward lookups for both zones
	for _, zone := range []string{"jvj28.com", "local"} {
		fqdn := "fileserver." + zone
		recs := ds.LookupName(fqdn, dns.TypeAAAA)
		if len(recs) != 1 {
			t.Fatalf("Forward AAAA lookup %s: expected 1 record, got %d", fqdn, len(recs))
		}
		if recs[0].Value != "fd00:1234:5678::24a" {
			t.Errorf("Forward AAAA %s: expected fd00:1234:5678::24a, got %s", fqdn, recs[0].Value)
		}
	}

	// Reverse lookup
	rev := reverseIPv6("fd00:1234:5678::24a")
	ptrRecords := ds.LookupReverse(rev)
	if len(ptrRecords) != 1 {
		t.Fatalf("Expected 1 IPv6 PTR record, got %d", len(ptrRecords))
	}

	// PTR must point to PRIMARY zone
	if ptrRecords[0].Value != "fileserver.jvj28.com" {
		t.Errorf("IPv6 PTR round-trip: expected 'fileserver.jvj28.com', got %q",
			ptrRecords[0].Value)
	}

	// PTR target must resolve back
	backRecords := ds.LookupName(ptrRecords[0].Value, dns.TypeAAAA)
	if len(backRecords) != 1 || backRecords[0].Value != "fd00:1234:5678::24a" {
		t.Error("IPv6 PTR target does not resolve back to original address")
	}
}

func TestPTR_RoundTrip_DualStack_MultiZone(t *testing.T) {
	ds := NewDeviceStoreMultiZone("jvj28.com", "local")
	ds.UpsertDevice(&Device{
		Hostnames: []string{"nas"},
		IPv4:      "10.0.0.50",
		IPv6:      "fd00::50",
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	})

	// IPv4 PTR round-trip
	rev4 := reverseIPv4("10.0.0.50")
	ptr4 := ds.LookupReverse(rev4)
	if len(ptr4) != 1 {
		t.Fatalf("Expected 1 IPv4 PTR, got %d", len(ptr4))
	}
	if ptr4[0].Value != "nas.jvj28.com" {
		t.Errorf("IPv4 PTR: expected 'nas.jvj28.com', got %q", ptr4[0].Value)
	}

	// IPv6 PTR round-trip
	rev6 := reverseIPv6("fd00::50")
	ptr6 := ds.LookupReverse(rev6)
	if len(ptr6) != 1 {
		t.Fatalf("Expected 1 IPv6 PTR, got %d", len(ptr6))
	}
	if ptr6[0].Value != "nas.jvj28.com" {
		t.Errorf("IPv6 PTR: expected 'nas.jvj28.com', got %q", ptr6[0].Value)
	}

	// Both PTRs must point to the same canonical name
	if ptr4[0].Value != ptr6[0].Value {
		t.Errorf("IPv4 PTR (%q) and IPv6 PTR (%q) should be identical",
			ptr4[0].Value, ptr6[0].Value)
	}

	// That canonical name resolves for BOTH record types
	a := ds.LookupName(ptr4[0].Value, dns.TypeA)
	aaaa := ds.LookupName(ptr4[0].Value, dns.TypeAAAA)
	if len(a) != 1 || a[0].Value != "10.0.0.50" {
		t.Error("PTR canonical name doesn't resolve A record back")
	}
	if len(aaaa) != 1 || aaaa[0].Value != "fd00::50" {
		t.Error("PTR canonical name doesn't resolve AAAA record back")
	}
}

func TestPTR_RoundTrip_ZoneSwitch(t *testing.T) {
	// Start with "local" as primary, verify PTR → local
	ds := NewDeviceStore("local")
	ds.UpsertDevice(&Device{
		Hostnames: []string{"printer"},
		IPv4:      "192.168.1.55",
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	})

	rev := reverseIPv4("192.168.1.55")
	ptr1 := ds.LookupReverse(rev)
	if len(ptr1) != 1 || ptr1[0].Value != "printer.local" {
		t.Fatalf("Before zone switch: expected PTR → 'printer.local', got %v", ptr1)
	}

	// Switch primary to jvj28.com — PTR should now point to jvj28.com
	ds.SetZones([]string{"jvj28.com", "local"})

	ptr2 := ds.LookupReverse(rev)
	if len(ptr2) != 1 {
		t.Fatalf("After zone switch: expected 1 PTR, got %d", len(ptr2))
	}
	if ptr2[0].Value != "printer.jvj28.com" {
		t.Errorf("After zone switch: PTR should point to new primary 'printer.jvj28.com', got %q",
			ptr2[0].Value)
	}

	// Forward lookup on the new PTR target must work
	back := ds.LookupName(ptr2[0].Value, dns.TypeA)
	if len(back) != 1 || back[0].Value != "192.168.1.55" {
		t.Error("PTR target after zone switch doesn't resolve back")
	}
}

func TestPTR_NoDuplicates_MultiZone(t *testing.T) {
	// Ensure each IP produces exactly ONE PTR record, even with many zones
	ds := NewDeviceStoreMultiZone("jvj28.com", "local", "home.arpa")
	ds.UpsertDevice(&Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		IPv6:      "fd00::1a",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	rev4 := reverseIPv4("192.168.1.100")
	ptr4 := ds.LookupReverse(rev4)
	if len(ptr4) != 1 {
		t.Errorf("IPv4 should have exactly 1 PTR record even with 3 zones, got %d", len(ptr4))
	}

	rev6 := reverseIPv6("fd00::1a")
	ptr6 := ds.LookupReverse(rev6)
	if len(ptr6) != 1 {
		t.Errorf("IPv6 should have exactly 1 PTR record even with 3 zones, got %d", len(ptr6))
	}

	// Forward records should exist in all 3 zones
	for _, zone := range []string{"jvj28.com", "local", "home.arpa"} {
		fqdn := "macmini." + zone
		if len(ds.LookupName(fqdn, dns.TypeA)) != 1 {
			t.Errorf("Expected A record for %s", fqdn)
		}
		if len(ds.LookupName(fqdn, dns.TypeAAAA)) != 1 {
			t.Errorf("Expected AAAA record for %s", fqdn)
		}
	}

	// PTR always targets primary
	if ptr4[0].Value != "macmini.jvj28.com" {
		t.Errorf("PTR should target primary 'macmini.jvj28.com', got %q", ptr4[0].Value)
	}
}

// ==========================================================================
// IP Conflict Eviction tests
// ==========================================================================

func TestUpsertDevice_EvictsIPv4FromOtherDevice(t *testing.T) {
	ds := NewDeviceStore("local")

	// Device A gets 192.168.1.100
	idA := ds.UpsertDevice(&Device{
		Hostnames: []string{"device-a"},
		IPv4:      "192.168.1.100",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
	})

	// Verify A has the IP
	a := ds.FindDeviceByIP("192.168.1.100")
	if a == nil || a.ID != idA {
		t.Fatal("Expected device A to own 192.168.1.100")
	}

	// Device B gets the SAME IP (DHCP reassigned it)
	idB := ds.UpsertDevice(&Device{
		Hostnames: []string{"device-b"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	// B now owns the IP
	b := ds.FindDeviceByIP("192.168.1.100")
	if b == nil || b.ID != idB {
		t.Fatalf("Expected device B to own 192.168.1.100, got %+v", b)
	}

	// A should have lost the IP
	aAfter := ds.GetDevice(idA)
	if aAfter == nil {
		t.Fatal("Device A should still exist")
	}
	if aAfter.IPv4 != "" {
		t.Errorf("Device A should have empty IPv4 after eviction, got %q", aAfter.IPv4)
	}

	// A should have no DNS records
	recsA := ds.LookupName("device-a.local", dns.TypeA)
	if len(recsA) != 0 {
		t.Errorf("Expected 0 A records for device-a after eviction, got %d", len(recsA))
	}

	// B should have DNS records
	recsB := ds.LookupName("device-b.local", dns.TypeA)
	if len(recsB) != 1 {
		t.Errorf("Expected 1 A record for device-b, got %d", len(recsB))
	}
}

func TestUpsertDevice_EvictsIPv6FromOtherDevice(t *testing.T) {
	ds := NewDeviceStore("local")

	idA := ds.UpsertDevice(&Device{
		Hostnames: []string{"device-a"},
		IPv6:      "fd00::100",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
	})

	idB := ds.UpsertDevice(&Device{
		Hostnames: []string{"device-b"},
		IPv6:      "fd00::100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	// B owns it
	b := ds.FindDeviceByIP("fd00::100")
	if b == nil || b.ID != idB {
		t.Fatalf("Expected device B to own fd00::100")
	}

	// A lost it
	aAfter := ds.GetDevice(idA)
	if aAfter.IPv6 != "" {
		t.Errorf("Device A should have empty IPv6, got %q", aAfter.IPv6)
	}
}

func TestUpsertDevice_SameDeviceSameIP_NoEviction(t *testing.T) {
	ds := NewDeviceStore("local")

	id := ds.UpsertDevice(&Device{
		Hostnames: []string{"device-a"},
		IPv4:      "192.168.1.50",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	// Re-upsert same device with same IP
	ds.UpsertDevice(&Device{
		ID:        id,
		Hostnames: []string{"device-a"},
		IPv4:      "192.168.1.50",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	d := ds.GetDevice(id)
	if d.IPv4 != "192.168.1.50" {
		t.Errorf("Expected IP preserved on same-device upsert, got %q", d.IPv4)
	}
}

func TestUpdateDeviceIP_EvictsFromOtherDevice(t *testing.T) {
	ds := NewDeviceStore("local")

	idA := ds.UpsertDevice(&Device{
		Hostnames: []string{"device-a"},
		IPv4:      "192.168.1.100",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
	})

	idB := ds.UpsertDevice(&Device{
		Hostnames: []string{"device-b"},
		IPv4:      "192.168.1.200",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
	})

	// B takes A's IP via UpdateDeviceIP
	ds.UpdateDeviceIP(idB, "192.168.1.100", "")

	bAfter := ds.GetDevice(idB)
	if bAfter.IPv4 != "192.168.1.100" {
		t.Errorf("Expected B to have 192.168.1.100, got %q", bAfter.IPv4)
	}

	aAfter := ds.GetDevice(idA)
	if aAfter.IPv4 != "" {
		t.Errorf("Expected A to lose 192.168.1.100 after eviction, got %q", aAfter.IPv4)
	}

	// DNS records should reflect the new state
	recsB := ds.LookupName("device-b.local", dns.TypeA)
	if len(recsB) != 1 || recsB[0].Value != "192.168.1.100" {
		t.Errorf("Expected B to have A record for 192.168.1.100, got %+v", recsB)
	}
	recsA := ds.LookupName("device-a.local", dns.TypeA)
	if len(recsA) != 0 {
		t.Errorf("Expected A to have no A records after eviction, got %d", len(recsA))
	}
}

func TestUpsertDevice_EvictionPreservesDeviceIdentity(t *testing.T) {
	ds := NewDeviceStore("local")

	// Device A: has hostname, MAC, and IP
	idA := ds.UpsertDevice(&Device{
		Hostnames: []string{"printer"},
		MACs:      []string{"aa:bb:cc:dd:ee:ff"},
		IPv4:      "192.168.1.100",
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
	})

	// Device B steals A's IP
	ds.UpsertDevice(&Device{
		Hostnames: []string{"laptop"},
		IPv4:      "192.168.1.100",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	})

	// A should still exist with hostname and MAC, just no IP
	aAfter := ds.GetDevice(idA)
	if aAfter == nil {
		t.Fatal("Device A should still exist after IP eviction")
	}
	if len(aAfter.Hostnames) == 0 || aAfter.Hostnames[0] != "printer" {
		t.Errorf("Expected hostname 'printer' preserved, got %v", aAfter.Hostnames)
	}
	if len(aAfter.MACs) == 0 || aAfter.MACs[0] != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("Expected MAC preserved, got %v", aAfter.MACs)
	}
	if aAfter.IPv4 != "" {
		t.Errorf("Expected empty IPv4 after eviction, got %q", aAfter.IPv4)
	}
}
