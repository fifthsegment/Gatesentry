package gatesentryDnsScheduler

import (
	"log"
	"sync"
	"time"

	gatesentryDnsFilter "bitbucket.org/abdullah_irfan/gatesentryf/dns/filter"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

type InitializerType func(*map[string]bool, *[]string, *sync.Mutex)

func RunScheduler(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string]string,
	exceptionDomains *map[string]bool,
	mutex *sync.Mutex,
	settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo,
	updateIntervalHourly int,
	restartChan chan bool,
) {

	ticker := time.NewTicker(time.Duration(updateIntervalHourly) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-restartChan:
			log.Println("Restarting scheduler...")
			doInitialize(blockedDomains, blockedLists, internalRecords, exceptionDomains, mutex, settings, dnsinfo, updateIntervalHourly, restartChan)
			// Here you would re-initialize anything necessary for a restart
		case <-ticker.C:
			log.Println("Running scheduler...")
			doInitialize(blockedDomains, blockedLists, internalRecords, exceptionDomains, mutex, settings, dnsinfo, updateIntervalHourly, restartChan)
		}
	}

}

func doInitialize(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string]string,
	exceptionDomains *map[string]bool,
	mutex *sync.Mutex,
	settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo,
	updateIntervalHourly int,
	restartChan chan bool) {
	gatesentryDnsFilter.InitializeFilters(blockedDomains, blockedLists, internalRecords, exceptionDomains, mutex, settings, dnsinfo)
	dnsinfo.NextUpdate = int(time.Now().Add(time.Hour * time.Duration(updateIntervalHourly)).Unix())
}
