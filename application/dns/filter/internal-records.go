package gatesentryDnsFilter

import (
	"encoding/json"
	"fmt"
	"sync"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

func InitializeInternalRecords(records *map[string]string, mutex *sync.Mutex, settings *gatesentry2storage.MapStore) {
	mutex.Lock()
	defer mutex.Unlock()
	fmt.Println("Initializing internal records...")
	internalRecordsString := settings.Get("DNS_custom_entries")

	// parse json string to struct
	var customEntries []gatesentryTypes.DNSCustomEntry
	json.Unmarshal([]byte(internalRecordsString), &customEntries)

	for _, entry := range customEntries {
		(*records)[entry.Domain] = entry.IP
	}

	fmt.Println("Internal records initialized. Number of internal records = ", len(*records))
}
