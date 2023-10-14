package gatesentryWebserverEndpoints

import gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

func GSApiAboutGET(runtime *gatesentryWebserverTypes.TemporaryRuntime) interface{} {
	apikey := runtime.GetInstallationId()
	usagedata, msg := runtime.GetTotalConsumptionData()
	version := runtime.GetApplicationVersion()

	return struct {
		Apikey              string
		Usagedata           string
		Additionalusagedata string
		Version             string `json:"version"`
	}{Apikey: apikey, Usagedata: usagedata, Additionalusagedata: msg, Version: version}
}
