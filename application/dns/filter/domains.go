package gatesentryDnsFilter

import (
	"bufio"
	"log"
	"net/http"
	"strings"
	"sync"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func InitializeFilters(blockedDomains *map[string]bool, blockedLists *[]string, internalRecords *map[string]string, exceptionDomains *map[string]bool, mutex *sync.Mutex, settings *gatesentry2storage.MapStore) {
	InitializeInternalRecords(internalRecords, mutex, settings)
	InitializeBlockedDomains(blockedDomains, blockedLists, mutex)
	InitializeExceptionDomains(exceptionDomains, mutex)
}

func InitializeBlockedDomains(blockedDomains *map[string]bool, blocklists *[]string, mutex *sync.Mutex) {
	var wg sync.WaitGroup
	log.Println("[DNS] Downloading blocklists...")

	for _, blocklistURL := range *blocklists {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			domains, err := fetchDomainsFromBlocklist(url)
			if err != nil {
				log.Println("[DNS] [Error] Failed to fetch blocklist:", err)
				return
			}
			addDomainsToBlockedMap(blockedDomains, domains, mutex)
		}(blocklistURL)
	}

	wg.Wait()
	log.Println("[DNS] Blocklists downloaded and processed.")
}

func fetchDomainsFromBlocklist(url string) ([]string, error) {
	log.Println("[DNS] Downloading blocklist from:", url)
	resp, err := http.Get(url)
	if err != nil {
		log.Println("[DNS] [Error] downloading blocklist:", err)
		return nil, err
	}
	defer resp.Body.Close()

	var domains []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			// Skip comments and empty lines
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			ip := parts[0]
			domain := parts[1]
			if ip == "0.0.0.0" || ip == "::1" {
				domains = append(domains, domain)
			}
		} else {
			domain := parts[0]
			domains = append(domains, domain)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Println("[DNS] [Error] Reading blocklist:", err)
		return nil, err
	}

	return domains, nil
}

func addDomainsToBlockedMap(blockedDomains *map[string]bool, newDomains []string, mutex *sync.Mutex) {
	mutex.Lock()
	defer mutex.Unlock()

	for _, domain := range newDomains {
		(*blockedDomains)[domain] = true
	}

	log.Println("[DNS] Added", len(newDomains), "domains to blocked map")
	log.Println("[DNS] Total domains in blocked map:", len(*blockedDomains))
}
