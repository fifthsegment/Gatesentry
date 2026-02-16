package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// validHostnameOrIP matches hostnames, IPv4 addresses, and bracketed IPv6.
// Rejects strings containing JS-special characters (quotes, backslashes, semicolons).
var validHostnameOrIP = regexp.MustCompile(`^[a-zA-Z0-9._:\[\]-]+$`)

// validPort matches a numeric port (1–65535 range checked separately).
var validPort = regexp.MustCompile(`^[0-9]{1,5}$`)

// validDomain matches domain names: alphanumeric, dots, hyphens only.
// Used to sanitize bypass domain entries before interpolating into PAC JS.
var validDomain = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

// maxBypassDomains limits how many bypass domains can be included in the PAC file.
// PAC files are evaluated by the browser on every request, so too many entries
// will degrade performance.
const maxBypassDomains = 1000

// loadBypassDomains reads the wpad_bypass_domain_lists setting (JSON array of
// domain list IDs), fetches all domains from those lists via the DomainListManager,
// and returns a deduplicated, sanitized slice of domain names.
func loadBypassDomains(settings *gatesentry2storage.MapStore) []string {
	raw := settings.Get("wpad_bypass_domain_lists")
	if raw == "" {
		return nil
	}

	var listIDs []string
	if err := json.Unmarshal([]byte(raw), &listIDs); err != nil {
		log.Printf("[WPAD] WARNING: failed to parse wpad_bypass_domain_lists: %v", err)
		return nil
	}

	dlm := GetDomainListManager()
	if dlm == nil {
		log.Printf("[WPAD] WARNING: domain list manager not initialized, cannot load bypass domains")
		return nil
	}

	seen := make(map[string]bool)
	var domains []string

	for _, listID := range listIDs {
		listDomains, err := dlm.GetDomainsForList(listID)
		if err != nil {
			log.Printf("[WPAD] WARNING: failed to load domains for list %s: %v", listID, err)
			continue
		}
		for _, d := range listDomains {
			d = strings.TrimRight(strings.TrimSpace(strings.ToLower(d)), ".")
			if d == "" || seen[d] {
				continue
			}
			// Sanitize: only allow safe domain characters (defense-in-depth
			// against JS injection even though these come from admin-managed lists)
			if !validDomain.MatchString(d) {
				log.Printf("[WPAD] WARNING: skipping invalid bypass domain %q", d)
				continue
			}
			seen[d] = true
			domains = append(domains, d)
			if len(domains) >= maxBypassDomains {
				log.Printf("[WPAD] WARNING: bypass domain limit reached (%d), truncating", maxBypassDomains)
				break
			}
		}
		if len(domains) >= maxBypassDomains {
			break
		}
	}

	return domains
}

// GeneratePACFile generates a Proxy Auto-Config (PAC) file.
//
// The PAC file tells browsers:
//   - Bypass the proxy for local/private addresses (RFC 1918)
//   - Bypass the proxy for the GateSentry admin UI itself
//   - Bypass the proxy for admin-configured bypass domains (e.g. 1Password, Copilot)
//   - Route all other traffic through the GateSentry proxy
//
// proxyHost and proxyPort come from admin-configured settings —
// they are NOT auto-detected, because only the admin knows how
// clients on their network can reach the proxy.
//
// bypassDomains is an optional list of domains that should connect DIRECT,
// bypassing the proxy entirely. Useful for apps that don't support proxy
// authentication (certificate pinning, etc.).
func GeneratePACFile(proxyHost, proxyPort string, bypassDomains []string) string {
	// Validate inputs to prevent JavaScript injection in the PAC file.
	// These values come from admin-configured settings, but defense-in-depth
	// requires we never interpolate unvalidated strings into JS.
	if !validHostnameOrIP.MatchString(proxyHost) {
		log.Printf("[WPAD] WARNING: invalid proxyHost %q — refusing to generate PAC", proxyHost)
		return "function FindProxyForURL(url, host) { return \"DIRECT\"; }\n"
	}
	if !validPort.MatchString(proxyPort) {
		log.Printf("[WPAD] WARNING: invalid proxyPort %q — refusing to generate PAC", proxyPort)
		return "function FindProxyForURL(url, host) { return \"DIRECT\"; }\n"
	}

	// Build the optional bypass domains section
	var bypassSection string
	if len(bypassDomains) > 0 {
		var sb strings.Builder
		sb.WriteString("\n    // --- Admin-configured proxy bypass domains ---\n")
		sb.WriteString("    // These domains connect DIRECTLY without going through the proxy.\n")
		sb.WriteString("    // Configured via Settings > WPAD > Proxy Bypass Lists.\n")
		sb.WriteString("    // Useful for apps that don't support proxy auth (1Password, Copilot, etc.)\n")

		if len(bypassDomains) <= 15 {
			// Small list: use a single if-statement with || for readability
			sb.WriteString("    if (")
			for i, d := range bypassDomains {
				if i > 0 {
					sb.WriteString(" ||\n        ")
				}
				sb.WriteString(fmt.Sprintf("dnsDomainIs(host, \"%s\")", d))
			}
			sb.WriteString(") {\n        return \"DIRECT\";\n    }\n")
		} else {
			// Large list: use an array + loop for a smaller, faster PAC file
			sb.WriteString("    var bypassDomains = [\n")
			for i, d := range bypassDomains {
				if i > 0 {
					sb.WriteString(",\n")
				}
				sb.WriteString(fmt.Sprintf("        \"%s\"", d))
			}
			sb.WriteString("\n    ];\n")
			sb.WriteString("    for (var i = 0; i < bypassDomains.length; i++) {\n")
			sb.WriteString("        if (dnsDomainIs(host, bypassDomains[i])) return \"DIRECT\";\n")
			sb.WriteString("    }\n")
		}
		bypassSection = sb.String()
	}

	return fmt.Sprintf(`function FindProxyForURL(url, host) {
    // --- Direct connections (bypass proxy) ---
    // These correspond to the NO_PROXY / no_proxy environment variable.

    // Plain hostnames (no dots — e.g. "myserver") and localhost variants
    if (isPlainHostName(host) ||
        shExpMatch(host, "localhost") ||
        shExpMatch(host, "localhost.*") ||
        shExpMatch(host, "127.*") ||
        shExpMatch(host, "::1")) {
        return "DIRECT";
    }

    // Private IP literals — only bypass when the USER typed a private IP
    // in the address bar (e.g. http://192.168.1.1, http://10.0.0.1).
    //
    // IMPORTANT: We do NOT use dnsResolve() here.  dnsResolve() would
    // cause DNS-blocked domains (which resolve to GateSentry's private IP)
    // to bypass the proxy, preventing the proxy from showing its block
    // page and performing HTTPS MITM interception.
    if (shExpMatch(host, "10.*") ||
        shExpMatch(host, "172.16.*") || shExpMatch(host, "172.17.*") ||
        shExpMatch(host, "172.18.*") || shExpMatch(host, "172.19.*") ||
        shExpMatch(host, "172.20.*") || shExpMatch(host, "172.21.*") ||
        shExpMatch(host, "172.22.*") || shExpMatch(host, "172.23.*") ||
        shExpMatch(host, "172.24.*") || shExpMatch(host, "172.25.*") ||
        shExpMatch(host, "172.26.*") || shExpMatch(host, "172.27.*") ||
        shExpMatch(host, "172.28.*") || shExpMatch(host, "172.29.*") ||
        shExpMatch(host, "172.30.*") || shExpMatch(host, "172.31.*") ||
        shExpMatch(host, "192.168.*")) {
        return "DIRECT";
    }

    // Link-local addresses (RFC 3927)
    if (shExpMatch(host, "169.254.*")) {
        return "DIRECT";
    }

    // mDNS / Bonjour / local service discovery
    if (shExpMatch(host, "*.local")) {
        return "DIRECT";
    }

    // GateSentry admin UI itself
    if (shExpMatch(host, "%s")) {
        return "DIRECT";
    }
%s
    // --- Everything else goes through GateSentry proxy ---
    // PROXY = HTTP proxying;  HTTPS = HTTPS proxying (CONNECT tunnelling)
    // Both are needed so clients set both HTTP_PROXY and HTTPS_PROXY.
    return "PROXY %s:%s; HTTPS %s:%s; DIRECT";
}
`, proxyHost, bypassSection, proxyHost, proxyPort, proxyHost, proxyPort)
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
			bypassDomains := loadBypassDomains(settings)
			pac = GeneratePACFile(proxyHost, proxyPort, bypassDomains)
			log.Printf("[WPAD] Served PAC file to %s (proxy=%s:%s, bypass_domains=%d)",
				clientIP(r), proxyHost, proxyPort, len(bypassDomains))
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

		// Load bypass domains for the PAC preview
		var bypassDomains []string
		if configured {
			bypassDomains = loadBypassDomains(settings)
		}

		result := struct {
			Enabled       bool   `json:"enabled"`
			Configured    bool   `json:"configured"`
			ProxyHost     string `json:"proxyHost"`
			ProxyPort     string `json:"proxyPort"`
			AdminPort     string `json:"adminPort"`
			PACURL        string `json:"pacUrl"`
			PACFile       string `json:"pacFile"`
			BypassDomains int    `json:"bypassDomains"`
		}{
			Enabled:       settings.Get("wpad_enabled") != "false",
			Configured:    configured,
			ProxyHost:     proxyHost,
			ProxyPort:     proxyPort,
			AdminPort:     adminPort,
			PACURL:        pacURL,
			BypassDomains: len(bypassDomains),
		}

		if configured {
			result.PACFile = GeneratePACFile(proxyHost, proxyPort, bypassDomains)
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
