package gatesentryDnsFilter

import (
	"fmt"
	"log"
	"sync"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// InitializeFilters initializes internal records and exception domains.
// Blocklist downloads are now handled by DomainListManager.LoadAllLists(),
// called separately by the scheduler.
func InitializeFilters(internalRecords *map[string]string, exceptionDomains *map[string]bool, mutex *sync.RWMutex, settings *gatesentry2storage.MapStore) {
	// Reset internal records and exception domains under write lock
	mutex.Lock()
	*internalRecords = make(map[string]string)
	*exceptionDomains = make(map[string]bool)
	mutex.Unlock()

	InitializeInternalRecords(internalRecords, mutex, settings)
	InitializeExceptionDomains(exceptionDomains, mutex)

	fmt.Println("[DNS Filter] Internal records and exception domains initialized")
	log.Printf("[DNS Filter] Internal records: %d, Exception domains: %d",
		len(*internalRecords), len(*exceptionDomains))
}
