package domainlist

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// FetchDomainsFromURL downloads a blocklist from a URL and parses domains.
// Supports two formats:
//   - Hosts-file format: "0.0.0.0 domain" or "::1 domain" (leading IP stripped)
//   - Plain domain-per-line format: "domain.com"
//
// Lines starting with "#" are treated as comments and skipped.
// Empty lines are skipped.
// All domains are lowercased before returning.
func FetchDomainsFromURL(url string) ([]string, error) {
	log.Printf("[DomainList] Downloading list from: %s", url)
	start := time.Now()

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	var domains []string
	scanner := bufio.NewScanner(resp.Body)

	// Increase scanner buffer for large blocklists (default is 64KB)
	const maxLineSize = 1024 * 1024 // 1MB
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		var domain string

		if len(parts) >= 2 {
			ip := parts[0]
			// Hosts-file format: "0.0.0.0 domain" or "::1 domain" or "127.0.0.1 domain"
			if ip == "0.0.0.0" || ip == "::1" || ip == "127.0.0.1" {
				domain = parts[1]
			} else {
				// Unknown multi-field format — take first field as domain
				domain = parts[0]
			}
		} else {
			// Single field — plain domain
			domain = parts[0]
		}

		// Normalize: lowercase, strip trailing dot
		domain = strings.ToLower(strings.TrimSuffix(domain, "."))

		// Skip empty or localhost entries
		if domain == "" || domain == "localhost" || domain == "0.0.0.0" {
			continue
		}

		domains = append(domains, domain)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading blocklist: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("[DomainList] Downloaded %d domains from %s in %v", len(domains), url, elapsed)

	return domains, nil
}

// ParseDomainsFromLines parses domain entries from a slice of raw strings.
// Used for local (admin-managed) lists where domains are stored directly.
// Applies the same normalization as FetchDomainsFromURL: lowercase, strip
// trailing dot, skip empty/comment lines.
func ParseDomainsFromLines(lines []string) []string {
	var domains []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		domain := strings.ToLower(strings.TrimSuffix(line, "."))
		if domain == "" {
			continue
		}
		domains = append(domains, domain)
	}
	return domains
}
