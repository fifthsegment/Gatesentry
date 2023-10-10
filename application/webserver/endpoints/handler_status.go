package gatesentryWebserverEndpoints

import (
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	"github.com/kataras/iris/v12"
)

type StatusResponse struct {
	ServerUrl string `json:"server_url"`
}

func ApiGetStatus(ctx iris.Context, logger *gatesentryLogger.Log, boundAddress *string) {
	// get current server ip

	response := StatusResponse{
		ServerUrl: *boundAddress,
	}

	ctx.JSON(response)
}
