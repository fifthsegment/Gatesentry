package gatesentryWebserverEndpoints

import (
	"fmt"
	"strings"

	gatesentryWebserverBinarydata "bitbucket.org/abdullah_irfan/gatesentryf/webserver/binarydata"
	"github.com/kataras/iris/v12"
)

func GSVirtualStatic(ctx iris.Context) {
	requestedId := ctx.Params().Get("id")
	// fmt.Println(requestedId )
	data, err := gatesentryWebserverBinarydata.Asset("buildGoAsset/" + requestedId)
	_ = data
	if err != nil {
		// Asset was not found.
		fmt.Println("Asset was not found " + requestedId)
	}
	if strings.Contains(requestedId, ".js") {
		ctx.ContentType("application/javascript")
	} else if strings.Contains(requestedId, ".css") {
		ctx.ContentType("text/css")
	}

	ctx.Write((data))
	// ctx.Text(200, string(data))
}
