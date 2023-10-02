package gatesentryWebserver

import (
	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryWebserverAuth "bitbucket.org/abdullah_irfan/gatesentryf/webserver/auth"
	gatesentryWebserverEndpoints "bitbucket.org/abdullah_irfan/gatesentryf/webserver/endpoints"
	gatesentryFrontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

	"github.com/kataras/iris/v12"
)

var (
	settingsStore *gatesentryWebserverTypes.SettingsStore
)

func RegisterEndpoints(
	app *iris.Application,
	settings *gatesentryWebserverTypes.SettingsStore,
	filters *[]gatesentryFilters.GSFilter,
	logger *gatesentryLogger.Log,
	runtime *gatesentryWebserverTypes.TemporaryRuntime,
) {
	settingsStore = settings
	gatesentryWebserverEndpoints.Init(settings)
	authentication, jwtMiddleware := gatesentryWebserverAuth.InitAuthMiddleware(settings.WebGet("admin"), settings.WebGet("pass"))

	app.HandleDir("/css", iris.Dir("./resources/css"))
	app.HandleDir("/js", iris.Dir("./resources/js"))

	app.Get("/admin", authentication, gatesentryWebserverEndpoints.GSwebProtectedindex)
	app.Get("/api/auth/verify", jwtMiddleware.Serve, gatesentryWebserverAuth.VerifyToken)
	app.Get("/api/filters", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.ApiFiltersGET(ctx, filters)
	})
	app.Get("/api/filters/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.ApiFilterSingleGET(ctx, filters)
	})
	app.Post("/api/filters/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.ApiFilterSinglePOST(ctx, filters, settings.OnMajorSettingsChange)
	})
	app.Get("/api/settings/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.GSApiSettingsGET(ctx, settings)
	})
	app.Post("/api/settings/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.GSApiSettingsPOST(ctx, settings)
	})
	app.Get("/api/consumption", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.GSApiConsumptionGET(ctx, settings, runtime)
	})
	app.Post("/api/consumption", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.GSApiConsumptionPOST(ctx, settings, runtime)
	})
	app.Get("/api/logs/:id", func(ctx iris.Context) {
		gatesentryWebserverEndpoints.ApiLogsGET(ctx, logger)
	})
	app.Get("/api/about/info", jwtMiddleware.Serve)

	app.Get("/api/stats", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.ApiGetStats(ctx, logger)
	})

	app.Get("/api/stats/byUrl", jwtMiddleware.Serve, func(ctx iris.Context) {
		gatesentryWebserverEndpoints.ApiGetStatsByURL(ctx, logger)
	})

	app.Post("/api/verify/certificate", jwtMiddleware.Serve, gatesentryWebserverEndpoints.ApiVerifyCert)

	app.Get("/", gatesentryWebserverEndpoints.GSwebindex)
	app.Get("/virtual/:id", gatesentryWebserverEndpoints.GSVirtualStatic)

	app.Post("/api/auth/token", gatesentryWebserverEndpoints.GSGetToken)
	app.Post("/api/auth/token", gatesentryWebserverEndpoints.GSGetToken)

	app.Get("/home", gatesentryWebserverEndpoints.GetHomeEndpoint)

	app.Get("/home/:id", func(ctx iris.Context) {
		certificateData := []byte(settingsStore.Get("capem"))
		gatesentryWebserverEndpoints.GetCertificateEndpoint(ctx, certificateData)
	})

	app.HandleDir("/fs", gatesentryFrontend.GetIrisHandler(), iris.DirOptions{
		IndexName: "index.html",
		ShowList:  false,
		SPA:       true,
	})

}
