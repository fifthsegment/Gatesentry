package gatesentryf

import (
	"fmt"
	"os"
	"strconv"

	gatesentryWebserver "bitbucket.org/abdullah_irfan/gatesentryf/webserver"
)

var R *GSRuntime

func Start(webadminport int) (*GSRuntime, error) {
	GSVerString := GetApplicationVersion()
	fmt.Println("Starting GateSentry v " + GSVerString)
	R = &GSRuntime{
		WebServerPort:    webadminport,
		FilterFiles:      make(map[string]string),
		DNSServerChannel: make(chan int),

		// FileContents : make(map[string][]GSFILTERLINE),
	}
	if err := R.Init(); err != nil {
		return nil, err
	}
	// Validate and apply first-run authentication before any network service
	// starts. Unsafe or malformed unattended configuration must fail startup.
	if _, err := gatesentryWebserver.NewAuthManager(R.GSSettings, os.Getenv("GATESENTRY_BOOTSTRAP_FILE")); err != nil {
		return nil, fmt.Errorf("initialize administrator authentication: %w", err)
	}
	LoadFilters()
	fmt.Println("Starting GateSentry webserver on port " + strconv.Itoa(R.WebServerPort))
	go GSwebserverStart(R.WebServerPort)

	return R, nil
}

func Stop() {
	fmt.Println("Stopping GateSentry " + GSVerString)
}
