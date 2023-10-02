package gatesentryDnsFilter

import (
	"fmt"
	"net"
	"sync"

	"github.com/miekg/dns"
)

func InitializeInternalRecords(records *map[string][]dns.RR, mutex *sync.Mutex) {
	mutex.Lock()
	defer mutex.Unlock()
	fmt.Println("Initializing internal records...")
	(*records)["abc.com"] = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{Name: "abc.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 3600},
			A:   net.ParseIP("10.1.0.138"),
		},
	}
	fmt.Println("Internal records initialized. Number of internal records = ", len(*records))
}
