package gatesentryDnsFilter

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/miekg/dns"
)

func InitializeFilters(blockedDomains *map[string]bool, blockedLists *[]string, internalRecords *map[string][]dns.RR, exceptionDomains *map[string]bool, mutex *sync.Mutex) {
	InitializeInternalRecords(internalRecords, mutex)
	InitializeBlockedDomains(blockedDomains, blockedLists, mutex)
	InitializeExceptionDomains(exceptionDomains, mutex)
}

func InitializeBlockedDomains(blockedDomains *map[string]bool, blocklists *[]string, mutex *sync.Mutex) {
	var wg sync.WaitGroup
	fmt.Println("Downloading blocklists...")

	for _, blocklistURL := range *blocklists {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			domains, err := fetchDomainsFromBlocklist(url)
			if err != nil {
				fmt.Println("Error fetching blocklist:", err)
				return
			}
			addDomainsToBlockedMap(blockedDomains, domains, mutex)
		}(blocklistURL)
	}

	wg.Wait()
	fmt.Println("Blocklists downloaded and processed.")
}

func fetchDomainsFromBlocklist(url string) ([]string, error) {
	fmt.Println("Downloading blocklist from:", url)
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error downloading blocklist:", err)
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
		fmt.Println("Error reading blocklist:", err)
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

	fmt.Println("Added", len(newDomains), "domains to blocked map")
	fmt.Println("Total domains in blocked map:", len(*blockedDomains))
}
