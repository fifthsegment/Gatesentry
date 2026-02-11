package discovery

import (
	"bufio"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

// ObservePassiveQuery records that a DNS query was seen from the given IP.
// If the IP is already known, it updates LastSeen (fast path).
// If unknown, it attempts a MAC lookup and creates a new passive device entry.
//
// This is the main entry point for Phase 2 passive discovery.
// Called from handleDNSRequest in a goroutine to avoid adding latency.
func (ds *DeviceStore) ObservePassiveQuery(clientIP string) {
	if clientIP == "" {
		return
	}

	// Skip loopback addresses — not real devices
	if clientIP == "127.0.0.1" || clientIP == "::1" || clientIP == "0.0.0.0" {
		return
	}

	// Fast path: known device — just touch it (map lookup + timestamp update).
	// FindDeviceByIP uses RLock internally, then TouchDevice uses Lock briefly.
	existing := ds.FindDeviceByIP(clientIP)
	if existing != nil {
		ds.TouchDevice(existing.ID)
		return
	}

	// Slow path: unknown device — create it.
	// This only happens once per unique IP, so the cost is acceptable.
	mac := LookupARPEntry(clientIP)

	// Check if we know this MAC under a different IP (DHCP renewal / IP change)
	if mac != "" {
		existingByMAC := ds.FindDeviceByMAC(mac)
		if existingByMAC != nil {
			// Known device, new IP — update the address
			if net.ParseIP(clientIP).To4() != nil {
				ds.UpdateDeviceIP(existingByMAC.ID, clientIP, "")
			} else {
				ds.UpdateDeviceIP(existingByMAC.ID, "", clientIP)
			}
			log.Printf("[Discovery] Passive: updated IP for device %s (%s → %s)",
				existingByMAC.GetDisplayName(), existingByMAC.IPv4, clientIP)
			return
		}
	}

	// Completely new device — create a passive entry
	now := time.Now()
	device := &Device{
		Source:    SourcePassive,
		Sources:   []DiscoverySource{SourcePassive},
		FirstSeen: now,
		LastSeen:  now,
		Online:    true,
	}

	if net.ParseIP(clientIP) != nil && net.ParseIP(clientIP).To4() != nil {
		device.IPv4 = clientIP
	} else {
		device.IPv6 = clientIP
	}

	if mac != "" {
		device.MACs = []string{mac}
	}

	ds.UpsertDevice(device)
	log.Printf("[Discovery] Passive: new device from %s (MAC: %s)", clientIP, mac)
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
			mac := strings.ToLower(fields[3])
			// "00:00:00:00:00:00" means incomplete ARP entry
			if mac == "00:00:00:00:00:00" {
				return ""
			}
			return mac
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
