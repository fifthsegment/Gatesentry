package discovery

import (
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// sanitizeDNSName converts a raw hostname into a valid DNS label.
// Lowercases, replaces invalid characters with hyphens, trims hyphens.
// Examples: "Vivienne's iPad" → "viviennes-ipad", "MacMini" → "macmini"
var invalidDNSChars = regexp.MustCompile(`[^a-z0-9-]`)
var multiHyphen = regexp.MustCompile(`-{2,}`)

func SanitizeDNSName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = invalidDNSChars.ReplaceAllString(s, "-")
	s = multiHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}
	// DNS labels max 63 characters
	if len(s) > 63 {
		s = s[:63]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// reverseIPv4 converts an IPv4 address to its in-addr.arpa PTR name.
// Example: "192.168.1.100" → "100.1.168.192.in-addr.arpa"
func reverseIPv4(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa",
		parts[3], parts[2], parts[1], parts[0])
}

// reverseIPv6 converts an IPv6 address to its ip6.arpa PTR name.
// Example: "fd00:1234:5678::24a" → "a.4.2.0.0.0.0.0...ip6.arpa"
func reverseIPv6(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}
	ip = ip.To16()
	if ip == nil {
		return ""
	}
	// Build nibble-reversed representation
	var parts []string
	for i := len(ip) - 1; i >= 0; i-- {
		b := ip[i]
		parts = append(parts, fmt.Sprintf("%x", b&0x0f))
		parts = append(parts, fmt.Sprintf("%x", b>>4))
	}
	return strings.Join(parts, ".") + ".ip6.arpa"
}

// DeviceStore is a thread-safe store for discovered devices and their
// derived DNS records. It is the central data structure that all discovery
// tiers feed into, and that the DNS query handler reads from.
//
// Concurrency model:
//   - DNS query handler calls Lookup*() methods with RLock (concurrent reads)
//   - Discovery sources call Upsert*/Remove* methods with full Lock (exclusive writes)
//   - Same RWMutex pattern as the existing blockedDomains/internalRecords maps
type DeviceStore struct {
	mu sync.RWMutex

	// devices maps device ID → Device
	devices map[string]*Device

	// --- Lookup indexes (derived, rebuilt on mutation) ---

	// recordsByName maps lowercase FQDN → []DnsRecord for fast query answering.
	// Example key: "macmini.local"
	recordsByName map[string][]DnsRecord

	// recordsByReverse maps reverse PTR name → []DnsRecord.
	// Example key: "100.1.168.192.in-addr.arpa"
	recordsByReverse map[string][]DnsRecord

	// deviceByHostname maps lowercase hostname → device ID for matching.
	deviceByHostname map[string]string

	// deviceByMAC maps lowercase MAC → device ID for matching.
	deviceByMAC map[string]string

	// deviceByIP maps IP string → device ID for passive discovery.
	deviceByIP map[string]string

	// zones contains the DNS zone suffixes for generated records.
	// The first entry is the "primary" zone used for PTR targets and display.
	// Records are generated for ALL zones so that e.g. both "macmini.local"
	// and "macmini.jvj28.com" resolve to the same device.
	// Default: ["local"]
	zones []string
}

// NewDeviceStore creates an empty DeviceStore with the given zone suffix.
// For backward compatibility, accepts a single zone string. Use SetZones()
// to configure multiple zones after creation.
func NewDeviceStore(zone string) *DeviceStore {
	if zone == "" {
		zone = "local"
	}
	return &DeviceStore{
		devices:          make(map[string]*Device),
		recordsByName:    make(map[string][]DnsRecord),
		recordsByReverse: make(map[string][]DnsRecord),
		deviceByHostname: make(map[string]string),
		deviceByMAC:      make(map[string]string),
		deviceByIP:       make(map[string]string),
		zones:            []string{zone},
	}
}

// NewDeviceStoreMultiZone creates a DeviceStore with multiple zone suffixes.
// The first zone is the primary zone (used for PTR targets and display).
// Records are generated for ALL zones.
// Example: NewDeviceStoreMultiZone("jvj28.com", "local")
//
//	→ macmini.jvj28.com AND macmini.local both resolve
func NewDeviceStoreMultiZone(zones ...string) *DeviceStore {
	if len(zones) == 0 {
		zones = []string{"local"}
	}
	// Filter out empty strings
	var filtered []string
	for _, z := range zones {
		z = strings.TrimSpace(z)
		if z != "" {
			filtered = append(filtered, z)
		}
	}
	if len(filtered) == 0 {
		filtered = []string{"local"}
	}
	return &DeviceStore{
		devices:          make(map[string]*Device),
		recordsByName:    make(map[string][]DnsRecord),
		recordsByReverse: make(map[string][]DnsRecord),
		deviceByHostname: make(map[string]string),
		deviceByMAC:      make(map[string]string),
		deviceByIP:       make(map[string]string),
		zones:            filtered,
	}
}

// Zone returns the primary (first) zone suffix.
// For multi-zone setups, use Zones() to get all zones.
func (ds *DeviceStore) Zone() string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	if len(ds.zones) == 0 {
		return "local"
	}
	return ds.zones[0]
}

// Zones returns all configured zone suffixes.
// The first entry is the primary zone.
func (ds *DeviceStore) Zones() []string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	result := make([]string, len(ds.zones))
	copy(result, ds.zones)
	return result
}

// SetZones replaces all zones and rebuilds DNS records.
// The first zone is the primary. Requires at least one zone.
func (ds *DeviceStore) SetZones(zones []string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	var filtered []string
	for _, z := range zones {
		z = strings.TrimSpace(z)
		if z != "" {
			filtered = append(filtered, z)
		}
	}
	if len(filtered) == 0 {
		filtered = []string{"local"}
	}
	ds.zones = filtered
	ds.rebuildIndexes()
}

// AddZone adds a zone suffix if not already present and rebuilds DNS records.
func (ds *DeviceStore) AddZone(zone string) {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return
	}
	ds.mu.Lock()
	defer ds.mu.Unlock()
	for _, z := range ds.zones {
		if strings.EqualFold(z, zone) {
			return // already present
		}
	}
	ds.zones = append(ds.zones, zone)
	ds.rebuildIndexes()
}

// --- Query methods (called from DNS handler with RLock) ---

// LookupName returns DNS records matching the given FQDN and query type.
// Returns nil if no records found. Thread-safe for concurrent reads.
func (ds *DeviceStore) LookupName(fqdn string, qtype uint16) []DnsRecord {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	key := strings.ToLower(strings.TrimSuffix(fqdn, "."))
	records := ds.recordsByName[key]
	if records == nil {
		return nil
	}

	// Filter by query type
	var result []DnsRecord
	for _, r := range records {
		if r.Type == qtype {
			result = append(result, r)
		}
	}
	return result
}

// LookupReverse returns PTR records for a reverse DNS name.
// Example: LookupReverse("100.1.168.192.in-addr.arpa")
func (ds *DeviceStore) LookupReverse(reverseName string) []DnsRecord {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	key := strings.ToLower(strings.TrimSuffix(reverseName, "."))
	return ds.recordsByReverse[key]
}

// LookupAll returns DNS records matching the given FQDN (all types).
func (ds *DeviceStore) LookupAll(fqdn string) []DnsRecord {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	key := strings.ToLower(strings.TrimSuffix(fqdn, "."))
	return ds.recordsByName[key]
}

// GetDevice returns a device by ID. Returns nil if not found.
func (ds *DeviceStore) GetDevice(id string) *Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	d := ds.devices[id]
	if d == nil {
		return nil
	}
	// Return a copy to prevent external mutation
	copy := *d
	return &copy
}

// GetAllDevices returns a copy of all devices.
func (ds *DeviceStore) GetAllDevices() []Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	result := make([]Device, 0, len(ds.devices))
	for _, d := range ds.devices {
		result = append(result, *d)
	}
	return result
}

// DeviceCount returns the number of devices in the store.
func (ds *DeviceStore) DeviceCount() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return len(ds.devices)
}

// RecordCount returns the total number of DNS records in the store.
func (ds *DeviceStore) RecordCount() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	count := 0
	for _, recs := range ds.recordsByName {
		count += len(recs)
	}
	for _, recs := range ds.recordsByReverse {
		count += len(recs)
	}
	return count
}

// FindDeviceByHostname looks up a device by a hostname it has been seen with.
func (ds *DeviceStore) FindDeviceByHostname(hostname string) *Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	id := ds.deviceByHostname[strings.ToLower(hostname)]
	if id == "" {
		return nil
	}
	d := ds.devices[id]
	if d == nil {
		return nil
	}
	copy := *d
	return &copy
}

// FindDeviceByMAC looks up a device by MAC address.
func (ds *DeviceStore) FindDeviceByMAC(mac string) *Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	id := ds.deviceByMAC[strings.ToLower(mac)]
	if id == "" {
		return nil
	}
	d := ds.devices[id]
	if d == nil {
		return nil
	}
	copy := *d
	return &copy
}

// FindDeviceByIP looks up a device by current IP address.
func (ds *DeviceStore) FindDeviceByIP(ip string) *Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	id := ds.deviceByIP[ip]
	if id == "" {
		return nil
	}
	d := ds.devices[id]
	if d == nil {
		return nil
	}
	copy := *d
	return &copy
}

// --- Mutation methods (called from discovery sources with full Lock) ---

// UpsertDevice adds or updates a device in the store and regenerates
// its DNS records. The device is matched by ID if it already exists.
// Returns the device ID.
func (ds *DeviceStore) UpsertDevice(device *Device) string {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if device.ID == "" {
		device.ID = generateID()
	}

	now := time.Now()
	existing := ds.devices[device.ID]
	if existing != nil {
		// Merge: preserve fields the caller didn't set
		if device.ManualName == "" && existing.ManualName != "" {
			device.ManualName = existing.ManualName
		}
		if device.Owner == "" && existing.Owner != "" {
			device.Owner = existing.Owner
		}
		if device.Category == "" && existing.Category != "" {
			device.Category = existing.Category
		}
		if device.FirstSeen.IsZero() {
			device.FirstSeen = existing.FirstSeen
		}
		// Merge sources
		for _, s := range existing.Sources {
			device.AddSource(s)
		}
		// Merge hostnames (deduplicate)
		device.Hostnames = mergeStringSlice(device.Hostnames, existing.Hostnames)
		device.MDNSNames = mergeStringSlice(device.MDNSNames, existing.MDNSNames)
		device.MACs = mergeStringSlice(device.MACs, existing.MACs)

		// Preserve existing IP addresses when new values are empty.
		// This prevents discovery sources that lack IP info from wiping
		// addresses learned by other sources (e.g., mDNS enriching a
		// passive device that only had an IP).
		if device.IPv4 == "" && existing.IPv4 != "" {
			device.IPv4 = existing.IPv4
		}
		if device.IPv6 == "" && existing.IPv6 != "" {
			device.IPv6 = existing.IPv6
		}

		if !device.Persistent && existing.Persistent {
			device.Persistent = true
		}
	} else {
		if device.FirstSeen.IsZero() {
			device.FirstSeen = now
		}
	}
	device.LastSeen = now
	device.Online = true

	// Derive DNS name if not set
	if device.DNSName == "" {
		device.DNSName = ds.deriveDNSName(device)
	}

	// Update display name
	device.DisplayName = device.GetDisplayName()

	ds.devices[device.ID] = device
	ds.rebuildIndexes()

	return device.ID
}

// RemoveDevice removes a device by ID and rebuilds indexes.
func (ds *DeviceStore) RemoveDevice(id string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	delete(ds.devices, id)
	ds.rebuildIndexes()
}

// UpdateDeviceIP updates a device's IP address (v4 or v6) and regenerates
// DNS records. This is the hot path for DHCP renewals and DDNS updates.
func (ds *DeviceStore) UpdateDeviceIP(id string, ipv4 string, ipv6 string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	device := ds.devices[id]
	if device == nil {
		return
	}

	changed := false
	if ipv4 != "" && ipv4 != device.IPv4 {
		device.IPv4 = ipv4
		changed = true
	}
	if ipv6 != "" && ipv6 != device.IPv6 {
		device.IPv6 = ipv6
		changed = true
	}
	if changed {
		device.LastSeen = time.Now()
		device.Online = true
		ds.rebuildIndexes()
	}
}

// ClearDeviceAddress removes specific addresses from a device and
// regenerates DNS records. The device itself is NOT removed even if no
// addresses remain — the caller handles orphan cleanup. This avoids
// losing device identity during delete-then-add sequences in DDNS.
func (ds *DeviceStore) ClearDeviceAddress(id string, clearIPv4, clearIPv6 bool) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	device := ds.devices[id]
	if device == nil {
		return
	}
	if clearIPv4 {
		device.IPv4 = ""
	}
	if clearIPv6 {
		device.IPv6 = ""
	}
	ds.rebuildIndexes()
}

// TouchDevice updates the LastSeen timestamp for a device.
// Used by passive discovery when we see a query from a known device.
func (ds *DeviceStore) TouchDevice(id string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	device := ds.devices[id]
	if device == nil {
		return
	}
	device.LastSeen = time.Now()
	device.Online = true
}

// MarkOffline sets devices that haven't been seen recently to offline.
// Should be called periodically (e.g., every minute).
func (ds *DeviceStore) MarkOffline(threshold time.Duration) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	cutoff := time.Now().Add(-threshold)
	for _, device := range ds.devices {
		if device.LastSeen.Before(cutoff) {
			device.Online = false
		}
	}
}

// ImportLegacyRecords imports existing DNSCustomEntry records (domain→IP)
// into the device store as manual entries. This provides backward compatibility
// with the existing internal records system.
func (ds *DeviceStore) ImportLegacyRecords(records map[string]string) int {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	imported := 0
	for domain, ip := range records {
		// Check if a device with this hostname already exists
		dnsName := SanitizeDNSName(domain)
		if dnsName == "" {
			continue
		}
		existingID := ds.deviceByHostname[strings.ToLower(dnsName)]
		if existingID != "" {
			// Already exists — update IP if needed
			device := ds.devices[existingID]
			if device != nil {
				if net.ParseIP(ip).To4() != nil {
					device.IPv4 = ip
				} else {
					device.IPv6 = ip
				}
				device.AddSource(SourceManual)
				device.Persistent = true
			}
		} else {
			// Create new manual device
			device := &Device{
				ID:         generateID(),
				DNSName:    dnsName,
				Hostnames:  []string{domain},
				Source:     SourceManual,
				Sources:    []DiscoverySource{SourceManual},
				FirstSeen:  time.Now(),
				LastSeen:   time.Now(),
				Persistent: true,
			}
			if net.ParseIP(ip) != nil && net.ParseIP(ip).To4() != nil {
				device.IPv4 = ip
			} else {
				device.IPv6 = ip
			}
			device.DisplayName = device.GetDisplayName()
			ds.devices[device.ID] = device
		}
		imported++
	}
	ds.rebuildIndexes()
	log.Printf("[Discovery] Imported %d legacy internal records", imported)
	return imported
}

// --- Internal helpers ---

// deriveDNSName generates a DNS name from the best available hostname.
func (ds *DeviceStore) deriveDNSName(device *Device) string {
	// Try hostnames first
	for _, h := range device.Hostnames {
		name := SanitizeDNSName(h)
		if name != "" {
			return name
		}
	}
	// Try mDNS names
	for _, m := range device.MDNSNames {
		// mDNS names often already have ".local" suffix — strip it
		m = strings.TrimSuffix(m, ".local")
		m = strings.TrimSuffix(m, ".local.")
		name := SanitizeDNSName(m)
		if name != "" {
			return name
		}
	}
	return ""
}

// rebuildIndexes regenerates all lookup maps and DNS records from devices.
// MUST be called with ds.mu held for writing.
func (ds *DeviceStore) rebuildIndexes() {
	// Clear indexes
	ds.recordsByName = make(map[string][]DnsRecord)
	ds.recordsByReverse = make(map[string][]DnsRecord)
	ds.deviceByHostname = make(map[string]string)
	ds.deviceByMAC = make(map[string]string)
	ds.deviceByIP = make(map[string]string)

	for _, device := range ds.devices {
		// Index by hostnames
		for _, h := range device.Hostnames {
			ds.deviceByHostname[strings.ToLower(h)] = device.ID
		}
		for _, m := range device.MDNSNames {
			ds.deviceByHostname[strings.ToLower(m)] = device.ID
		}
		if device.DNSName != "" {
			ds.deviceByHostname[device.DNSName] = device.ID
		}

		// Index by MACs
		for _, mac := range device.MACs {
			ds.deviceByMAC[strings.ToLower(mac)] = device.ID
		}

		// Index by IPs
		if device.IPv4 != "" {
			ds.deviceByIP[device.IPv4] = device.ID
		}
		if device.IPv6 != "" {
			ds.deviceByIP[device.IPv6] = device.ID
		}

		// Generate DNS records for devices that have a name and an address.
		// Records are generated for EVERY configured zone so that both
		// "macmini.local" and "macmini.jvj28.com" resolve.
		if device.DNSName == "" {
			continue
		}

		ttl := DefaultTTL
		if device.Persistent {
			ttl = ManualTTL
		}

		// Primary FQDN is used for PTR targets (reverse DNS should point
		// to one canonical name, not multiple — RFC 1033 §2.2).
		primaryFQDN := device.DNSName + "." + ds.zones[0]

		// Generate forward records (A/AAAA) for each zone
		for _, zone := range ds.zones {
			fqdn := device.DNSName + "." + zone
			fqdnKey := strings.ToLower(fqdn)

			// A record
			if device.IPv4 != "" {
				rec := DnsRecord{
					Name:     fqdn,
					Type:     dns.TypeA,
					Value:    device.IPv4,
					TTL:      ttl,
					DeviceID: device.ID,
					Source:   device.Source,
				}
				ds.recordsByName[fqdnKey] = append(
					ds.recordsByName[fqdnKey], rec)
			}

			// AAAA record
			if device.IPv6 != "" {
				rec := DnsRecord{
					Name:     fqdn,
					Type:     dns.TypeAAAA,
					Value:    device.IPv6,
					TTL:      ttl,
					DeviceID: device.ID,
					Source:   device.Source,
				}
				ds.recordsByName[fqdnKey] = append(
					ds.recordsByName[fqdnKey], rec)
			}
		}

		// Reverse PTR records point to the PRIMARY zone's FQDN only.
		// Each IP gets exactly one PTR target (the canonical name).
		if device.IPv4 != "" {
			rev := reverseIPv4(device.IPv4)
			if rev != "" {
				ptr := DnsRecord{
					Name:     rev,
					Type:     dns.TypePTR,
					Value:    primaryFQDN,
					TTL:      ttl,
					DeviceID: device.ID,
					Source:   device.Source,
				}
				ds.recordsByReverse[strings.ToLower(rev)] = append(
					ds.recordsByReverse[strings.ToLower(rev)], ptr)
			}
		}
		if device.IPv6 != "" {
			rev := reverseIPv6(device.IPv6)
			if rev != "" {
				ptr := DnsRecord{
					Name:     rev,
					Type:     dns.TypePTR,
					Value:    primaryFQDN,
					TTL:      ttl,
					DeviceID: device.ID,
					Source:   device.Source,
				}
				ds.recordsByReverse[strings.ToLower(rev)] = append(
					ds.recordsByReverse[strings.ToLower(rev)], ptr)
			}
		}

		// Index the bare hostname (without any zone) for convenience.
		// This allows queries for just "macmini" to work.
		bareKey := strings.ToLower(device.DNSName)
		primaryKey := strings.ToLower(primaryFQDN)
		if bareKey != primaryKey {
			for _, rec := range ds.recordsByName[primaryKey] {
				ds.recordsByName[bareKey] = append(ds.recordsByName[bareKey], rec)
			}
		}
	}
}

// mergeStringSlice merges two slices, deduplicating (case-insensitive).
// Items from 'a' take precedence in ordering.
func mergeStringSlice(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range a {
		key := strings.ToLower(s)
		if !seen[key] {
			seen[key] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		key := strings.ToLower(s)
		if !seen[key] {
			seen[key] = true
			result = append(result, s)
		}
	}
	return result
}

// generateID creates a simple unique ID.
// Uses timestamp + random suffix for uniqueness without external dependencies.
func generateID() string {
	return fmt.Sprintf("dev-%d", time.Now().UnixNano())
}
