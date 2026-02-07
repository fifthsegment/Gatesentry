package gatesentryDnsFilter

import (
	"bufio"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

func InitializeFilters(blockedDomains *map[string]bool, blockedLists *[]string, internalRecords *map[string]string, exceptionDomains *map[string]bool, mutex *sync.RWMutex, settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo) {
	// Hold write lock while replacing the maps to prevent race with readers
	mutex.Lock()
	*blockedDomains = make(map[string]bool)
	*blockedLists = []string{}
	*internalRecords = make(map[string]string)
	*exceptionDomains = make(map[string]bool)
	mutex.Unlock()

	dnsinfo.NumberDomainsBlocked = 0
	custom_entries := settings.Get("dns_custom_entries")
	log.Println("[DNS.SERVER] Custom entries found")
	// unmarshall json array string to array
	custom_entries_array := []string{}
	//convert string to byte array
	err := json.Unmarshal([]byte(custom_entries), &custom_entries_array)
	if err != nil {
		log.Println("[DNS.SERVER] Error unmarshalling custom entries:", err)
	} else {
		// check if blocklists already contains custom entries
		entriesAdded := 0
		for _, custom_entry := range custom_entries_array {
			found := false
			for _, blocklist := range *blockedLists {
				if blocklist == custom_entry {
					found = true
					break
				}
			}
			if !found {
				*blockedLists = append(*blockedLists, custom_entry)
				entriesAdded++
			}
		}
		log.Println("[DNS.SERVER] Custom entries added to blocklists count:", entriesAdded)
	}
	InitializeInternalRecords(internalRecords, mutex, settings)
	InitializeBlockedDomains(blockedDomains, blockedLists, mutex, dnsinfo)
	InitializeExceptionDomains(exceptionDomains, mutex)
}

func InitializeBlockedDomains(blockedDomains *map[string]bool, blocklists *[]string, mutex *sync.RWMutex, dnsinfo *gatesentryTypes.DnsServerInfo) {
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
			addDomainsToBlockedMap(blockedDomains, domains, mutex, dnsinfo)
		}(blocklistURL)
	}
	dnsinfo.LastUpdated = int(time.Now().Unix())

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

func addDomainsToBlockedMap(blockedDomains *map[string]bool, newDomains []string, mutex *sync.RWMutex, dnsinfo *gatesentryTypes.DnsServerInfo) {
	mutex.Lock()
	defer mutex.Unlock()

	for _, domain := range newDomains {
		(*blockedDomains)[domain] = true
		dnsinfo.NumberDomainsBlocked++
	}

	log.Println("[DNS] Added", len(newDomains), "domains to blocked map")
	log.Println("[DNS] Total domains in blocked map:", len(*blockedDomains))
}
