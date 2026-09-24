package gatesentryf

import (
	"log"
	"sync"

	"github.com/oleksandr/bonjour"
)

var bonjourServices struct {
	sync.Mutex
	servers []*bonjour.Server
	stopped bool
	pending sync.WaitGroup
}

func StartBonjour() {
	log.Println("Starting Bonjour service")
	bonjourServices.Lock()
	bonjourServices.stopped = false
	bonjourServices.pending.Add(2)
	bonjourServices.Unlock()

	// Advertise the web admin UI so browsers resolve http://gatesentry.local.
	go registerBonjour("GateSentry", "_http._tcp", 80, []string{"txtv=1", "app=gatesentry", "path=/"})
	// Advertise the filtering proxy for proxy auto-discovery.
	go registerBonjour("GateSentry Proxy", "_gatesentry_proxy._tcp", 10413, []string{"txtv=1", "app=gatesentry"})
}

func registerBonjour(instance, service string, port int, text []string) {
	defer bonjourServices.pending.Done()
	server, err := bonjour.Register(instance, service, "", port, text, nil)
	if err != nil {
		log.Printf("[Bonjour] %s registration error: %v", instance, err)
		return
	}
	bonjourServices.Lock()
	if bonjourServices.stopped {
		bonjourServices.Unlock()
		server.Shutdown()
		return
	}
	bonjourServices.servers = append(bonjourServices.servers, server)
	bonjourServices.Unlock()
}

func StopBonjour() {
	bonjourServices.Lock()
	bonjourServices.stopped = true
	bonjourServices.Unlock()
	bonjourServices.pending.Wait()

	bonjourServices.Lock()
	servers := bonjourServices.servers
	bonjourServices.servers = nil
	bonjourServices.Unlock()
	for _, server := range servers {
		server.Shutdown()
	}
}
