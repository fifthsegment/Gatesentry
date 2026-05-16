package domainlist

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
)

// knownURLMetadata provides human-readable names and categories for
// the default DNS blocklist URLs shipped with GateSentry. This avoids
// showing raw URLs as list names in the UI after migration.
var knownURLMetadata = map[string]struct{ Name, Category string }{
	"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts":                                                                       {"StevenBlack Unified", "Ads"},
	"https://raw.githubusercontent.com/anudeepND/blacklist/master/adservers.txt":                                                             {"anudeepND Ad Servers", "Ads"},
	"https://v.firebog.net/hosts/AdguardDNS.txt":                                                                                             {"AdGuard DNS", "Ads"},
	"https://raw.githubusercontent.com/PolishFiltersTeam/KADhosts/master/KADhosts.txt":                                                       {"KADhosts", "Ads"},
	"https://raw.githubusercontent.com/FadeMind/hosts.extras/master/add.Spam/hosts":                                                          {"FadeMind Spam", "Spam"},
	"https://v.firebog.net/hosts/static/w3kbl.txt":                                                                                           {"W3KBL", "Malware"},
	"https://adaway.org/hosts.txt":                                                                                                           {"AdAway", "Ads"},
	"https://v.firebog.net/hosts/RPiList-Phishing.txt":                                                                                       {"RPiList Phishing", "Phishing"},
	"https://v.firebog.net/hosts/RPiList-Malware.txt":                                                                                        {"RPiList Malware", "Malware"},
	"https://gitlab.com/quidsup/notrack-blocklists/raw/master/notrack-malware.txt":                                                           {"NoTrack Malware", "Malware"},
	"https://pgl.yoyo.org/adservers/serverlist.php?hostformat=hosts&showintro=0&mimetype=plaintext":                                          {"Yoyo Ad Servers", "Ads"},
	"https://bitbucket.org/ethanr/dns-blacklists/raw/8575c9f96e5b4a1308f2f12394abd86d0927a4a0/bad_lists/Mandiant_APT1_Report_Appendix_D.txt": {"Mandiant APT1", "Malware"},
	"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/popupads-onlydomains.txt":                                         {"Hagezi Popup Ads", "Ads"},
	"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/tif-onlydomains.txt":                                              {"Hagezi Threat Intelligence", "Malware"},
}

// MigrateIfNeeded checks for legacy data formats and migrates them to Domain Lists.
// This is called once at startup. It is idempotent — if Domain Lists already exist,
// migration is skipped.
//
// Migrations:
//  1. dns_custom_entries (JSON array of URLs) → URL-sourced Domain Lists
//  2. blockedsites.json entries → local Domain List "Blocked Sites (migrated)"
//  3. exceptionsitelist.json entries → local Domain List "DNS Exceptions (migrated)"
//     assigned to dns_whitelist_domain_lists
func MigrateIfNeeded(manager *DomainListManager, settings *gatesentry2storage.MapStore) {
	// Check if we already have domain lists — if so, migration was already done
	lists, err := manager.GetLists()
	if err != nil {
		log.Printf("[DomainList] Error checking existing lists during migration: %v", err)
		return
	}
	if len(lists) > 0 {
		log.Println("[DomainList] Domain lists already exist, skipping migration")
		return
	}

	log.Println("[DomainList] No domain lists found — running migration from legacy format...")

	var dnsDomainListIDs []string

	// --- Migration 1: dns_custom_entries → URL-sourced Domain Lists ---
	customEntries := settings.Get("dns_custom_entries")
	if customEntries != "" {
		var urls []string
		if err := json.Unmarshal([]byte(customEntries), &urls); err != nil {
			log.Printf("[DomainList] Error parsing dns_custom_entries: %v", err)
		} else {
			for _, url := range urls {
				url = strings.TrimSpace(url)
				if url == "" {
					continue
				}

				// Look up name and category from known list
				meta, known := knownURLMetadata[url]
				name := meta.Name
				category := meta.Category
				if !known {
					// Extract a reasonable name from the URL
					name = inferNameFromURL(url)
					category = "Custom"
				}

				dl := DomainList{
					ID:          gatesentryUtils.RandomString(16),
					Name:        name,
					Description: "Migrated from DNS blocklist configuration",
					Category:    category,
					Source:      "url",
					URL:         url,
					CreatedAt:   time.Now().Format(time.RFC3339),
					LastUpdated: time.Now().Format(time.RFC3339),
				}

				created, err := manager.AddList(dl)
				if err != nil {
					log.Printf("[DomainList] Error migrating URL list %s: %v", url, err)
					continue
				}

				dnsDomainListIDs = append(dnsDomainListIDs, created.ID)
				log.Printf("[DomainList] Migrated URL list: %s → %s (%s)", url, created.Name, created.ID)
			}
		}
	}

	// Persist the DNS domain list assignment
	if len(dnsDomainListIDs) > 0 {
		dnsListsJSON, _ := json.Marshal(dnsDomainListIDs)
		settings.Update("dns_domain_lists", string(dnsListsJSON))
		log.Printf("[DomainList] Assigned %d lists to dns_domain_lists", len(dnsDomainListIDs))
	}

	log.Println("[DomainList] Migration complete")
}

// inferNameFromURL extracts a human-readable name from a URL.
// e.g., "https://example.com/hosts/blocklist.txt" → "blocklist"
func inferNameFromURL(url string) string {
	// Get the last path segment
	parts := strings.Split(url, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		segment := parts[i]
		// Strip file extension
		segment = strings.TrimSuffix(segment, ".txt")
		segment = strings.TrimSuffix(segment, ".json")
		segment = strings.TrimSuffix(segment, ".hosts")
		// Skip empty or generic segments
		if segment == "" || segment == "hosts" || segment == "raw" || segment == "master" || segment == "main" {
			continue
		}
		// Strip query string
		if idx := strings.Index(segment, "?"); idx > 0 {
			segment = segment[:idx]
		}
		return segment
	}
	return "Unnamed List"
}
