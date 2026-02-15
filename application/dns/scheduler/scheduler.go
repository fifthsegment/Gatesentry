package gatesentryDnsScheduler

import (
	"log"
	"sync"
	"time"

	gatesentryDnsFilter "bitbucket.org/abdullah_irfan/gatesentryf/dns/filter"
	gatesentryDomainList "bitbucket.org/abdullah_irfan/gatesentryf/domainlist"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

type InitializerType func(*map[string]bool, *[]string, *sync.RWMutex)

func RunScheduler(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string]string,
	exceptionDomains *map[string]bool,
	mutex *sync.RWMutex,
	settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo,
	updateIntervalHourly int,
	restartChan chan bool,
	dlManager *gatesentryDomainList.DomainListManager,
) {

	ticker := time.NewTicker(time.Duration(updateIntervalHourly) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-restartChan:
			log.Println("Restarting scheduler...")
			doInitialize(blockedDomains, blockedLists, internalRecords, exceptionDomains, mutex, settings, dnsinfo, updateIntervalHourly, restartChan, dlManager)
		case <-ticker.C:
			log.Println("Running scheduler...")
			doInitialize(blockedDomains, blockedLists, internalRecords, exceptionDomains, mutex, settings, dnsinfo, updateIntervalHourly, restartChan, dlManager)
		}
	}

}

func doInitialize(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string]string,
	exceptionDomains *map[string]bool,
	mutex *sync.RWMutex,
	settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo,
	updateIntervalHourly int,
	restartChan chan bool,
	dlManager *gatesentryDomainList.DomainListManager) {

	// Initialize internal records and exception domains (legacy path — still needed)
	gatesentryDnsFilter.InitializeFilters(internalRecords, exceptionDomains, mutex, settings)

	// Refresh all domain lists (downloads URL-sourced lists, rebuilds index).
	// This replaces the old InitializeBlockedDomains flow.
	if dlManager != nil {
		log.Println("[DNS Scheduler] Refreshing domain lists via DomainListManager...")
		dlManager.LoadAllLists()
		dnsinfo.NumberDomainsBlocked = dlManager.Index.TotalDomains()
		log.Printf("[DNS Scheduler] Domain list refresh complete. Total indexed domains: %d", dnsinfo.NumberDomainsBlocked)
	}

	dnsinfo.NextUpdate = int(time.Now().Add(time.Hour * time.Duration(updateIntervalHourly)).Unix())
	dnsinfo.LastUpdated = int(time.Now().Unix())
}
