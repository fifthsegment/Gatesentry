package gatesentryf

import (
	"fmt"
	"log"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentry2logger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

var (
	blocklists = []string{}
)

func DNSServerThread(baseDir string, logger *gatesentry2logger.Log, c <-chan int, settings *gatesentry2storage.MapStore, info *gatesentryTypes.DnsServerInfo) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic:", r)
		}
	}()

	for {
		select {
		case msg := <-c:
			log.Println("[DNS.SERVER] Received message:", msg)
			if msg == 1 {
				// Start the DNS server
				go gatesentryDnsServer.StartDNSServer(baseDir, logger, blocklists, settings, R.DnsServerInfo, R.RunRuleHandler)
				log.Println("[DNS.SERVER] started")
			} else if msg == 2 {
				log.Println("[DNS.SERVER] Stopping DNS server")
				// Stop the DNS server
				go gatesentryDnsServer.StopDNSServer()
				log.Println("[DNS.SERVER] DNS server stopped")
			}
		}
	}

}
