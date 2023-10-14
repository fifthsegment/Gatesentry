package gatesentryWebserver

// import (
// 	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
// 	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
// 	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
// 	gatesentryWebserverAuth "bitbucket.org/abdullah_irfan/gatesentryf/webserver/auth"
// 	gatesentryWebserverEndpoints "bitbucket.org/abdullah_irfan/gatesentryf/webserver/endpoints"
// 	gatesentryFrontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
// 	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

// 	"github.com/kataras/iris/v12"
// )

// var (
// 	settingsStore *gatesentryWebserverTypes.SettingsStore
// )

// func RegisterEndpoints(
// 	app *iris.Application,
// 	settings *gatesentryWebserverTypes.SettingsStore,
// 	filters *[]gatesentryFilters.GSFilter,
// 	logger *gatesentryLogger.Log,
// 	runtime *gatesentryWebserverTypes.TemporaryRuntime,
// 	_boundAddress *string,
// ) {
// 	settingsStore = settings
// 	gatesentryWebserverEndpoints.Init(settings)
// 	authentication, jwtMiddleware := gatesentryWebserverAuth.InitAuthMiddleware(settings.GetAdminUser(), settings.GetAdminPassword())

// 	app.HandleDir("/css", iris.Dir("./resources/css"))
// 	app.HandleDir("/js", iris.Dir("./resources/js"))

// 	app.Get("/admin", authentication, gatesentryWebserverEndpoints.GSwebProtectedindex)
// 	app.Get("/api/auth/verify", jwtMiddleware.Serve, gatesentryWebserverAuth.VerifyToken)
// 	app.Get("/api/filters", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		gatesentryWebserverEndpoints.ApiFiltersGET(ctx, filters)
// 	})
// 	app.Get("/api/filters/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		gatesentryWebserverEndpoints.ApiFilterSingleGET(ctx, filters)
// 	})
// 	app.Post("/api/filters/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		gatesentryWebserverEndpoints.ApiFilterSinglePOST(ctx, filters, settings.OnMajorSettingsChange)
// 	})
// 	app.Get("/api/settings/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		requestedId := ctx.Params().Get("id")
// 		output := gatesentryWebserverEndpoints.GSApiSettingsGET(requestedId, settings)
// 		ctx.JSON(output)
// 	})
// 	app.Post("/api/settings/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		requestedId := ctx.Params().Get("id")
// 		var temp gatesentryWebserverTypes.Datareceiver
// 		err := ctx.ReadJSON(&temp)
// 		if err != nil {
// 			return
// 		}

// 		output := gatesentryWebserverEndpoints.GSApiSettingsPOST(requestedId, settings, temp)
// 		ctx.JSON(output)
// 	})

// 	app.Get("/api/users", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		output := gatesentryWebserverEndpoints.GSApiUsersGET(runtime, settings.GetSettings("authusers"))
// 		ctx.JSON(output)
// 	})

// 	app.Put("/api/users", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		var userJson gatesentryWebserverEndpoints.UserInputJsonSingle
// 		err := ctx.ReadJSON(&userJson)

// 		if err != nil {
// 			return
// 		}
// 		output := gatesentryWebserverEndpoints.GSApiUserPUT(settings, userJson)
// 		ctx.JSON(output)
// 		runtime.Reload()
// 	})

// 	app.Delete("/api/users/:username", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		var username = ctx.Params().Get("username")
// 		output := gatesentryWebserverEndpoints.GSApiUserDELETE(username, settings)
// 		ctx.JSON(output)
// 		runtime.Reload()
// 	})

// 	app.Post("/api/users", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		var userJson gatesentryWebserverEndpoints.UserInputJsonSingle
// 		err := ctx.ReadJSON(&userJson)
// 		if err != nil {
// 			return
// 		}
// 		output := gatesentryWebserverEndpoints.GSApiUserCreate(userJson, settings)
// 		ctx.JSON(output)
// 		runtime.Reload()
// 	})

// 	app.Get("/api/consumption", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		data := string(runtime.GetUserGetJSON())
// 		output := gatesentryWebserverEndpoints.GSApiConsumptionGET(data, settings, runtime)
// 		ctx.JSON(output)
// 	})
// 	app.Post("/api/consumption", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		var temp gatesentryWebserverEndpoints.Datareceiver
// 		err := ctx.ReadJSON(&temp)
// 		if err != nil {
// 			return
// 		}
// 		output := gatesentryWebserverEndpoints.GSApiConsumptionPOST(temp, settings, runtime)
// 		ctx.JSON(output)
// 	})
// 	app.Get("/api/logs/:id", func(ctx iris.Context) {
// 		output := gatesentryWebserverEndpoints.ApiLogsGET(logger)
// 		ctx.JSON(output)
// 	})
// 	app.Get("/api/about/info", jwtMiddleware.Serve)

// 	app.Get("/api/dns/custom_entries", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		data := settings.Get("DNS_custom_entries")
// 		output := gatesentryWebserverEndpoints.GSApiDNSEntriesCustom(data, settings, runtime)
// 		ctx.JSON(output)
// 	})

// 	app.Post("/api/dns/custom_entries", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		var customEntries []gatesentryTypes.DNSCustomEntry
// 		err := ctx.ReadJSON(&customEntries)
// 		if err != nil {
// 			return
// 		}
// 		output := gatesentryWebserverEndpoints.GSApiDNSSaveEntriesCustom(customEntries, settings, runtime)
// 		ctx.JSON(output)
// 	})

// 	app.Get("/api/stats", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		fromTimeParam := ctx.URLParam("fromTime")
// 		output := gatesentryWebserverEndpoints.ApiGetStats(fromTimeParam, logger)
// 		ctx.JSON(output)
// 	})

// 	app.Get("/api/status", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		output := gatesentryWebserverEndpoints.ApiGetStatus(logger, _boundAddress)
// 		ctx.JSON(output)
// 	})

// 	app.Get("/api/stats/byUrl", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		output := gatesentryWebserverEndpoints.ApiGetStatsByURL(logger)
// 		ctx.JSON(output)
// 	})

// 	app.Get("/api/toggleServer/:id", jwtMiddleware.Serve, func(ctx iris.Context) {
// 		id := ctx.Params().Get("id")
// 		output := gatesentryWebserverEndpoints.ApiToggleServer(id, logger)
// 		ctx.JSON(output)
// 	})

// 	app.Post("/api/verify/certificate", jwtMiddleware.Serve, gatesentryWebserverEndpoints.ApiVerifyCert)

// 	app.Get("/", gatesentryWebserverEndpoints.GSwebindex)
// 	app.Get("/virtual/:id", gatesentryWebserverEndpoints.GSVirtualStatic)

// 	app.Post("/api/auth/token", gatesentryWebserverEndpoints.GSGetToken)
// 	app.Post("/api/auth/token", gatesentryWebserverEndpoints.GSGetToken)

// 	app.Get("/home", gatesentryWebserverEndpoints.GetHomeEndpoint)

// 	app.Get("/home/:id", func(ctx iris.Context) {
// 		certificateData := []byte(settingsStore.Get("capem"))
// 		gatesentryWebserverEndpoints.GetCertificateEndpoint(ctx, certificateData)
// 	})

// 	app.HandleDir("/fs", gatesentryFrontend.GetIrisHandler(), iris.DirOptions{
// 		IndexName: "index.html",
// 		ShowList:  false,
// 		SPA:       true,
// 	})

// }
