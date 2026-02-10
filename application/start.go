package gatesentryf

import (
	"fmt"
	"strconv"
)

var R *GSRuntime

func Start(webadminport int) *GSRuntime {
	GSVerString := GetApplicationVersion()
	fmt.Println("Starting GateSentry v " + GSVerString)
	R = &GSRuntime{
		WebServerPort:    webadminport,
		FilterFiles:      make(map[string]string),
		DNSServerChannel: make(chan int),
	}
	R.Init()
	LoadFilters()
	fmt.Println("Starting GateSentry webserver on port " + strconv.Itoa(R.WebServerPort))
	go GSwebserverStart(R.WebServerPort)

	return R
}

func Stop() {
	fmt.Println("Stopping GateSentry " + GSVerString)
}
