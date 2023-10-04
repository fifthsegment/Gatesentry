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
	go func() {
		_, err := bonjour.Register("GateSentry", "_gatesentry_proxy._tcp", "", 10413, []string{"txtv=1", "app=gatesentry"}, nil)
		if err != nil {
			log.Println(err.Error())
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
