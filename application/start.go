package gatesentryf

import (
	"fmt"
	"strconv"
)

var R *GSRuntime

func Start(webadminport int) (*GSRuntime, error) {
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
	if err := R.Init(); err != nil {
		return nil, err
	}
	LoadFilters()
	// RegisterProxyHandlers();
	fmt.Println("Starting GateSentry webserver on port " + strconv.Itoa(R.WebServerPort))
	go GSwebserverStart(R.WebServerPort)

	// proxy.Listen();

	return R, nil
}

func Stop() {
	fmt.Println("Stopping GateSentry " + GSVerString)
}
