package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	"github.com/kataras/iris/v12"
)

func ApiToggleServer(ctx iris.Context, logger *gatesentryLogger.Log) {
	id := ctx.Params().Get("id")

	switch id {
	case "dns":
		ctx.StatusCode(iris.StatusOK)
		ctx.JSON(iris.Map{"success": true})
	case "http":
		ctx.StatusCode(iris.StatusOK)
		ctx.JSON(iris.Map{"success": true})
	default:
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid id"})
		return
	}

}
