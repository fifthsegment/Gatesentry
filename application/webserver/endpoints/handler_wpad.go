package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// GeneratePACFile generates a Proxy Auto-Config (PAC) file.
//
// The PAC file tells browsers:
//   - Bypass the proxy for local/private addresses (RFC 1918)
//   - Bypass the proxy for the GateSentry admin UI itself
//   - Route all other traffic through the GateSentry proxy
//
// proxyHost and proxyPort come from admin-configured settings —
// they are NOT auto-detected, because only the admin knows how
// clients on their network can reach the proxy.
func GeneratePACFile(proxyHost, proxyPort string) string {
	return fmt.Sprintf(`function FindProxyForURL(url, host) {
    // --- Direct connections (bypass proxy) ---

    // Localhost
    if (isPlainHostName(host) ||
        shExpMatch(host, "localhost") ||
        shExpMatch(host, "127.*") ||
        shExpMatch(host, "::1")) {
        return "DIRECT";
    }

    // Private networks (RFC 1918) — local LAN devices, printers, NAS, etc.
    if (isInNet(dnsResolve(host), "10.0.0.0", "255.0.0.0") ||
        isInNet(dnsResolve(host), "172.16.0.0", "255.240.0.0") ||
        isInNet(dnsResolve(host), "192.168.0.0", "255.255.0.0")) {
        return "DIRECT";
    }

    // GateSentry admin UI itself
    if (shExpMatch(host, "%s")) {
        return "DIRECT";
    }

    // --- Everything else goes through GateSentry proxy ---
    return "PROXY %s:%s; DIRECT";
}
`, proxyHost, proxyHost, proxyPort)
}

// GSApiWPADHandler serves the WPAD/PAC file.
// This handler is registered WITHOUT authentication — WPAD auto-discovery
// requires unauthenticated access from all network clients.
//
// The proxy address comes from the admin-configured wpad_proxy_host and
// wpad_proxy_port settings. If the admin hasn't configured them yet,
// the PAC file returns DIRECT for everything (safe fallback).
func GSApiWPADHandler(settings *gatesentry2storage.MapStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxyHost := settings.Get("wpad_proxy_host")
		proxyPort := settings.Get("wpad_proxy_port")
		if proxyPort == "" {
			proxyPort = "10413"
		}

		var pac string
		if proxyHost == "" {
			// Not configured yet — safe fallback: bypass everything
			pac = "function FindProxyForURL(url, host) {\n" +
				"    // GateSentry WPAD: proxy host not configured yet.\n" +
				"    return \"DIRECT\";\n" +
				"}\n"
			log.Printf("[WPAD] Served UNCONFIGURED PAC file to %s (wpad_proxy_host not set)",
				clientIP(r))
		} else {
			pac = GeneratePACFile(proxyHost, proxyPort)
			log.Printf("[WPAD] Served PAC file to %s (proxy=%s:%s)",
				clientIP(r), proxyHost, proxyPort)
		}

		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pac)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(pac))
	}
}

// GSApiWPADInfoHandler returns the current WPAD configuration as JSON.
// Used by the admin UI to show WPAD status and let the admin configure it.
//
// adminPort is the backend's own HTTP port (e.g. "8080") so the UI can
// construct the correct PAC file URL without relying on window.location
// (which may be the dev-server port or a reverse-proxy frontend).
func GSApiWPADInfoHandler(settings *gatesentry2storage.MapStore, adminPort string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		proxyHost := settings.Get("wpad_proxy_host")
		proxyPort := settings.Get("wpad_proxy_port")
		if proxyPort == "" {
			proxyPort = "10413"
		}

		configured := proxyHost != ""

		// Build the canonical PAC URL that clients should use.
		// Uses the configured proxy host + the backend admin port.
		var pacURL string
		if configured {
			if adminPort == "" || adminPort == "80" {
				pacURL = fmt.Sprintf("http://%s/wpad.dat", proxyHost)
			} else {
				pacURL = fmt.Sprintf("http://%s:%s/wpad.dat", proxyHost, adminPort)
			}
		}

		result := struct {
			Enabled    bool   `json:"enabled"`
			Configured bool   `json:"configured"`
			ProxyHost  string `json:"proxyHost"`
			ProxyPort  string `json:"proxyPort"`
			AdminPort  string `json:"adminPort"`
			PACURL     string `json:"pacUrl"`
			PACFile    string `json:"pacFile"`
		}{
			Enabled:    settings.Get("wpad_enabled") != "false",
			Configured: configured,
			ProxyHost:  proxyHost,
			ProxyPort:  proxyPort,
			AdminPort:  adminPort,
			PACURL:     pacURL,
		}

		if configured {
			result.PACFile = GeneratePACFile(proxyHost, proxyPort)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}
