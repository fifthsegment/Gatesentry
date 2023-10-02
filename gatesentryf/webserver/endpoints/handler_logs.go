package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	"github.com/kataras/iris/v12"
)

func ApiLogsGET(ctx iris.Context, logger *gatesentryLogger.Log) {
	items := `[` + logger.GetLog(1) + `]`
	ctx.JSON(struct{ Items string }{Items: items})
}
