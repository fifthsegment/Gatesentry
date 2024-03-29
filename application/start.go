package gatesentryf

import (
	"fmt"
	"strconv"
	// "gatesentry2/proxy"
)

var R *GSRuntime

func Start(webadminport int) *GSRuntime {
	GSVerString := GetApplicationVersion()
	fmt.Println("Starting GateSentry v " + GSVerString)
	// proxy := gatesentry2proxy.StartProxy();
	R = &GSRuntime{
		WebServerPort:    webadminport,
		FilterFiles:      make(map[string]string),
		DNSServerChannel: make(chan int),

		// Proxy: proxy,
		// FileContents : make(map[string][]GSFILTERLINE),
	}
	R.Init()
	LoadFilters()
	// RegisterProxyHandlers();
	fmt.Println("Starting GateSentry webserver on port " + strconv.Itoa(R.WebServerPort))
	go GSwebserverStart(R.WebServerPort)

	// proxy.Listen();

	return R
}

func Stop() {
	fmt.Println("Stopping GateSentry " + GSVerString)
}
