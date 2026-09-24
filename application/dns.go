package gatesentryf

import (
	"context"
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

func DNSServerThread(ctx context.Context, baseDir string, logger *gatesentry2logger.Log, c <-chan int, settings *gatesentry2storage.MapStore, devices *gatesentry2storage.MapStore, info *gatesentryTypes.DnsServerInfo) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic:", r)
		}
	}()

	// Policies are enforced by the proxy as well as DNS, so the policy
	// service, device store, and category feeds start even when the DNS
	// listener is disabled.
	gatesentryDnsServer.StartPolicyEnforcement(baseDir, blocklists, settings, devices, info)

	var serverDone chan struct{}
	startServer := func() {
		if serverDone != nil {
			return
		}
		ready := make(chan struct{})
		serverDone = make(chan struct{})
		go func(done chan struct{}) {
			defer close(done)
			gatesentryDnsServer.StartDNSServerWithReady(baseDir, logger, blocklists, settings, devices, info, ready)
		}(serverDone)
		<-ready
		log.Println("[DNS.SERVER] started")
	}
	stopServer := func() {
		if serverDone == nil {
			return
		}
		gatesentryDnsServer.StopDNSServer()
		<-serverDone
		serverDone = nil
		log.Println("[DNS.SERVER] stopped")
	}

	for {
		select {
		case <-ctx.Done():
			stopServer()
			return
		case msg := <-c:
			log.Println("[DNS.SERVER] Received message:", msg)
			if msg == 1 {
				startServer()
			} else if msg == 2 {
				stopServer()
			}
		}
	}
}
