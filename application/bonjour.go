package gatesentryf

import (
	"log"
	"os"
	"strconv"

	"github.com/oleksandr/bonjour"
)

func StartBonjour() {
	log.Println("Starting Bonjour service")

	// Derive admin port from environment (same source of truth as main.go)
	adminPort := 80
	if envPort := os.Getenv("GS_ADMIN_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			adminPort = p
		}
	}

	// Derive base path for TXT record
	basePath := os.Getenv("GS_BASE_PATH")
	if basePath == "" {
		basePath = "/"
	}

	// Advertise the web admin UI so browsers resolve http://gatesentry.local
	go func() {
		_, err := bonjour.Register("GateSentry", "_http._tcp", "", adminPort, []string{"txtv=1", "app=gatesentry", "path=" + basePath}, nil)
		if err != nil {
			log.Println("[Bonjour] HTTP registration error:", err.Error())
		}
	}()

	// Advertise the filtering proxy for proxy auto-discovery
	go func() {
		_, err := bonjour.Register("GateSentry Proxy", "_gatesentry_proxy._tcp", "", 10413, []string{"txtv=1", "app=gatesentry"}, nil)
		if err != nil {
			log.Println("[Bonjour] Proxy registration error:", err.Error())
		}
	}()

	// Run registration (blocking call)

	// Ctrl+C handling
	// handler := make(chan os.Signal, 1)
	// signal.Notify(handler, os.Interrupt)
	// for sig := range handler {
	//     if sig == os.Interrupt {
	//         s.Shutdown()
	//         time.Sleep(1e9)
	//         break
	//     }
	// }
}
