package gatesentryDnsScheduler

import (
	"fmt"
	"sync"
	"time"

	gatesentryDnsFilter "bitbucket.org/abdullah_irfan/gatesentryf/dns/filter"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

type InitializerType func(*map[string]bool, *[]string, *sync.Mutex)

func RunScheduler(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string]string,
	exceptionDomains *map[string]bool,
	mutex *sync.Mutex,
	settings *gatesentry2storage.MapStore) {

	for {
		fmt.Println("Running scheduler...")
		gatesentryDnsFilter.InitializeFilters(blockedDomains, blockedLists, internalRecords, exceptionDomains, mutex, settings)
		time.Sleep(10 * time.Hour)
	}
}
