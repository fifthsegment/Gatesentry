package gatesentryDnsFilter

import (
	"log"
	"sync"
	"time"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// InitializeCategories downloads the categories the gateway actually uses and
// records their domain sets in the shared index, which is the only place
// category domains live: the index covers parent domains, while the global
// blocked map is an exact-match lookup, and copying a multi-million-domain feed
// into both would double its memory for no gain.
//
// The used set is the categories enabled gateway-wide plus the categories any
// policy group selects, which referenced reports. A category in neither is
// dropped from the index: the malware feed alone is millions of domains, so an
// unused feed must not stay resident. A category whose feed fails to download
// keeps its previous index entry, so a transient upstream failure cannot
// silently widen what the gateway blocks.
func InitializeCategories(settings *gatesentry2storage.MapStore, index *gatesentryPolicy.CategoryIndex, referenced func() []string) {
	if index == nil {
		return
	}
	catalog := gatesentryPolicy.CategoryCatalog()
	if len(catalog) == 0 {
		return
	}
	enabled := make(map[string]bool)
	for _, id := range gatesentryPolicy.LoadEnabledCategories(settings) {
		enabled[id] = true
	}
	wanted := make(map[string]bool, len(enabled))
	for id := range enabled {
		wanted[id] = true
	}
	if referenced != nil {
		for _, id := range referenced() {
			wanted[id] = true
		}
	}
	toDownload := make([]gatesentryPolicy.Category, 0, len(catalog))
	for _, category := range catalog {
		if wanted[category.ID] {
			toDownload = append(toDownload, category)
			continue
		}
		index.Drop(category.ID)
	}
	if len(toDownload) == 0 {
		return
	}

	type categoryResult struct {
		id      string
		domains []string
		failed  bool
	}

	// Downloads run concurrently, matching the blocklist refresh: the sources
	// are independent and the largest feeds would otherwise serialize startup.
	results := make(chan categoryResult, len(toDownload))
	var wg sync.WaitGroup
	for _, category := range toDownload {
		wg.Add(1)
		go func(category gatesentryPolicy.Category) {
			defer wg.Done()
			var domains []string
			failed := false
			for _, source := range category.Sources {
				fetched, err := fetchDomainsFromBlocklist(source)
				if err != nil {
					log.Printf("[DNS] [Error] Category %s source failed: %v", category.ID, err)
					failed = true
					continue
				}
				domains = append(domains, fetched...)
			}
			results <- categoryResult{id: category.ID, domains: domains, failed: failed}
		}(category)
	}
	wg.Wait()
	close(results)

	now := time.Now()
	for result := range results {
		if result.failed {
			continue
		}
		index.Replace(result.id, result.domains, now)
		log.Printf("[DNS] Category %s loaded with %d domains", result.id, len(result.domains))
	}
}
