package gatesentryDnsUtils

import (
	"fmt"
	"net"
)

// GetLocalIP returns the local IPv4 address that the OS would use to reach
// the internet (i.e. the address on the default-route interface).
//
// It works by opening a UDP "connection" to a well-known external IP.
// No traffic is sent — UDP is connectionless — but the OS routing table
// picks the correct source interface. This is reliable on multi-NIC hosts
// where iterating interfaces would return Docker bridges, WireGuard tunnels,
// or secondary loopback addresses before the real LAN IP.
func GetLocalIP() (string, error) {
	// Try the routing-table approach first (most reliable)
	conn, err := net.Dial("udp4", "8.8.8.8:53")
	if err == nil {
		defer conn.Close()
		if localAddr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
			return localAddr.IP.String(), nil
		}
	}

	// Fallback: iterate interfaces, skip loopback interface entirely,
	// and prefer 192.168.x.x or 10.x.x.x addresses on real interfaces.
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	var fallback string
	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil && !ipnet.IP.IsLoopback() {
				ip := ipnet.IP.String()
				// Prefer common LAN ranges
				if len(ip) > 4 && (ip[:4] == "192." || ip[:3] == "10.") {
					return ip, nil
				}
				if fallback == "" {
					fallback = ip
				}
			}
		}
	}

	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("no local IPv4 address found")
}
