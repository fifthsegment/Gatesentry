package gatesentryDnsScheduler

import (
	"fmt"
	"sync"
	"time"

	gatesentryDnsFilter "bitbucket.org/abdullah_irfan/gatesentryf/dns/filter"
	"github.com/miekg/dns"
)

type InitializerType func(*map[string]bool, *[]string, *sync.Mutex)

func RunScheduler(blockedDomains *map[string]bool,
	blockedLists *[]string,
	internalRecords *map[string][]dns.RR,
	exceptionDomains *map[string]bool,
	mutex *sync.Mutex) {

	for {
		fmt.Println("Running scheduler...")
		gatesentryDnsFilter.InitializeFilters(blockedDomains, blockedLists, internalRecords, exceptionDomains, mutex)
		time.Sleep(10 * time.Hour)
	}
}
