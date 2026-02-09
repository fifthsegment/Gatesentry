package gatesentryf

import (
	"log"
	// "os"
	// "os/signal"
	// "time"

	"github.com/oleksandr/bonjour"
)

func StartBonjour() {
	log.Println("Starting Bonjour service")

	// Advertise the web admin UI so browsers resolve http://gatesentry.local
	go func() {
		_, err := bonjour.Register("GateSentry", "_http._tcp", "", 80, []string{"txtv=1", "app=gatesentry", "path=/"}, nil)
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
