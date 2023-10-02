package gatesentryWebserverEndpoints

import (
	gatesentryWebserverFrontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
	"github.com/kataras/iris/v12"
)

func GSwebindex(ctx iris.Context) {
	data := gatesentryWebserverFrontend.GetIndexHtml()
	ctx.HTML(string(data))
}

func GSwebProtectedindex(ctx iris.Context) {
	username := ctx.Values().GetString("user")
	ctx.Write([]byte("Hello authenticated user: " + username))
}
