package discovery

import (
	"bufio"
	"errors"
	"log"
	"net"
	"os"
	"strings"
)

func (ds *DeviceStore) ObservePassiveQuery(clientIP string) {
	ip := normalizeIP(clientIP)
	if ip == "" {
		return
	}
	device := Device{
		Source:  SourcePassive,
		Sources: []DiscoverySource{SourcePassive},
		Online:  true,
	}
	if net.ParseIP(ip).To4() != nil {
		device.IPv4 = ip
		if mac := LookupARPEntry(ip); mac != "" {
			device.MACs = []string{mac}
		}
	} else {
		device.IPv6 = ip
	}
	id, created, err := ds.ObserveDevice(device)
	if err != nil {
		if !errors.Is(err, ErrAmbiguousObservation) {
			log.Printf("[Discovery] Passive observation rejected: %v", err)
		}
		return
	}
	if created {
		log.Printf("[Discovery] Passive: new device from %s", ip)
	} else {
		log.Printf("[Discovery] Passive: refreshed device %s from %s", id, ip)
	}
}

// LookupARPEntry attempts to find the MAC address for an IP from the
// system ARP cache. Returns empty string if not found.
//
// On Linux, reads /proc/net/arp which is fast (virtual filesystem).
// Format: IP address, HW type, Flags, HW address, Mask, Device
// Example: 192.168.1.100 0x1 0x2 aa:bb:cc:dd:ee:ff * eth0
//
// On non-Linux systems, returns "" (future: support arp -a, ndp).
func LookupARPEntry(ip string) string {
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return "" // Not Linux, or /proc not available
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Scan() // Skip header line

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		if fields[0] == ip {
			return normalizeMAC(fields[3])
		}
	}
	return ""
}

// ExtractClientIP extracts the IP address from a net.Addr, stripping
// the port component. Returns empty string if extraction fails.
func ExtractClientIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		// Might not have a port (e.g., Unix socket)
		return ""
	}
	return host
}
