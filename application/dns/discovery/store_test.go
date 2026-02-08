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
		ID:   "dev-123",
		IPv4: "192.168.1.42",
		MACs: []string{"aa:bb:cc:dd:ee:ff"},
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
