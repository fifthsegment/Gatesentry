package discovery

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/oleksandr/bonjour"
)

// DefaultServiceTypes lists common mDNS/Bonjour service types to browse.
// These cover the vast majority of devices found on home networks:
// Apple devices, Chromecasts, printers, NAS boxes, smart speakers, etc.
var DefaultServiceTypes = []string{
	"_http._tcp",            // Web servers, management UIs, IoT devices
	"_https._tcp",           // Secure web servers
	"_airplay._tcp",         // Apple AirPlay (Apple TV, HomePod, AirPlay speakers)
	"_raop._tcp",            // Remote Audio Output Protocol (AirPlay audio)
	"_googlecast._tcp",      // Google Chromecast, Google Home, Nest Hub
	"_printer._tcp",         // Network printers (generic)
	"_ipp._tcp",             // Internet Printing Protocol
	"_ipps._tcp",            // IPP over TLS
	"_pdl-datastream._tcp",  // HP JetDirect / PCL printers
	"_scanner._tcp",         // Network scanners
	"_smb._tcp",             // SMB/CIFS file sharing (Windows, Samba, NAS)
	"_afpovertcp._tcp",      // Apple Filing Protocol (older Macs, Time Machine)
	"_nfs._tcp",             // NFS file sharing
	"_ssh._tcp",             // SSH servers (Linux boxes, NAS, routers)
	"_sftp-ssh._tcp",        // SFTP over SSH
	"_rfb._tcp",             // VNC remote desktop
	"_companion-link._tcp",  // Apple Companion Link (iOS ↔ Apple TV)
	"_homekit._tcp",         // Apple HomeKit accessories
	"_hap._tcp",             // HomeKit Accessory Protocol
	"_sleep-proxy._udp",     // Apple Sleep Proxy (Mac Mini, Apple TV)
	"_spotify-connect._tcp", // Spotify Connect devices
	"_sonos._tcp",           // Sonos speakers
	"_daap._tcp",            // Digital Audio Access Protocol (iTunes sharing)
	"_touch-able._tcp",      // Apple Remote (iOS Remote app)
	"_workstation._tcp",     // Workstation/computer discovery
	"_device-info._tcp",     // Device information service
	"_udisks-ssh._tcp",      // USB disk sharing over SSH
}

// DefaultScanInterval is the default time between full mDNS scan cycles.
const DefaultScanInterval = 60 * time.Second

// DefaultBrowseTimeout is how long to wait for mDNS responses per service type.
// mDNS responses on a LAN are nearly instant; 5 seconds is generous.
const DefaultBrowseTimeout = 5 * time.Second

// MDNSBrowser performs periodic mDNS/Bonjour service discovery on the
// local network and feeds discovered devices into the DeviceStore.
//
// It browses a configurable list of service types (e.g., _airplay._tcp,
// _googlecast._tcp, _printer._tcp) and for each discovered service entry:
//   - Correlates with existing devices by IP, hostname, or mDNS instance name
//   - Enriches existing devices (e.g., adding a name to a passive-only device)
//   - Creates new devices for previously unseen hosts
//
// The browser runs as a background goroutine started by Start() and stopped
// by Stop(). It performs an immediate scan on startup, then scans at the
// configured interval.
type MDNSBrowser struct {
	store         *DeviceStore
	interval      time.Duration
	browseTimeout time.Duration
	serviceTypes  []string

	stopCh  chan struct{}
	stopped chan struct{}
	mu      sync.Mutex
	running bool
}

// NewMDNSBrowser creates an mDNS browser that will populate the given store.
// If interval is <= 0, DefaultScanInterval is used.
func NewMDNSBrowser(store *DeviceStore, interval time.Duration) *MDNSBrowser {
	if interval <= 0 {
		interval = DefaultScanInterval
	}
	return &MDNSBrowser{
		store:         store,
		interval:      interval,
		browseTimeout: DefaultBrowseTimeout,
		serviceTypes:  DefaultServiceTypes,
	}
}

// SetServiceTypes overrides the default list of mDNS service types to browse.
func (b *MDNSBrowser) SetServiceTypes(types []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.serviceTypes = types
}

// SetBrowseTimeout sets the per-service-type browse timeout.
func (b *MDNSBrowser) SetBrowseTimeout(timeout time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.browseTimeout = timeout
}

// Start begins periodic mDNS scanning in a background goroutine.
// Calling Start on an already-running browser is a no-op.
func (b *MDNSBrowser) Start() {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.stopCh = make(chan struct{})
	b.stopped = make(chan struct{})
	b.running = true
	b.mu.Unlock()

	log.Printf("[mDNS] Browser started (interval: %s, browse timeout: %s/type, %d service types)",
		b.interval, b.browseTimeout, len(b.serviceTypes))

	go b.run()
}

// Stop signals the browser to stop and waits for it to finish.
// Calling Stop on an already-stopped browser is a no-op.
func (b *MDNSBrowser) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	close(b.stopCh)
	<-b.stopped

	b.mu.Lock()
	b.running = false
	b.mu.Unlock()

	log.Println("[mDNS] Browser stopped")
}

// IsRunning returns whether the browser is actively scanning.
func (b *MDNSBrowser) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// ScanNow triggers an immediate scan cycle. Safe to call while running.
// If the browser is not running, this is a no-op.
func (b *MDNSBrowser) ScanNow() {
	b.mu.Lock()
	running := b.running
	b.mu.Unlock()
	if running {
		go b.scanOnce()
	}
}

// run is the main loop that performs periodic scans.
func (b *MDNSBrowser) run() {
	defer close(b.stopped)

	// Run an immediate scan on startup so devices are discovered
	// without waiting for the first interval tick.
	b.scanOnce()

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.scanOnce()
		case <-b.stopCh:
			return
		}
	}
}

// scanOnce performs one full scan cycle across all configured service types.
func (b *MDNSBrowser) scanOnce() {
	b.mu.Lock()
	serviceTypes := make([]string, len(b.serviceTypes))
	copy(serviceTypes, b.serviceTypes)
	browseTimeout := b.browseTimeout
	b.mu.Unlock()

	totalEntries := 0
	for _, svcType := range serviceTypes {
		// Check for stop signal between service types to allow fast shutdown
		select {
		case <-b.stopCh:
			return
		default:
		}

		entries := b.browseServiceType(svcType, browseTimeout)
		for _, entry := range entries {
			b.processEntry(entry)
		}
		totalEntries += len(entries)
	}

	if totalEntries > 0 {
		log.Printf("[mDNS] Scan complete: discovered %d service entries across %d types",
			totalEntries, len(serviceTypes))
	}
}

// browseServiceType performs a single mDNS browse for one service type.
// Returns discovered service entries, or nil on error.
func (b *MDNSBrowser) browseServiceType(serviceType string, timeout time.Duration) []*bonjour.ServiceEntry {
	resolver, err := bonjour.NewResolver(nil)
	if err != nil {
		log.Printf("[mDNS] Failed to create resolver for %s: %v", serviceType, err)
		return nil
	}

	// Buffered channel prevents the resolver's mainloop from blocking
	// when we stop reading after timeout. Without this, the resolver
	// goroutine could deadlock trying to send an entry while we're
	// trying to send on the Exit channel.
	entries := make(chan *bonjour.ServiceEntry, 100)

	err = resolver.Browse(serviceType, "local.", entries)
	if err != nil {
		log.Printf("[mDNS] Failed to browse %s: %v", serviceType, err)
		resolver.Exit <- true
		return nil
	}

	var results []*bonjour.ServiceEntry
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case entry := <-entries:
			if entry != nil {
				results = append(results, entry)
			}
		case <-timer.C:
			resolver.Exit <- true
			return results
		case <-b.stopCh:
			resolver.Exit <- true
			return results
		}
	}
}

// processEntry takes a discovered mDNS service entry and upserts it into the
// device store, merging with any existing device matched by IP or hostname.
//
// Match priority:
//  1. Existing device by IPv4 (most common — passive discovery already created it)
//  2. Existing device by IPv6
//  3. Existing device by cleaned hostname
//  4. Existing device by mDNS instance name
//  5. Create new device
func (b *MDNSBrowser) processEntry(entry *bonjour.ServiceEntry) {
	if entry == nil {
		return
	}

	instanceName := strings.TrimSpace(entry.Instance)
	hostname := CleanMDNSHostname(entry.HostName)

	var ipv4, ipv6 string
	if entry.AddrIPv4 != nil && !entry.AddrIPv4.IsUnspecified() {
		ipv4 = entry.AddrIPv4.String()
	}
	if entry.AddrIPv6 != nil && !entry.AddrIPv6.IsUnspecified() {
		ipv6 = entry.AddrIPv6.String()
	}

	// Need at least an IP or hostname to create a meaningful device entry
	if ipv4 == "" && ipv6 == "" && hostname == "" {
		return
	}

	// Try to find an existing device to enrich
	var existing *Device
	if ipv4 != "" {
		existing = b.store.FindDeviceByIP(ipv4)
	}
	if existing == nil && ipv6 != "" {
		existing = b.store.FindDeviceByIP(ipv6)
	}
	if existing == nil && hostname != "" {
		existing = b.store.FindDeviceByHostname(hostname)
	}
	if existing == nil && instanceName != "" {
		existing = b.store.FindDeviceByHostname(instanceName)
	}

	// Build the device struct for upsert
	device := &Device{
		Source:  SourceMDNS,
		Sources: []DiscoverySource{SourceMDNS},
		IPv4:    ipv4,
		IPv6:    ipv6,
		Online:  true,
	}

	if instanceName != "" {
		device.MDNSNames = []string{instanceName}
	}
	if hostname != "" {
		device.Hostnames = []string{hostname}
	}

	// If enriching an existing device, set its ID so UpsertDevice merges
	if existing != nil {
		device.ID = existing.ID

		// Preserve existing IPs that mDNS didn't provide
		if device.IPv4 == "" && existing.IPv4 != "" {
			device.IPv4 = existing.IPv4
		}
		if device.IPv6 == "" && existing.IPv6 != "" {
			device.IPv6 = existing.IPv6
		}

		// Prefer GUA/ULA over link-local IPv6 — don't downgrade a better address
		if device.IPv6 != "" && existing.IPv6 != "" &&
			IsLinkLocalIPv6(device.IPv6) && !IsLinkLocalIPv6(existing.IPv6) {
			device.IPv6 = existing.IPv6
		}
	}

	// Attempt MAC lookup from ARP cache if we have an IPv4 address
	if device.IPv4 != "" {
		mac := LookupARPEntry(device.IPv4)
		if mac != "" {
			device.MACs = []string{mac}
		}
	}

	deviceID := b.store.UpsertDevice(device)

	if existing == nil {
		log.Printf("[mDNS] New device: %q (%s) at %s/%s [%s]",
			instanceName, hostname, ipv4, ipv6, entry.Service)
	} else {
		log.Printf("[mDNS] Enriched device %s: %q (%s) [%s]",
			deviceID, instanceName, hostname, entry.Service)
	}
}

// CleanMDNSHostname strips mDNS suffixes and trailing dots from a hostname.
//
// Examples:
//
//	"Viviennes-iPad.local." → "Viviennes-iPad"
//	"macmini.local"         → "macmini"
//	"printer."              → "printer"
//	"myhost"                → "myhost"
func CleanMDNSHostname(hostname string) string {
	h := strings.TrimSpace(hostname)
	h = strings.TrimSuffix(h, ".")      // Strip trailing FQDN dot
	h = strings.TrimSuffix(h, ".local") // Strip mDNS domain
	return h
}

// IsLinkLocalIPv6 returns true if the IP is an IPv6 link-local address (fe80::/10).
// Link-local addresses are valid for on-link communication but less useful for
// DNS resolution since they require a zone ID (scope) to be routable.
func IsLinkLocalIPv6(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLinkLocalUnicast()
}
