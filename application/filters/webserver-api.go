package gatesentry2filters

import (
	"encoding/json"
	"log"

	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	"github.com/kataras/iris/v12"
)

func GetAPIResponse(endpoint string, Filters []GSFilter, ctx iris.Context, comm *GatesentryTypes.GSWebServerCommunicator) interface{} {
	switch endpoint {
	case "GET /filters":
		x := []GSAPIStructFilter{}
		for _, v := range Filters {
			x = append(x, GSAPIStructFilter{Id: v.Id, Name: v.FilterName, Handles: v.Handles, Entries: v.FileContents})
		}
		return x
		break
	case "GET /filters/:id":
		x := []GSAPIStructFilter{}
		//requestedId := ctx.Param("id")
		requestedId := (ctx).Params().Get("id")
		for _, v := range Filters {
			if v.Id == requestedId {
				x = append(x, GSAPIStructFilter{Id: v.Id, Description: v.Description, HasStrength: v.HasStrength, Name: v.FilterName, Handles: v.Handles, Entries: v.FileContents})
			}
		}
		return x
		break
	case "POST /filters/:id":
		x := []GSAPIStructFilter{}
		requestedId := (ctx).Params().Get("id")
		for _, v := range Filters {
			if v.Id == requestedId {
				var dataRecv []GSFILTERLINE
				err := (ctx).ReadJSON(&dataRecv)
				log.Println(dataRecv)
				if err != nil {
					log.Println("Webserver-api POST /filters/:id" + err.Error())
				} else {

					(ctx).JSON(struct{ Response string }{Response: "Ok!"})
					// log.Println( dataRecv );
					data, _ := json.MarshalIndent(dataRecv, "", "  ")
					log.Println(string(data))
					GSSaveFilterFile(v.FileName, string(data))
					comm.Action = "RESTART"
					// if str, ok := dataRecv.(string); ok {
					//     fmt.Println( str );
					// } else {
					//     // not string
					// }
				}
				// x = append( x , GSAPIStructFilter{Id: v.Id, Name: v.FilterName, Handles: v.Handles, Entries: v.FileContents} );
			}
		}
		return x
		break
	}
	return nil

}
