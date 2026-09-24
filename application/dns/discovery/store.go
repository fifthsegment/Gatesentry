package discovery

import (
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// SanitizeDNSName normalizes a hostname into a valid DNS label.
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

func reverseIPv4(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ""
	}
	return fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa",
		parts[3], parts[2], parts[1], parts[0])
}

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

// DeviceStore is a thread-safe store for discovered devices and DNS records.
type DeviceStore struct {
	mu sync.RWMutex
	// mutationMu serializes mutations that may change durable assignment state.
	// The device lock is deliberately released during storage I/O; serializing
	// here prevents an older durable snapshot from overtaking a newer link,
	// unlink, rename, or removal.
	mutationMu sync.Mutex
	// persistence stores the durable user-managed assignment subset when
	// attached. It is nil until AttachPersistence succeeds.
	persistence *Persistence

	// devices maps device ID → Device
	devices map[string]*Device

	// --- Lookup indexes (derived, rebuilt on mutation) ---

	// recordsByName maps lowercase FQDN → []DnsRecord for fast query answering.
	// Example key: "macmini.local"
	recordsByName map[string][]DnsRecord

	// recordsByReverse maps reverse PTR name → []DnsRecord.
	recordsByReverse map[string][]DnsRecord

	deviceByHostname      map[string]string
	deviceByMAC           map[string]string
	deviceByIP            map[string]string
	deviceByIPClaims      map[string][]string
	deviceByTailscaleNode map[string]string
	zones                 []string
}

// NewDeviceStore creates an empty DeviceStore with the given zone suffix.
// For backward compatibility, accepts a single zone string. Use SetZones()
// to configure multiple zones after creation.
func NewDeviceStore(zone string) *DeviceStore {
	if zone == "" {
		zone = "local"
	}
	return &DeviceStore{
		devices:               make(map[string]*Device),
		recordsByName:         make(map[string][]DnsRecord),
		recordsByReverse:      make(map[string][]DnsRecord),
		deviceByHostname:      make(map[string]string),
		deviceByMAC:           make(map[string]string),
		deviceByIP:            make(map[string]string),
		deviceByIPClaims:      make(map[string][]string),
		deviceByTailscaleNode: make(map[string]string),
		zones:                 []string{zone},
	}
}

// NewDeviceStoreMultiZone creates a DeviceStore with multiple zone suffixes.
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
		devices:               make(map[string]*Device),
		recordsByName:         make(map[string][]DnsRecord),
		recordsByReverse:      make(map[string][]DnsRecord),
		deviceByHostname:      make(map[string]string),
		deviceByMAC:           make(map[string]string),
		deviceByIP:            make(map[string]string),
		deviceByIPClaims:      make(map[string][]string),
		deviceByTailscaleNode: make(map[string]string),
		zones:                 filtered,
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

func cloneDevice(device *Device) *Device {
	if device == nil {
		return nil
	}
	copy := *device
	copy.Hostnames = append([]string(nil), device.Hostnames...)
	copy.MDNSNames = append([]string(nil), device.MDNSNames...)
	copy.MACs = append([]string(nil), device.MACs...)
	copy.Sources = append([]DiscoverySource(nil), device.Sources...)
	copy.TailscaleNodes = make([]TailscaleIdentity, len(device.TailscaleNodes))
	for i, identity := range device.TailscaleNodes {
		copy.TailscaleNodes[i] = identity
		copy.TailscaleNodes[i].Addresses = append([]string(nil), identity.Addresses...)
		copy.TailscaleNodes[i].WoLMACs = append([]string(nil), identity.WoLMACs...)
	}
	return &copy
}

// GetDevice returns a device by ID. Returns nil if not found.
func (ds *DeviceStore) GetDevice(id string) *Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	d := ds.devices[id]
	if d == nil {
		return nil
	}
	return cloneDevice(d)
}

// GetAllDevices returns a copy of all devices.
func (ds *DeviceStore) GetAllDevices() []Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	result := make([]Device, 0, len(ds.devices))
	for _, d := range ds.devices {
		result = append(result, *cloneDevice(d))
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
	return cloneDevice(d)
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
	return cloneDevice(d)
}

// FindDeviceByIP looks up a device by current IP address.
func (ds *DeviceStore) FindDeviceByIP(ip string) *Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	id := ds.deviceByIP[normalizeIP(ip)]
	if id == "" {
		return nil
	}
	d := ds.devices[id]
	if d == nil {
		return nil
	}
	return cloneDevice(d)
}

// FindDeviceByLANIP returns a device only when the address is an unambiguous
// current LAN claim. Overlay aliases are not evidence for DDNS/LAN enrichment.
func (ds *DeviceStore) FindDeviceByLANIP(ip string) *Device {
	ip = normalizeIP(ip)
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	var match *Device
	for _, id := range ds.deviceByIPClaims[ip] {
		device := ds.devices[id]
		if !deviceHasLANIP(device, ip) {
			continue
		}
		if match != nil && match.ID != id {
			return nil
		}
		match = device
	}
	return cloneDevice(match)
}

// LastSeenForIP returns freshness for the specific address claim. LAN
// observations and Tailscale aliases have independent lifecycles: activity on
// one must not extend policy ownership of the other.
func (ds *DeviceStore) LastSeenForIP(ip string) time.Time {
	ip = normalizeIP(ip)
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	id := ds.deviceByIP[ip]
	device := ds.devices[id]
	if device == nil {
		return time.Time{}
	}
	var latest time.Time
	if deviceHasLANIP(device, ip) {
		latest = device.LANLastSeen
		// Compatibility for records inserted directly by older callers/tests.
		if latest.IsZero() {
			latest = device.LastSeen
		}
	}
	for _, identity := range device.TailscaleNodes {
		for _, address := range identity.Addresses {
			if normalizeIP(address) == ip && identity.LastSeen.After(latest) {
				latest = identity.LastSeen
			}
		}
	}
	return latest
}

// IPClaimCount returns how many canonical devices currently claim an address.
func (ds *DeviceStore) IPClaimCount(ip string) int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return len(ds.deviceByIPClaims[normalizeIP(ip)])
}

// ResolveIPClaim returns one consistent view of address ownership for policy
// resolution. A sole linked overlay alias is durable identity and never expires;
// LAN claims retain the ordinary observation freshness timestamp.
func (ds *DeviceStore) ResolveIPClaim(ip string) (deviceID string, ambiguous bool, lastSeen time.Time, tailscaleAlias bool) {
	ip = normalizeIP(ip)
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	claims := ds.deviceByIPClaims[ip]
	ambiguous = len(claims) > 1
	deviceID = ds.deviceByIP[ip]
	device := ds.devices[deviceID]
	if device == nil {
		return "", ambiguous, time.Time{}, false
	}
	if deviceHasLANIP(device, ip) {
		lastSeen = device.LANLastSeen
		if lastSeen.IsZero() {
			lastSeen = device.LastSeen
		}
	}
	for _, identity := range device.TailscaleNodes {
		for _, address := range identity.Addresses {
			if normalizeIP(address) != ip {
				continue
			}
			tailscaleAlias = true
			if identity.LastSeen.After(lastSeen) {
				lastSeen = identity.LastSeen
			}
		}
	}
	return deviceID, ambiguous, lastSeen, tailscaleAlias
}

// IPClaimDeviceIDs returns every canonical device currently claiming an address
// in deterministic order. Callers use this to reject identity changes that
// would discard policy-bearing records rather than choosing a winner silently.
func (ds *DeviceStore) IPClaimDeviceIDs(ip string) []string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	ids := append([]string(nil), ds.deviceByIPClaims[normalizeIP(ip)]...)
	sort.Strings(ids)
	return ids
}

// FindDeviceByTailscaleNode looks up an explicitly linked stable node ID.
func (ds *DeviceStore) FindDeviceByTailscaleNode(nodeID string) *Device {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	id := ds.deviceByTailscaleNode[normalizeNodeID(nodeID)]
	return cloneDevice(ds.devices[id])
}

// --- Mutation methods (called from discovery sources with full Lock) ---

// ErrAmbiguousObservation means more than one canonical device claims the
// supplied strong identity. Discovery must not choose a winner silently.
var ErrAmbiguousObservation = errors.New("device observation is ambiguous")

// ErrTailscaleIdentityConflict means a stable node ID is already linked to a
// different canonical device or its current address belongs to durable state.
var ErrTailscaleIdentityConflict = errors.New("tailscale identity conflicts with another device")

// ObserveDevice atomically associates an observation by strong identity and
// merges or creates its canonical record. It replaces discovery's racy
// find-then-upsert sequence.
func (ds *DeviceStore) ObserveDevice(observation Device) (string, bool, error) {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	observation.IPv4 = normalizeIP(observation.IPv4)
	observation.IPv6 = normalizeIP(observation.IPv6)
	if observation.IPv4 == "" && observation.IPv6 == "" && len(observation.TailscaleNodes) == 0 {
		return "", false, errors.New("device observation needs a valid address or linked identity")
	}
	var macs []string
	for _, value := range observation.MACs {
		if mac := normalizeMAC(value); mac != "" {
			macs = append(macs, mac)
		}
	}
	observation.MACs = mergeStringSlice(macs, nil)

	ds.mu.Lock()
	candidateIDs := make(map[string]struct{})
	for _, mac := range observation.MACs {
		for id, device := range ds.devices {
			for _, known := range device.MACs {
				if normalizeMAC(known) == mac {
					candidateIDs[id] = struct{}{}
				}
			}
		}
	}
	// A unique MAC is authoritative. Otherwise only an unambiguous LAN
	// address can associate an observation. A passive query from a linked
	// Tailscale alias confirms traffic but is not evidence that the overlay
	// address became the device's LAN address.
	if len(candidateIDs) == 0 {
		addressClaimIDs := make(map[string]struct{})
		for _, address := range []string{observation.IPv4, observation.IPv6} {
			for _, id := range ds.deviceByIPClaims[address] {
				addressClaimIDs[id] = struct{}{}
				if deviceHasLANIP(ds.devices[id], address) {
					candidateIDs[id] = struct{}{}
				}
			}
		}
		if len(addressClaimIDs) > 1 {
			ds.mu.Unlock()
			return "", false, ErrAmbiguousObservation
		}
		if len(candidateIDs) == 0 && len(addressClaimIDs) == 1 {
			// The sole claim is an overlay alias. Do not copy it into LAN
			// fields or create a duplicate passive inventory row.
			for id := range addressClaimIDs {
				ds.mu.Unlock()
				return id, false, nil
			}
		}
	}
	// Hostnames enrich a device only after a strong MAC or current-address
	// match. They are not identity keys: unrelated devices commonly reuse
	// defaults such as "android" or "printer".
	if len(candidateIDs) > 1 {
		ds.mu.Unlock()
		return "", false, ErrAmbiguousObservation
	}
	created := len(candidateIDs) == 0
	for id := range candidateIDs {
		observation.ID = id
		if existing := ds.devices[id]; existing != nil && observation.IPv6 != "" && existing.IPv6 != "" &&
			IsLinkLocalIPv6(observation.IPv6) && !IsLinkLocalIPv6(existing.IPv6) {
			observation.IPv6 = existing.IPv6
		}
	}
	stored := ds.prepareUpsertDeviceLocked(&observation, time.Now())
	persistence := ds.persistence
	if persistence == nil {
		ds.commitUpsertDeviceLocked(stored)
		ds.mu.Unlock()
		return stored.ID, created, nil
	}
	ds.mu.Unlock()
	if err := persistence.persistIfChanged(stored); err != nil {
		return stored.ID, created, err
	}

	ds.mu.Lock()
	ds.commitUpsertDeviceLocked(stored)
	ds.mu.Unlock()
	return stored.ID, created, nil
}

// LinkTailscaleIdentityE attaches one stable tailscaled node to an existing
// canonical device. Throwaway observations of the peer address are removed;
// durable/user-managed conflicts are rejected.
func (ds *DeviceStore) LinkTailscaleIdentityE(deviceID string, identity TailscaleIdentity) (*Device, error) {
	return ds.LinkTailscaleIdentityProtectedE(deviceID, identity, nil)
}

// LinkTailscaleIdentityProtectedE is LinkTailscaleIdentityE with an additional
// set of canonical IDs that must never be removed during transient-alias
// cleanup because another durable subsystem still references them.
func (ds *DeviceStore) LinkTailscaleIdentityProtectedE(deviceID string, identity TailscaleIdentity, protectedIDs map[string]bool) (*Device, error) {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	identity.NodeID = normalizeNodeID(identity.NodeID)
	if identity.NodeID == "" {
		return nil, errors.New("tailscale node id is required")
	}
	var addresses []string
	for _, value := range identity.Addresses {
		if ip := normalizeIP(value); ip != "" {
			addresses = append(addresses, ip)
		}
	}
	identity.Addresses = mergeStringSlice(addresses, nil)
	if identity.Online && identity.LastSeen.IsZero() {
		identity.LastSeen = time.Now()
	}

	ds.mu.Lock()
	device := ds.devices[deviceID]
	if device == nil {
		ds.mu.Unlock()
		return nil, fmt.Errorf("device %q not found", deviceID)
	}
	if owner := ds.deviceByTailscaleNode[identity.NodeID]; owner != "" && owner != deviceID {
		ds.mu.Unlock()
		return nil, ErrTailscaleIdentityConflict
	}
	for _, address := range identity.Addresses {
		for _, owner := range append([]string(nil), ds.deviceByIPClaims[address]...) {
			if owner == deviceID {
				continue
			}
			other := ds.devices[owner]
			if other == nil {
				continue
			}
			if isDurableDevice(other) || protectedIDs[owner] {
				ds.mu.Unlock()
				return nil, ErrTailscaleIdentityConflict
			}
		}
	}
	updated := *cloneDevice(device)
	updated.TailscaleNodes = mergeTailscaleIdentities([]TailscaleIdentity{identity}, updated.TailscaleNodes)
	updated.AddSource(SourceTailscale)
	if updated.Source == "" {
		updated.Source = SourceTailscale
	}
	updated.DisplayName = updated.GetDisplayName()
	result := *cloneDevice(&updated)
	persistence := ds.persistence
	ds.mu.Unlock()
	if persistence != nil {
		if err := persistence.persistIfChanged(result); err != nil {
			return nil, err
		}
	}

	ds.mu.Lock()
	ds.devices[deviceID] = &updated
	for _, address := range identity.Addresses {
		for _, owner := range append([]string(nil), ds.deviceByIPClaims[address]...) {
			if owner != deviceID && !isDurableDevice(ds.devices[owner]) && !protectedIDs[owner] {
				delete(ds.devices, owner)
			}
		}
	}
	ds.rebuildIndexes()
	ds.mu.Unlock()
	return cloneDevice(&result), nil
}

// UnlinkTailscaleIdentityE removes one durable link without deleting the LAN
// device or any policy keyed by its canonical ID.
func (ds *DeviceStore) UnlinkTailscaleIdentityE(deviceID, nodeID string) (*Device, error) {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	nodeID = normalizeNodeID(nodeID)
	ds.mu.Lock()
	device := ds.devices[deviceID]
	if device == nil {
		ds.mu.Unlock()
		return nil, fmt.Errorf("device %q not found", deviceID)
	}
	updated := *cloneDevice(device)
	found := false
	identities := updated.TailscaleNodes[:0]
	for _, identity := range updated.TailscaleNodes {
		if normalizeNodeID(identity.NodeID) == nodeID {
			found = true
			continue
		}
		identities = append(identities, identity)
	}
	if !found {
		ds.mu.Unlock()
		return nil, fmt.Errorf("tailscale node %q is not linked to device", nodeID)
	}
	updated.TailscaleNodes = identities
	updated.DisplayName = updated.GetDisplayName()
	result := *cloneDevice(&updated)
	persistence := ds.persistence
	ds.mu.Unlock()
	if persistence != nil {
		if err := persistence.persistIfChanged(result); err != nil {
			return nil, err
		}
	}

	ds.mu.Lock()
	ds.devices[deviceID] = &updated
	ds.rebuildIndexes()
	ds.mu.Unlock()
	return cloneDevice(&result), nil
}

// ApplyTailscaleSnapshot refreshes linked nodes without creating inventory for
// unlinked peers. Callers without policy state get the same fail-closed address
// conflict behavior, but cannot identify otherwise transient protected rows.
func (ds *DeviceStore) ApplyTailscaleSnapshot(peers []TailscaleIdentity) error {
	return ds.ApplyTailscaleSnapshotProtected(peers, nil)
}

// ApplyTailscaleSnapshotProtected refreshes linked nodes and reconciles only
// disposable address-only observations. Durable or externally referenced rows
// remain as ambiguous claims rather than being silently consolidated.
func (ds *DeviceStore) ApplyTailscaleSnapshotProtected(peers []TailscaleIdentity, protectedIDs map[string]bool) error {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	byID := make(map[string]TailscaleIdentity, len(peers))
	for _, peer := range peers {
		peer.NodeID = normalizeNodeID(peer.NodeID)
		if peer.NodeID == "" {
			continue
		}
		addresses := make([]string, 0, len(peer.Addresses))
		for _, value := range peer.Addresses {
			if address := normalizeIP(value); address != "" {
				addresses = append(addresses, address)
			}
		}
		peer.Addresses = mergeStringSlice(addresses, nil)
		byID[peer.NodeID] = peer
	}

	ds.mu.Lock()
	updatedDevices := make(map[string]*Device)
	var changed []Device
	for id, current := range ds.devices {
		if len(current.TailscaleNodes) == 0 {
			continue
		}
		updated := *cloneDevice(current)
		observedOnline := false
		previousTailscaleOnline := false
		previousTailscaleSeen := time.Time{}
		latestSeen := updated.LastSeen
		for _, linked := range updated.TailscaleNodes {
			if linked.Online {
				previousTailscaleOnline = true
			}
			if linked.LastSeen.After(previousTailscaleSeen) {
				previousTailscaleSeen = linked.LastSeen
			}
		}
		for i, linked := range updated.TailscaleNodes {
			peer, ok := byID[normalizeNodeID(linked.NodeID)]
			if !ok {
				updated.TailscaleNodes[i].Online = false
				continue
			}
			peer.NodeID = linked.NodeID
			if peer.Name == "" {
				peer.Name = linked.Name
			}
			if peer.DNSName == "" {
				peer.DNSName = linked.DNSName
			}
			if len(peer.Addresses) == 0 {
				peer.Addresses = append([]string(nil), linked.Addresses...)
			}
			if peer.Online {
				observedOnline = true
				if peer.LastSeen.IsZero() {
					peer.LastSeen = time.Now()
				}
				if peer.LastSeen.After(latestSeen) {
					latestSeen = peer.LastSeen
				}
			}
			updated.TailscaleNodes[i] = peer
		}
		if observedOnline {
			updated.Online = true
			updated.LastSeen = latestSeen
		} else if previousTailscaleOnline && !previousTailscaleSeen.IsZero() && !updated.LastSeen.After(previousTailscaleSeen) {
			updated.Online = false
		}
		updated.DisplayName = updated.GetDisplayName()
		updatedDevices[id] = cloneDevice(&updated)
		changed = append(changed, updated)
	}
	persistence := ds.persistence
	ds.mu.Unlock()
	if persistence != nil {
		if err := persistence.persistDevicesIfChanged(changed); err != nil {
			return err
		}
	}

	ds.mu.Lock()
	for id, device := range updatedDevices {
		ds.devices[id] = device
	}
	// Rebuild once so address churn no longer leaves old aliases in the claim
	// index before deciding which pre-existing rows are safe to consolidate.
	ds.rebuildIndexes()
	for id, device := range updatedDevices {
		for _, identity := range device.TailscaleNodes {
			for _, address := range identity.Addresses {
				for _, owner := range append([]string(nil), ds.deviceByIPClaims[normalizeIP(address)]...) {
					if owner == id || protectedIDs[owner] || !isDisposableTailscaleAliasObservation(ds.devices[owner], address) {
						continue
					}
					delete(ds.devices, owner)
				}
			}
		}
	}
	ds.rebuildIndexes()
	ds.mu.Unlock()
	return nil
}

// ClearTailscaleObservations removes runtime reachability while retaining
// explicit node links, labels, and their durable last-known overlay aliases.
func (ds *DeviceStore) ClearTailscaleObservations() {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	ds.mu.Lock()
	defer ds.mu.Unlock()
	for _, device := range ds.devices {
		previousTailscaleSeen := time.Time{}
		for i := range device.TailscaleNodes {
			if device.TailscaleNodes[i].LastSeen.After(previousTailscaleSeen) {
				previousTailscaleSeen = device.TailscaleNodes[i].LastSeen
			}
			device.TailscaleNodes[i].Online = false
		}
		if !previousTailscaleSeen.IsZero() && !device.LastSeen.After(previousTailscaleSeen) {
			device.Online = false
		}
	}
	ds.rebuildIndexes()
}

func isDurableDevice(device *Device) bool {
	return device.Persistent || device.ManualName != "" || device.Owner != "" || device.Category != "" || len(device.TailscaleNodes) > 0
}

func isDisposableTailscaleAliasObservation(device *Device, address string) bool {
	if device == nil || isDurableDevice(device) || len(device.MACs) > 0 || len(device.Hostnames) > 0 || len(device.MDNSNames) > 0 {
		return false
	}
	address = normalizeIP(address)
	if address == "" || !deviceHasLANIP(device, address) {
		return false
	}
	for _, claimed := range deviceAddresses(device) {
		if claimed != address {
			return false
		}
	}
	return device.Source == SourcePassive && len(device.Sources) <= 1
}

// UpsertDevice adds or updates a device in the store and regenerates
// its DNS records. The device is matched by ID if it already exists.
// Returns the device ID.
func (ds *DeviceStore) UpsertDevice(device *Device) string {
	// Compatibility path used by discovery sources: observed identity churn
	// is persisted only through the durable subset, and persistence errors
	// are logged rather than dropped.
	id, err := ds.UpsertDeviceE(device)
	if err != nil {
		log.Printf("[Discovery] unable to persist device assignment: %v", err)
	}
	return id
}

// UpsertDeviceE adds or updates a device and persists the durable assignment
// subset when persistence is attached. The method returns persistence errors;
// user-facing mutations must surface them instead of silently losing data.
func (ds *DeviceStore) UpsertDeviceE(device *Device) (string, error) {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	ds.mu.Lock()
	candidate := cloneDevice(device)
	stored := ds.prepareUpsertDeviceLocked(candidate, time.Now())
	persistence := ds.persistence
	if persistence == nil {
		ds.commitUpsertDeviceLocked(stored)
		ds.mu.Unlock()
		device.ID = stored.ID
		return stored.ID, nil
	}
	ds.mu.Unlock()
	if err := persistence.persistIfChanged(stored); err != nil {
		return stored.ID, err
	}

	ds.mu.Lock()
	ds.commitUpsertDeviceLocked(stored)
	ds.mu.Unlock()
	device.ID = stored.ID
	return stored.ID, nil
}

func (ds *DeviceStore) upsertDeviceLocked(device *Device, now time.Time) Device {
	stored := ds.prepareUpsertDeviceLocked(device, now)
	ds.commitUpsertDeviceLocked(stored)
	return stored
}

func (ds *DeviceStore) prepareUpsertDeviceLocked(device *Device, now time.Time) Device {
	if device.ID == "" {
		device.ID = generateID()
	}
	existing := ds.devices[device.ID]
	if existing != nil {
		if device.ManualName == "" {
			device.ManualName = existing.ManualName
		}
		if device.Owner == "" {
			device.Owner = existing.Owner
		}
		if device.Category == "" {
			device.Category = existing.Category
		}
		if device.FirstSeen.IsZero() {
			device.FirstSeen = existing.FirstSeen
		}
		for _, source := range existing.Sources {
			device.AddSource(source)
		}
		device.Hostnames = mergeStringSlice(device.Hostnames, existing.Hostnames)
		device.MDNSNames = mergeStringSlice(device.MDNSNames, existing.MDNSNames)
		device.MACs = mergeStringSlice(device.MACs, existing.MACs)
		device.TailscaleNodes = mergeTailscaleIdentities(device.TailscaleNodes, existing.TailscaleNodes)
		if device.IPv4 == "" {
			device.IPv4 = existing.IPv4
		}
		if device.IPv6 == "" {
			device.IPv6 = existing.IPv6
		}
		device.Persistent = device.Persistent || existing.Persistent
	} else if device.FirstSeen.IsZero() {
		device.FirstSeen = now
	}
	device.LastSeen = now
	device.LANLastSeen = now
	device.Online = true
	if device.DNSName == "" {
		device.DNSName = ds.deriveDNSName(device)
	}
	device.DisplayName = device.GetDisplayName()
	return *cloneDevice(device)
}

func (ds *DeviceStore) commitUpsertDeviceLocked(stored Device) {
	ds.devices[stored.ID] = cloneDevice(&stored)
	ds.rebuildIndexes()
}

// ManualFieldsUpdate changes user-managed device fields. A true Set* field
// writes the corresponding value, including an empty value, so the API can
// clear a label without changing discovery's merge behavior; a false Set*
// field leaves the current value untouched.
type ManualFieldsUpdate struct {
	ManualName    string
	Owner         string
	Category      string
	SetManualName bool
	SetOwner      bool
	SetCategory   bool
}

// MarkDevicePersistentE retains one canonical device across restarts without
// turning its current LAN address into durable identity. Policy assignment uses
// this before storing a device ID so the reference cannot outlive its target.
func (ds *DeviceStore) MarkDevicePersistentE(id string) (*Device, bool, error) {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	ds.mu.Lock()
	current := ds.devices[id]
	if current == nil {
		ds.mu.Unlock()
		return nil, false, nil
	}
	original := cloneDevice(current)
	updated := *cloneDevice(current)
	updated.Persistent = true
	persistence := ds.persistence
	if persistence == nil {
		ds.devices[id] = cloneDevice(&updated)
		ds.rebuildIndexes()
		ds.mu.Unlock()
		return cloneDevice(&updated), true, nil
	}
	ds.mu.Unlock()
	if err := persistence.persistIfChanged(updated); err != nil {
		return original, true, err
	}

	ds.mu.Lock()
	ds.devices[id] = cloneDevice(&updated)
	ds.rebuildIndexes()
	ds.mu.Unlock()
	return cloneDevice(&updated), true, nil
}

// UpdateManualFieldsE applies user-managed fields to one device. Discovery
// remains the owner of observed identity: this method never changes addresses,
// sources, last-seen, or online state. It returns the updated copy and any
// persistence error so user-facing callers can surface a failed durable write.
func (ds *DeviceStore) UpdateManualFieldsE(id string, update ManualFieldsUpdate) (*Device, bool, error) {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	ds.mu.Lock()
	current := ds.devices[id]
	if current == nil {
		ds.mu.Unlock()
		return nil, false, nil
	}
	original := cloneDevice(current)
	device := *original
	if update.SetManualName {
		device.ManualName = update.ManualName
	}
	if update.SetOwner {
		device.Owner = update.Owner
	}
	if update.SetCategory {
		device.Category = update.Category
	}
	device.Persistent = true
	device.DisplayName = device.GetDisplayName()
	updated := *cloneDevice(&device)
	persistence := ds.persistence
	if persistence == nil {
		ds.devices[id] = cloneDevice(&updated)
		ds.rebuildIndexes()
		ds.mu.Unlock()
		return cloneDevice(&updated), true, nil
	}
	ds.mu.Unlock()
	if err := persistence.persistIfChanged(updated); err != nil {
		return original, true, err
	}

	ds.mu.Lock()
	ds.devices[id] = cloneDevice(&updated)
	ds.rebuildIndexes()
	ds.mu.Unlock()
	return cloneDevice(&updated), true, nil
}

// RemoveDevice removes a device by ID and rebuilds indexes.
func (ds *DeviceStore) RemoveDevice(id string) {
	// Errors are logged by the error-returning variant; this compatibility
	// method must retain its existing signature for discovery callers.
	_ = ds.RemoveDeviceE(id)
}

// RemoveDeviceE removes a device and persists the removal when persistence is
// attached. It returns persistence errors so user-facing mutation paths can
// fail visibly instead of silently losing a durable assignment.
func (ds *DeviceStore) RemoveDeviceE(id string) error {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

	ds.mu.Lock()
	device := ds.devices[id]
	persistence := ds.persistence
	if device == nil {
		ds.mu.Unlock()
		return nil
	}
	if persistence == nil {
		delete(ds.devices, id)
		ds.rebuildIndexes()
		ds.mu.Unlock()
		return nil
	}
	ds.mu.Unlock()
	if err := persistence.remove(id); err != nil {
		return err
	}

	ds.mu.Lock()
	delete(ds.devices, id)
	ds.rebuildIndexes()
	ds.mu.Unlock()
	return nil
}

// UpdateDeviceIP updates a device's IP address (v4 or v6) and regenerates
// DNS records. This is the hot path for DHCP renewals and DDNS updates.
func (ds *DeviceStore) UpdateDeviceIP(id string, ipv4 string, ipv6 string) {
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

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
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

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
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

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
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

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
	ds.mutationMu.Lock()
	defer ds.mutationMu.Unlock()

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
	ds.deviceByIPClaims = make(map[string][]string)
	ds.deviceByTailscaleNode = make(map[string]string)

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
		for _, address := range deviceAddresses(device) {
			ds.deviceByIPClaims[address] = append(ds.deviceByIPClaims[address], device.ID)
			if current := ds.deviceByIP[address]; current == "" || device.ID < current {
				ds.deviceByIP[address] = device.ID
			}
		}
		for _, identity := range device.TailscaleNodes {
			if nodeID := normalizeNodeID(identity.NodeID); nodeID != "" {
				if current := ds.deviceByTailscaleNode[nodeID]; current == "" || device.ID < current {
					ds.deviceByTailscaleNode[nodeID] = device.ID
				}
			}
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

func normalizeIP(value string) string {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
		return ""
	}
	return ip.String()
}

func normalizeNodeID(value string) string {
	return strings.TrimSpace(value)
}

func normalizeMAC(value string) string {
	hardware, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil || len(hardware) != 6 {
		return ""
	}
	normalized := strings.ToLower(hardware.String())
	if normalized == "00:00:00:00:00:00" {
		return ""
	}
	return normalized
}

func deviceHasLANIP(device *Device, ip string) bool {
	if device == nil || ip == "" {
		return false
	}
	return normalizeIP(device.IPv4) == ip || normalizeIP(device.IPv6) == ip
}

func deviceAddresses(device *Device) []string {
	var result []string
	for _, value := range []string{device.IPv4, device.IPv6} {
		if ip := normalizeIP(value); ip != "" {
			result = append(result, ip)
		}
	}
	for _, identity := range device.TailscaleNodes {
		for _, value := range identity.Addresses {
			if ip := normalizeIP(value); ip != "" {
				result = append(result, ip)
			}
		}
	}
	return mergeStringSlice(result, nil)
}

func mergeTailscaleIdentities(primary, secondary []TailscaleIdentity) []TailscaleIdentity {
	byID := make(map[string]TailscaleIdentity, len(primary)+len(secondary))
	order := make([]string, 0, len(primary)+len(secondary))
	for _, source := range [][]TailscaleIdentity{secondary, primary} {
		for _, identity := range source {
			id := normalizeNodeID(identity.NodeID)
			if id == "" {
				continue
			}
			current, exists := byID[id]
			if !exists {
				order = append(order, id)
			}
			identity.NodeID = id
			if identity.Name == "" {
				identity.Name = current.Name
			}
			if identity.DNSName == "" {
				identity.DNSName = current.DNSName
			}
			identity.Addresses = mergeStringSlice(identity.Addresses, current.Addresses)
			identity.WoLMACs = mergeStringSlice(identity.WoLMACs, current.WoLMACs)
			if identity.LastSeen.IsZero() {
				identity.LastSeen = current.LastSeen
			}
			byID[id] = identity
		}
	}
	result := make([]TailscaleIdentity, 0, len(order))
	for _, id := range order {
		result = append(result, byID[id])
	}
	return result
}

func normalizeHostname(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ".local")
	return value
}

func deviceHasHostname(device *Device, hostname string) bool {
	if normalizeHostname(device.DNSName) == hostname {
		return true
	}
	for _, value := range append(append([]string(nil), device.Hostnames...), device.MDNSNames...) {
		if normalizeHostname(value) == hostname {
			return true
		}
	}
	return false
}
