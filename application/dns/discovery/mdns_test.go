package discovery

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/oleksandr/bonjour"
)

// ==========================================================================
// CleanMDNSHostname tests
// ==========================================================================

func TestCleanMDNSHostname(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Viviennes-iPad.local.", "Viviennes-iPad"},
		{"macmini.local", "macmini"},
		{"printer.", "printer"},
		{"myhost", "myhost"},
		{"", ""},
		{"  spaced.local.  ", "spaced"},
		{"just-a-dot.", "just-a-dot"},
		{"host.other.domain.", "host.other.domain"},
		{"UPPERCASE.local.", "UPPERCASE"},
		{"multi.dots.name.local.", "multi.dots.name"},
		{"  ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CleanMDNSHostname(tt.input)
			if got != tt.expected {
				t.Errorf("CleanMDNSHostname(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ==========================================================================
// IsLinkLocalIPv6 tests
// ==========================================================================

func TestIsLinkLocalIPv6(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"fe80::1", true},
		{"fe80::abcd:ef01:2345:6789", true},
		{"fd00::1", false},      // ULA — not link-local
		{"2001:db8::1", false},  // Documentation range
		{"::1", false},          // Loopback
		{"192.168.1.1", false},  // IPv4
		{"", false},             // Empty
		{"invalid", false},      // Garbage
		{"fe80::", true},        // Minimal link-local
		{"fd12:3456::1", false}, // ULA
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsLinkLocalIPv6(tt.input)
			if got != tt.expected {
				t.Errorf("IsLinkLocalIPv6(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// ==========================================================================
// processEntry tests
// ==========================================================================

func TestProcessEntry_NewDevice(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	entry := bonjour.NewServiceEntry("Vivienne's iPad", "_airplay._tcp", "local")
	entry.HostName = "Viviennes-iPad.local."
	entry.Port = 7000
	entry.AddrIPv4 = net.ParseIP("192.168.1.42")

	browser.processEntry(entry)

	if store.DeviceCount() != 1 {
		t.Fatalf("Expected 1 device, got %d", store.DeviceCount())
	}

	device := store.FindDeviceByIP("192.168.1.42")
	if device == nil {
		t.Fatal("Expected to find device by IP")
	}
	if device.IPv4 != "192.168.1.42" {
		t.Errorf("Expected IPv4 192.168.1.42, got %s", device.IPv4)
	}
	if len(device.MDNSNames) == 0 || device.MDNSNames[0] != "Vivienne's iPad" {
		t.Errorf("Expected MDNSNames[0] = %q, got %v", "Vivienne's iPad", device.MDNSNames)
	}
	if len(device.Hostnames) == 0 || device.Hostnames[0] != "Viviennes-iPad" {
		t.Errorf("Expected Hostnames[0] = %q, got %v", "Viviennes-iPad", device.Hostnames)
	}
	if !device.HasSource(SourceMDNS) {
		t.Error("Expected device to have mDNS source")
	}
	if device.Source != SourceMDNS {
		t.Errorf("Expected primary source mDNS, got %s", device.Source)
	}
	// DNS name should be derived from the hostname
	if device.DNSName == "" {
		t.Error("Expected DNS name to be derived")
	}
	// The sanitized DNS name should be lowercase
	if device.DNSName != "viviennes-ipad" {
		t.Errorf("Expected DNSName 'viviennes-ipad', got %q", device.DNSName)
	}

	// Should generate DNS records
	records := store.LookupName("viviennes-ipad.local", dns.TypeA)
	if len(records) == 0 {
		t.Error("Expected A record for viviennes-ipad.local")
	}
	if len(records) > 0 && records[0].Value != "192.168.1.42" {
		t.Errorf("Expected A record value 192.168.1.42, got %s", records[0].Value)
	}
}

func TestProcessEntry_EnrichPassiveDevice(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Phase 2: passive discovery creates a device (just IP, no name)
	store.ObservePassiveQuery("192.168.1.42")
	if store.DeviceCount() != 1 {
		t.Fatalf("Expected 1 passive device, got %d", store.DeviceCount())
	}

	passiveDevice := store.FindDeviceByIP("192.168.1.42")
	if passiveDevice == nil {
		t.Fatal("Expected passive device to exist")
	}
	originalID := passiveDevice.ID

	// Passive device should have no name yet
	if passiveDevice.DNSName != "" {
		t.Errorf("Passive device should have no DNS name, got %q", passiveDevice.DNSName)
	}

	// Phase 3: mDNS discovers the same device — enriches with identity
	entry := bonjour.NewServiceEntry("Vivienne's iPad", "_airplay._tcp", "local")
	entry.HostName = "Viviennes-iPad.local."
	entry.Port = 7000
	entry.AddrIPv4 = net.ParseIP("192.168.1.42")

	browser.processEntry(entry)

	// Should still be 1 device (enriched, not duplicated)
	if store.DeviceCount() != 1 {
		t.Fatalf("Expected 1 device after enrichment, got %d", store.DeviceCount())
	}

	device := store.FindDeviceByIP("192.168.1.42")
	if device == nil {
		t.Fatal("Expected to find enriched device")
	}

	// Same device — not a new one
	if device.ID != originalID {
		t.Errorf("Expected same device ID %s, got %s", originalID, device.ID)
	}

	// Now has mDNS identity
	if len(device.MDNSNames) == 0 {
		t.Error("Expected MDNSNames to be populated after enrichment")
	}
	if len(device.Hostnames) == 0 || device.Hostnames[0] != "Viviennes-iPad" {
		t.Errorf("Expected hostname 'Viviennes-iPad', got %v", device.Hostnames)
	}

	// Both sources recorded
	if !device.HasSource(SourcePassive) {
		t.Error("Expected device to retain passive source")
	}
	if !device.HasSource(SourceMDNS) {
		t.Error("Expected device to gain mDNS source after enrichment")
	}

	// DNS name should now be derived
	if device.DNSName == "" {
		t.Error("Expected DNS name to be derived after enrichment")
	}

	// DNS records should now exist
	records := store.LookupName("viviennes-ipad.local", dns.TypeA)
	if len(records) == 0 {
		t.Error("Expected A record after enrichment")
	}
}

func TestProcessEntry_MultipleServiceTypes(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Same device discovered via AirPlay
	entry1 := bonjour.NewServiceEntry("Apple TV", "_airplay._tcp", "local")
	entry1.HostName = "Apple-TV.local."
	entry1.AddrIPv4 = net.ParseIP("192.168.1.50")

	// Same device discovered via RAOP (same IP)
	entry2 := bonjour.NewServiceEntry("Apple TV", "_raop._tcp", "local")
	entry2.HostName = "Apple-TV.local."
	entry2.AddrIPv4 = net.ParseIP("192.168.1.50")

	// Same device discovered via Companion Link
	entry3 := bonjour.NewServiceEntry("Apple TV", "_companion-link._tcp", "local")
	entry3.HostName = "Apple-TV.local."
	entry3.AddrIPv4 = net.ParseIP("192.168.1.50")

	browser.processEntry(entry1)
	browser.processEntry(entry2)
	browser.processEntry(entry3)

	// Should be 1 device, not 3 — all matched by IP
	if store.DeviceCount() != 1 {
		t.Fatalf("Expected 1 device for same IP, got %d", store.DeviceCount())
	}

	device := store.FindDeviceByIP("192.168.1.50")
	if device == nil {
		t.Fatal("Expected to find device")
	}
	if device.DNSName != "apple-tv" {
		t.Errorf("Expected DNSName 'apple-tv', got %q", device.DNSName)
	}
}

func TestProcessEntry_NilEntry(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Should not panic
	browser.processEntry(nil)

	if store.DeviceCount() != 0 {
		t.Errorf("Expected 0 devices after nil entry, got %d", store.DeviceCount())
	}
}

func TestProcessEntry_NoIPNoHostname(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Entry with no useful identity
	entry := bonjour.NewServiceEntry("", "_http._tcp", "local")
	browser.processEntry(entry)

	if store.DeviceCount() != 0 {
		t.Errorf("Expected 0 devices for entry with no identity, got %d", store.DeviceCount())
	}
}

func TestProcessEntry_IPv6Only(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	entry := bonjour.NewServiceEntry("Linux Box", "_ssh._tcp", "local")
	entry.HostName = "linuxbox.local."
	entry.AddrIPv6 = net.ParseIP("fd00::1234")

	browser.processEntry(entry)

	if store.DeviceCount() != 1 {
		t.Fatalf("Expected 1 device, got %d", store.DeviceCount())
	}

	device := store.FindDeviceByIP("fd00::1234")
	if device == nil {
		t.Fatal("Expected to find device by IPv6")
	}
	if device.IPv6 != "fd00::1234" {
		t.Errorf("Expected IPv6 fd00::1234, got %s", device.IPv6)
	}
	if device.DNSName != "linuxbox" {
		t.Errorf("Expected DNSName 'linuxbox', got %q", device.DNSName)
	}

	// Should generate AAAA record
	records := store.LookupName("linuxbox.local", dns.TypeAAAA)
	if len(records) == 0 {
		t.Error("Expected AAAA record for IPv6-only device")
	}
}

func TestProcessEntry_BothIPv4AndIPv6(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	entry := bonjour.NewServiceEntry("Mac Mini", "_http._tcp", "local")
	entry.HostName = "macmini.local."
	entry.AddrIPv4 = net.ParseIP("192.168.1.100")
	entry.AddrIPv6 = net.ParseIP("fd00::24a")

	browser.processEntry(entry)

	device := store.FindDeviceByIP("192.168.1.100")
	if device == nil {
		t.Fatal("Expected to find device")
	}
	if device.IPv4 != "192.168.1.100" {
		t.Errorf("Expected IPv4 192.168.1.100, got %s", device.IPv4)
	}
	if device.IPv6 != "fd00::24a" {
		t.Errorf("Expected IPv6 fd00::24a, got %s", device.IPv6)
	}

	// Should have both A and AAAA records
	aRecords := store.LookupName("macmini.local", dns.TypeA)
	if len(aRecords) == 0 {
		t.Error("Expected A record")
	}
	aaaaRecords := store.LookupName("macmini.local", dns.TypeAAAA)
	if len(aaaaRecords) == 0 {
		t.Error("Expected AAAA record")
	}

	// Should also have PTR records
	ptrRecords := store.LookupReverse("100.1.168.192.in-addr.arpa")
	if len(ptrRecords) == 0 {
		t.Error("Expected PTR record for IPv4 reverse")
	}
}

func TestProcessEntry_PreservesExistingIPv4(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Create a device with IPv4 and hostname (e.g., from prior discovery)
	device := &Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.100",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
	}
	store.UpsertDevice(device)

	// Verify initial state
	found := store.FindDeviceByHostname("macmini")
	if found == nil {
		t.Fatal("Expected to find device by hostname")
	}
	if found.IPv4 != "192.168.1.100" {
		t.Fatalf("Expected initial IPv4 192.168.1.100, got %s", found.IPv4)
	}

	// mDNS discovers same device with only IPv6 (no IPv4 in this entry)
	entry := bonjour.NewServiceEntry("Mac Mini", "_http._tcp", "local")
	entry.HostName = "macmini.local."
	entry.AddrIPv6 = net.ParseIP("fd00::24a")
	// AddrIPv4 is nil — mDNS didn't return it

	browser.processEntry(entry)

	// IPv4 should be preserved, IPv6 should be added
	found = store.FindDeviceByHostname("macmini")
	if found == nil {
		t.Fatal("Expected to find enriched device")
	}
	if found.IPv4 != "192.168.1.100" {
		t.Errorf("Expected IPv4 preserved as 192.168.1.100, got %s", found.IPv4)
	}
	if found.IPv6 != "fd00::24a" {
		t.Errorf("Expected IPv6 fd00::24a, got %s", found.IPv6)
	}
}

func TestProcessEntry_PrefersGUAOverLinkLocal(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Device already discovered with a GUA IPv6 (e.g., from DDNS)
	device := &Device{
		Hostnames: []string{"server"},
		IPv4:      "192.168.1.200",
		IPv6:      "2001:db8::1",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	store.UpsertDevice(device)

	// mDNS finds same device but only reports link-local IPv6
	entry := bonjour.NewServiceEntry("Server", "_http._tcp", "local")
	entry.HostName = "server.local."
	entry.AddrIPv4 = net.ParseIP("192.168.1.200")
	entry.AddrIPv6 = net.ParseIP("fe80::1234")

	browser.processEntry(entry)

	found := store.FindDeviceByHostname("server")
	if found == nil {
		t.Fatal("Expected to find device")
	}
	// GUA should be preserved — link-local should NOT overwrite it
	if found.IPv6 != "2001:db8::1" {
		t.Errorf("Expected GUA IPv6 preserved as 2001:db8::1, got %s", found.IPv6)
	}
}

func TestProcessEntry_LinkLocalAcceptedWhenNoExisting(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// New device with only link-local IPv6 — should still be stored
	entry := bonjour.NewServiceEntry("IoT Sensor", "_http._tcp", "local")
	entry.HostName = "sensor.local."
	entry.AddrIPv6 = net.ParseIP("fe80::abcd")

	browser.processEntry(entry)

	device := store.FindDeviceByIP("fe80::abcd")
	if device == nil {
		t.Fatal("Expected link-local device to be stored")
	}
	if device.IPv6 != "fe80::abcd" {
		t.Errorf("Expected IPv6 fe80::abcd, got %s", device.IPv6)
	}
}

func TestProcessEntry_HostnameOnly(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Entry with hostname but no IPs (unusual but possible)
	entry := bonjour.NewServiceEntry("Mystery Device", "_http._tcp", "local")
	entry.HostName = "mystery.local."
	// No AddrIPv4 or AddrIPv6

	browser.processEntry(entry)

	// Should create a device (hostname alone is sufficient)
	if store.DeviceCount() != 1 {
		t.Fatalf("Expected 1 device, got %d", store.DeviceCount())
	}

	device := store.FindDeviceByHostname("mystery")
	if device == nil {
		// Also try the mDNS instance name
		device = store.FindDeviceByHostname("Mystery Device")
	}
	if device == nil {
		t.Fatal("Expected to find device by hostname or instance name")
	}
}

func TestProcessEntry_MatchByHostname(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// First service type discovers device
	entry1 := bonjour.NewServiceEntry("NAS", "_smb._tcp", "local")
	entry1.HostName = "mynas.local."
	entry1.AddrIPv4 = net.ParseIP("192.168.1.150")

	browser.processEntry(entry1)

	// Second service type for same device, but with different IP
	// (device got a new DHCP lease between scans — unlikely within one scan but tests the logic)
	entry2 := bonjour.NewServiceEntry("NAS", "_http._tcp", "local")
	entry2.HostName = "mynas.local."
	entry2.AddrIPv4 = net.ParseIP("192.168.1.151")

	browser.processEntry(entry2)

	// Should still be 1 device (matched by hostname)
	if store.DeviceCount() != 1 {
		t.Fatalf("Expected 1 device, got %d", store.DeviceCount())
	}

	device := store.FindDeviceByHostname("mynas")
	if device == nil {
		t.Fatal("Expected to find device")
	}
	// IP should be updated to the latest
	if device.IPv4 != "192.168.1.151" {
		t.Errorf("Expected IPv4 updated to 192.168.1.151, got %s", device.IPv4)
	}
}

// ==========================================================================
// UpsertDevice IP preservation tests (verifies the store.go change)
// ==========================================================================

func TestUpsertDevice_PreservesIPv4WhenEmpty(t *testing.T) {
	store := NewDeviceStore("local")

	// Create device with IPv4
	d1 := &Device{
		Hostnames: []string{"test-host"},
		IPv4:      "10.0.0.1",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
	}
	id := store.UpsertDevice(d1)

	// Upsert same device with empty IPv4 (simulating a source that doesn't know the IP)
	d2 := &Device{
		ID:        id,
		Hostnames: []string{"test-host"},
		IPv6:      "fd00::1",
		Source:    SourceMDNS,
		Sources:   []DiscoverySource{SourceMDNS},
		// IPv4 intentionally empty
	}
	store.UpsertDevice(d2)

	found := store.GetDevice(id)
	if found == nil {
		t.Fatal("Expected to find device")
	}
	if found.IPv4 != "10.0.0.1" {
		t.Errorf("Expected IPv4 preserved as 10.0.0.1, got %q", found.IPv4)
	}
	if found.IPv6 != "fd00::1" {
		t.Errorf("Expected IPv6 fd00::1, got %q", found.IPv6)
	}
}

func TestUpsertDevice_PreservesIPv6WhenEmpty(t *testing.T) {
	store := NewDeviceStore("local")

	d1 := &Device{
		Hostnames: []string{"test-host"},
		IPv6:      "fd00::99",
		Source:    SourceDDNS,
		Sources:   []DiscoverySource{SourceDDNS},
	}
	id := store.UpsertDevice(d1)

	d2 := &Device{
		ID:        id,
		Hostnames: []string{"test-host"},
		IPv4:      "10.0.0.2",
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
		// IPv6 intentionally empty
	}
	store.UpsertDevice(d2)

	found := store.GetDevice(id)
	if found == nil {
		t.Fatal("Expected to find device")
	}
	if found.IPv6 != "fd00::99" {
		t.Errorf("Expected IPv6 preserved as fd00::99, got %q", found.IPv6)
	}
	if found.IPv4 != "10.0.0.2" {
		t.Errorf("Expected IPv4 10.0.0.2, got %q", found.IPv4)
	}
}

// ==========================================================================
// Browser constructor and lifecycle tests
// ==========================================================================

func TestNewMDNSBrowser_Defaults(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, 0) // 0 → default interval

	if browser.interval != DefaultScanInterval {
		t.Errorf("Expected default interval %s, got %s", DefaultScanInterval, browser.interval)
	}
	if browser.browseTimeout != DefaultBrowseTimeout {
		t.Errorf("Expected default browse timeout %s, got %s", DefaultBrowseTimeout, browser.browseTimeout)
	}
	if len(browser.serviceTypes) == 0 {
		t.Error("Expected default service types to be set")
	}
	if browser.store != store {
		t.Error("Expected store to be set")
	}
}

func TestNewMDNSBrowser_CustomInterval(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, 30*time.Second)

	if browser.interval != 30*time.Second {
		t.Errorf("Expected interval 30s, got %s", browser.interval)
	}
}

func TestMDNSBrowser_SetServiceTypes(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	custom := []string{"_http._tcp", "_ssh._tcp"}
	browser.SetServiceTypes(custom)

	browser.mu.Lock()
	if len(browser.serviceTypes) != 2 {
		t.Errorf("Expected 2 service types, got %d", len(browser.serviceTypes))
	}
	if browser.serviceTypes[0] != "_http._tcp" {
		t.Errorf("Expected first type _http._tcp, got %s", browser.serviceTypes[0])
	}
	browser.mu.Unlock()
}

func TestMDNSBrowser_SetBrowseTimeout(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	browser.SetBrowseTimeout(2 * time.Second)

	browser.mu.Lock()
	if browser.browseTimeout != 2*time.Second {
		t.Errorf("Expected browse timeout 2s, got %s", browser.browseTimeout)
	}
	browser.mu.Unlock()
}

func TestMDNSBrowser_StartStop(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Hour) // Long interval — won't trigger during test
	browser.SetBrowseTimeout(100 * time.Millisecond)
	browser.SetServiceTypes([]string{"_test._tcp"}) // Minimal — one type, fast timeout

	if browser.IsRunning() {
		t.Error("Browser should not be running before Start")
	}

	browser.Start()

	// Give the initial scan a moment to run and complete
	time.Sleep(500 * time.Millisecond)

	if !browser.IsRunning() {
		t.Error("Browser should be running after Start")
	}

	// Double Start is a no-op
	browser.Start()
	if !browser.IsRunning() {
		t.Error("Browser should still be running after double Start")
	}

	browser.Stop()
	if browser.IsRunning() {
		t.Error("Browser should not be running after Stop")
	}

	// Double Stop is a no-op
	browser.Stop()
}

func TestMDNSBrowser_StopBeforeStart(t *testing.T) {
	store := NewDeviceStore("local")
	browser := NewMDNSBrowser(store, time.Minute)

	// Should not panic or block
	browser.Stop()
}

// ==========================================================================
// DefaultServiceTypes validation
// ==========================================================================

func TestDefaultServiceTypes_NotEmpty(t *testing.T) {
	if len(DefaultServiceTypes) == 0 {
		t.Error("DefaultServiceTypes should not be empty")
	}
}

func TestDefaultServiceTypes_ValidFormat(t *testing.T) {
	for _, svc := range DefaultServiceTypes {
		if svc == "" {
			t.Error("Service type should not be empty")
		}
		if svc[0] != '_' {
			t.Errorf("Service type %q should start with underscore", svc)
		}
		// Should contain either _tcp or _udp
		hasTCP := len(svc) > 4 && (svc[len(svc)-4:] == "._tcp" || contains(svc, "._tcp"))
		hasUDP := len(svc) > 4 && (svc[len(svc)-4:] == "._udp" || contains(svc, "._udp"))
		if !hasTCP && !hasUDP {
			t.Errorf("Service type %q should contain ._tcp or ._udp", svc)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
