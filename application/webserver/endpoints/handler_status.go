package gatesentryWebserverEndpoints

import (
	"net"
	"os"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

type StatusResponse struct {
	ServerUrl  string `json:"server_url"`
	DnsAddress string `json:"dns_address"`
	DnsPort    string `json:"dns_port"`
	ProxyPort  string `json:"proxy_port"`
	ProxyUrl   string `json:"proxy_url"`
}

func ApiGetStatus(logger *gatesentryLogger.Log, boundAddress *string, settings *gatesentry2storage.MapStore) interface{} {
	// Determine the host IP: prefer the configured wpad_proxy_host from
	// settings (which the admin sets explicitly), then fall back to the
	// first non-loopback IPv4 address, and finally to the old hostname
	// lookup that was used before.
	hostIP := ""
	if settings != nil {
		hostIP = settings.Get("wpad_proxy_host")
	}
	if hostIP == "" {
		hostIP = detectLanIP()
	}
	if hostIP == "" {
		// Last resort: use the legacy bound address (may be 127.0.1.1)
		hostIP = *boundAddress
	}

	// DNS port from env var (mirrors what the DNS server actually binds to)
	dnsPort := os.Getenv("GATESENTRY_DNS_PORT")
	if dnsPort == "" {
		dnsPort = "53" // default
	}

	// Proxy port from settings (same host, different port)
	proxyPort := ""
	if settings != nil {
		proxyPort = settings.Get("wpad_proxy_port")
	}
	if proxyPort == "" {
		proxyPort = "10413" // default
	}

	response := StatusResponse{
		ServerUrl:  hostIP + ":" + dnsPort,
		DnsAddress: hostIP,
		DnsPort:    dnsPort,
		ProxyPort:  proxyPort,
		ProxyUrl:   hostIP + ":" + proxyPort,
	}

	return response
}

// detectLanIP returns the first non-loopback IPv4 address of this machine.
func detectLanIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
