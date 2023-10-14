package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

func ApiLogsGET(logger *gatesentryLogger.Log) interface{} {
	items := `[` + logger.GetLog(1) + `]`
	// ctx.JSON(struct{ Items string }{Items: items})
	return struct{ Items string }{Items: items}
}
