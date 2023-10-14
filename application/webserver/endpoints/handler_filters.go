package gatesentryWebserverEndpoints

// func ApiFiltersGET(ctx iris.Context, filters *[]gatesentryFilters.GSFilter) {
// 	comm := GatesentryTypes.GSWebServerCommunicator{Action: ""}
// 	x := gatesentryFilters.GetAPIResponse("GET /filters", *filters, ctx, &comm)
// 	ctx.JSON(x)
// }

// func GSApiFiltersGET(ctx iris.Context, filters *[]gatesentryFilters.GSFilter) {
// 	comm := GatesentryTypes.GSWebServerCommunicator{Action: ""}
// 	x := gatesentryFilters.GetAPIResponse("GET /filters", *filters, ctx, &comm)
// 	ctx.JSON(x)
// }

// func ApiFilterSingleGET(ctx iris.Context, filters *[]gatesentryFilters.GSFilter) {
// 	comm := GatesentryTypes.GSWebServerCommunicator{Action: ""}
// 	x := gatesentryFilters.GetAPIResponse("GET /filters/:id", *filters, ctx, &comm)
// 	ctx.JSON(x)
// }

// func ApiFilterSinglePOST(ctx iris.Context, filters *[]gatesentryFilters.GSFilter, initGatesentry func()) {
// 	comm := GatesentryTypes.GSWebServerCommunicator{Action: ""}
// 	gatesentryFilters.GetAPIResponse("POST /filters/:id", *filters, ctx, &comm)
// 	if comm.Action == "RESTART" {
// 		initGatesentry()
// 	}
// 	// The handler takes control from here so we don't need to write a response;
// 	// ctx.JSON(iris.StatusOK, x )
// }
