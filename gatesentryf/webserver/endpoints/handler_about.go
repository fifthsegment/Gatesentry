package gatesentryWebserverEndpoints

import (
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/kataras/iris/v12"
)

func GSApiAboutGET(ctx iris.Context, runtime *gatesentryWebserverTypes.TemporaryRuntime) {
	apikey := runtime.GetInstallationId()
	usagedata, msg := runtime.GetTotalConsumptionData()
	version := runtime.GetApplicationVersion()
	ctx.JSON(struct {
		Apikey              string
		Usagedata           string
		Additionalusagedata string
		Version             string
	}{Apikey: apikey, Usagedata: usagedata, Additionalusagedata: msg, Version: version})
}
