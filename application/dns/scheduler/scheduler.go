package gatesentryDnsScheduler

import (
	"log"
	"sync"
	"time"

	gatesentryDnsFilter "bitbucket.org/abdullah_irfan/gatesentryf/dns/filter"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

type InitializerType func(*map[string]bool, *[]string, *sync.RWMutex)

func RunScheduler(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string]string,
	mutex *sync.RWMutex,
	settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo,
	categories *gatesentryPolicy.CategoryIndex,
	referencedCategories func() []string,
	updateIntervalHourly int,
	restartChan chan bool,
) {

	ticker := time.NewTicker(time.Duration(updateIntervalHourly) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-restartChan:
			log.Println("Restarting scheduler...")
			doInitialize(blockedDomains, blockedLists, internalRecords, mutex, settings, dnsinfo, categories, referencedCategories, updateIntervalHourly, restartChan)
			// Here you would re-initialize anything necessary for a restart
		case <-ticker.C:
			log.Println("Running scheduler...")
			doInitialize(blockedDomains, blockedLists, internalRecords, mutex, settings, dnsinfo, categories, referencedCategories, updateIntervalHourly, restartChan)
		}
	}

}

func doInitialize(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string]string,
	mutex *sync.RWMutex,
	settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo,
	categories *gatesentryPolicy.CategoryIndex,
	referencedCategories func() []string,
	updateIntervalHourly int,
	restartChan chan bool) {
	gatesentryDnsFilter.InitializeFilters(blockedDomains, blockedLists, internalRecords, mutex, settings, dnsinfo, categories, referencedCategories)
	dnsinfo.NextUpdate = int(time.Now().Add(time.Hour * time.Duration(updateIntervalHourly)).Unix())
}
