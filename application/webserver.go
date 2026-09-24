package gatesentryf

import (
	"fmt"
	"net/http"
	"strconv"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryWebserver "bitbucket.org/abdullah_irfan/gatesentryf/webserver"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

func webServerHandler(port int) (http.Handler, error) {
	GSWebServerPort := port
	ggport := strconv.Itoa(GSWebServerPort)

	basePath := GetBasePath()
	fmt.Println("Webserver is listening on : " + ggport + " (base path: " + basePath + ")")
	gatesentry2storage.SetBaseDir(GSBASEDIR)
	var err error
	R.GSWebSettings, err = gatesentry2storage.OpenMapStore("GSWebSettings", true)
	if err != nil {
		return nil, fmt.Errorf("open web settings: %w", err)
	}

	runtimeArgs := gatesentryWebserverTypes.InputArgs{
		GetUserGetJSON:          R.GSUserGetDataJSON,
		AuthUsers:               R.AuthUsers,
		RemoveUser:              R.RemoveUser,
		UpdateUser:              R.UpdateUser,
		GetInstallationId:       R.GetInstallationId,
		GetTotalConsumptionData: R.GetTotalConsumptionData,
		GetApplicationVersion:   R.GetApplicationVersion,
		GetProxyTraffic:         proxyTrafficSnapshot,
		Reload: func() {
			if err := R.Init(); err != nil {
				fmt.Printf("Unable to reload GateSentry: %v\n", err)
			}
		},
	}
	runtime := gatesentryWebserverTypes.NewTemporaryRuntime(runtimeArgs)

	return gatesentryWebserver.RegisterEndpointsHandler(
		&R.Filters,
		runtime,
		R.Logger,
		R.DnsServerInfo,
		R.BoundAddress,
		strconv.Itoa(GSWebServerPort),
		R.GSSettings,
		basePath,
		R.GSDevices,
		GSBASEDIR,
	)
}
