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
	"_http._tcp",
	"_https._tcp",
	"_airplay._tcp",
	"_raop._tcp",
	"_googlecast._tcp",
	"_printer._tcp",
	"_ipp._tcp",
	"_ipps._tcp",
	"_pdl-datastream._tcp",
	"_scanner._tcp",
	"_smb._tcp",
	"_afpovertcp._tcp",
	"_nfs._tcp",
	"_ssh._tcp",
	"_sftp-ssh._tcp",
	"_rfb._tcp",
	"_companion-link._tcp",
	"_homekit._tcp",
	"_hap._tcp",
	"_sleep-proxy._udp",
	"_spotify-connect._tcp",
	"_sonos._tcp",
	"_daap._tcp",
	"_touch-able._tcp",
	"_workstation._tcp",
	"_device-info._tcp",
	"_udisks-ssh._tcp",
}

// DefaultScanInterval is the default time between full mDNS scan cycles.
const DefaultScanInterval = 60 * time.Second

// DefaultBrowseTimeout is how long to wait for mDNS responses per service type.
// mDNS responses on a LAN are nearly instant; 5 seconds is generous.
const DefaultBrowseTimeout = 5 * time.Second

// MDNSBrowser performs periodic mDNS/Bonjour service discovery on the
// local network and feeds discovered devices into the DeviceStore.
type MDNSBrowser struct {
	store                *DeviceStore
	interval             time.Duration
	browseTimeout        time.Duration
	serviceTypes         []string
	localAddressProvider func() []net.IP
	browseProvider       func(string, time.Duration) []*bonjour.ServiceEntry

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
	browser := &MDNSBrowser{
		store:                store,
		interval:             interval,
		browseTimeout:        DefaultBrowseTimeout,
		serviceTypes:         DefaultServiceTypes,
		localAddressProvider: localInterfaceAddresses,
	}
	browser.browseProvider = browser.browseServiceType
	return browser
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

		entries := b.browseProvider(svcType, browseTimeout)
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

	// Buffered channel prevents resolver from blocking when we stop reading after timeout
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

func (b *MDNSBrowser) processEntry(entry *bonjour.ServiceEntry) {
	if entry == nil || b.isSelfGateSentryEntry(entry) {
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

	// A service name without an address cannot identify a usable network
	// device. It may arrive as a partial mDNS response and must not create an
	// addressless row that later restores as Unknown.
	if ipv4 == "" && ipv6 == "" {
		return
	}

	device := Device{
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
	if ipv4 != "" {
		if mac := LookupARPEntry(ipv4); mac != "" {
			device.MACs = []string{mac}
		}
	}

	deviceID, created, err := b.store.ObserveDevice(device)
	if err != nil {
		log.Printf("[mDNS] Ignored ambiguous or invalid service observation: %v", err)
		return
	}
	if created {
		log.Printf("[mDNS] New device: %q (%s) at %s/%s [%s]", instanceName, hostname, ipv4, ipv6, entry.Service)
	} else {
		log.Printf("[mDNS] Enriched device %s: %q (%s) [%s]", deviceID, instanceName, hostname, entry.Service)
	}
}

func (b *MDNSBrowser) isSelfGateSentryEntry(entry *bonjour.ServiceEntry) bool {
	marker := false
	for _, text := range entry.Text {
		if strings.EqualFold(strings.TrimSpace(text), "app=gatesentry") {
			marker = true
			break
		}
	}
	if !marker {
		return false
	}
	addresses := b.localAddressProvider()
	for _, advertised := range []net.IP{entry.AddrIPv4, entry.AddrIPv6} {
		if advertised == nil {
			continue
		}
		for _, local := range addresses {
			if local.Equal(advertised) {
				return true
			}
		}
	}
	return false
}

func localInterfaceAddresses() []net.IP {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	result := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		var value string
		switch typed := address.(type) {
		case *net.IPNet:
			value = typed.IP.String()
		case *net.IPAddr:
			value = typed.IP.String()
		}
		if ip := net.ParseIP(value); ip != nil {
			result = append(result, ip)
		}
	}
	return result
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

func IsLinkLocalIPv6(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLinkLocalUnicast()
}
