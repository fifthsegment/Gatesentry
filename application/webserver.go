package gatesentryf

import (
	"fmt"
	"net"
	"strconv"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
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

	fmt.Println("Webserver is listening on : " + ggport)
	gatesentry2storage.SetBaseDir(GSBASEDIR)
	R.GSWebSettings = gatesentry2storage.NewMapStore("GSWebSettings", true)

	runtimeArgs := gatesentryWebserverTypes.InputArgs{
		GetUserGetJSON:          R.GSUserGetDataJSON,
		AuthUsers:               R.AuthUsers,
		RemoveUser:              R.RemoveUser,
		UpdateUser:              R.UpdateUser,
		GetInstallationId:       R.GetInstallationId,
		GetTotalConsumptionData: R.GetTotalConsumptionData,
		GetApplicationVersion:   R.GetApplicationVersion,
		Reload:                  R.Init,
	}
	runtime := gatesentryWebserverTypes.NewTemporaryRuntime(runtimeArgs)

	// gatesentryWebserver.RegisterEndpoints(app, settings, &R.Filters, R.Logger, runtime, R.BoundAddress)

	gatesentryWebserver.RegisterEndpointsStartServer(
		&R.Filters,
		runtime,
		R.Logger,
		R.DnsServerInfo,
		R.BoundAddress,
		strconv.Itoa(GSWebServerPort),
		R.GSSettings,
	)

	// app.Listen(":" + strconv.Itoa(GSWebServerPort))
}
