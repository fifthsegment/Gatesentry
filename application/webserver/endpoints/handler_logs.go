package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

func ApiLogsGET(logger *gatesentryLogger.Log) interface{} {
	items := `[` + logger.GetLog() + `]`
	// ctx.JSON(struct{ Items string }{Items: items})
	return struct{ Items string }{Items: items}
}

func ApiLogsSearchGET(logger *gatesentryLogger.Log, search string) interface{} {
	items := `[` + logger.GetLogSearch(search) + `]`
	// ctx.JSON(struct{ Items string }{Items: items})
	return struct{ Items string }{Items: items}
}
