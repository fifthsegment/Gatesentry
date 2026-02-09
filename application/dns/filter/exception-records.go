package gatesentryDnsFilter

import (
	"fmt"
	"sync"
)

func InitializeExceptionDomains(exceptionDomains *map[string]bool, mutex *sync.RWMutex) {
	mutex.Lock()
	defer mutex.Unlock()
	fmt.Println("Initializing exception domains...")
	// (*exceptionDomains)["doubleclick.net"] = true
	fmt.Println("Exception domains initialized. Number of exception domains = ", len(*exceptionDomains))
}
