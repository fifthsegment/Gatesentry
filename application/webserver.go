package gatesentryf

import (
	"fmt"
	"net"
	"strconv"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserver "bitbucket.org/abdullah_irfan/gatesentryf/webserver"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

func GSwebserverStart(port int) {

	GSWebServerPort := port
	ggport := strconv.Itoa(GSWebServerPort)
	t := time.NewTicker(time.Second * 10)
	portavailable := false
	for {
		fmt.Println("Checking if port is available")
		ln, err := net.Listen("tcp", ":"+ggport)
		if err != nil {
			fmt.Println("Port is not open for webserver")
		} else {
			portavailable = true
			err = ln.Close()
		}

		if portavailable {
			break
		}
		<-t.C
	}

	basePath := GetBasePath()
	fmt.Println("Webserver is listening on : " + ggport + " (base path: " + basePath + ")")
	gatesentry2storage.SetBaseDir(GSBASEDIR)
	R.GSWebSettings = gatesentry2storage.NewMapStore("GSWebSettings", true)

	runtimeArgs := gatesentryWebserverTypes.InputArgs{
		GetUserGetJSON:          R.GSUserGetDataJSON,
		GetAuthUsers:            func() []GatesentryTypes.GSUser { return R.AuthUsers },
		AuthUsers:               R.AuthUsers,
		RemoveUser:              R.RemoveUser,
		UpdateUser:              R.UpdateUser,
		GetInstallationId:       R.GetInstallationId,
		GetTotalConsumptionData: R.GetTotalConsumptionData,
		GetApplicationVersion:   R.GetApplicationVersion,
		Reload:                  R.Init,
	}
	runtime := gatesentryWebserverTypes.NewTemporaryRuntime(runtimeArgs)

	// Use the shared domain list manager from R (created in Start())
	dlManager := R.DomainListManager

	// Create the rule manager and wire in the domain list manager for DomainLists lookups
	ruleMgr := NewRuleManager(R.GSSettings)
	ruleMgr.SetDomainListManager(dlManager)

	gatesentryWebserver.RegisterEndpointsStartServer(
		&R.Filters,
		runtime,
		R.Logger,
		R.DnsServerInfo,
		R.BoundAddress,
		strconv.Itoa(GSWebServerPort),
		R.GSSettings,
		ruleMgr,
		dlManager,
		basePath,
	)

	// app.Listen(":" + strconv.Itoa(GSWebServerPort))
}
