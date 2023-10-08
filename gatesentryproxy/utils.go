package gatesentryproxy

import "net"

func isLanAddress(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch ip4[0] {
		case 10, 127:
			return true
		case 172:
			return ip4[1]&0xf0 == 16
		case 192:
			return ip4[1] == 168
		}
		return false
	}

	// IPv6
	switch {
	case ip[0]&0xfe == 0xfc:
		return true
	case ip[0] == 0xfe && (ip[1]&0xfc) == 0x80:
		return true
	case ip.Equal(ip6Loopback):
		return true
	}

	return false
}

func isAVIF(data []byte) bool {
	// Check for 'ftyp' box and 'avif' major brand
	return len(data) > 12 &&
		string(data[4:8]) == "ftyp" &&
		string(data[8:12]) == "avif"
}
