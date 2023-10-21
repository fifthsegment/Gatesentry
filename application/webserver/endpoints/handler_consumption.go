package gatesentryWebserverEndpoints

import (
	"encoding/json"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

type Datareceiver struct {
	EnableUsers bool   `json:"EnableUsers"`
	Data        string `json:"Data"`
}

func GSApiConsumptionGET(data string, settings *gatesentry2storage.MapStore, runtime *gatesentryWebserverTypes.TemporaryRuntime) interface{} {
	temp := settings.Get("EnableUsers")
	enableusers := false
	if temp == "true" {
		enableusers = true
	}
	// ctx.JSON(struct {
	// 	EnableUsers bool
	// 	Data        string
	// }{Data: data, EnableUsers: enableusers})
	return struct {
		EnableUsers bool
		Data        string
	}{Data: data, EnableUsers: enableusers}

}

func GSApiConsumptionPOST(temp Datareceiver, settings *gatesentry2storage.MapStore, runtime *gatesentryWebserverTypes.TemporaryRuntime) interface{} {
	// data := string(R.GSUserGetDataJSON())
	// ctx.JSON(200, struct{Data string}{Data: data})

	enableusersstring := "false"
	if temp.EnableUsers {
		enableusersstring = "true"
	}
	settings.Update("EnableUsers", enableusersstring)
	users := []GatesentryTypes.GSUserPublic{}
	json.Unmarshal([]byte(temp.Data), &users)
	// R.AuthUsers
	// Run tests for deleted users first
	// todelete := []GSUserPublic{}
	for i := 0; i < len(runtime.AuthUsers); i++ {
		// current := R.AuthUsers[i];
		found := false
		for j := 0; j < len(users); j++ {
			if runtime.AuthUsers[i].User == users[j].User {
				found = true
			}
		}
		if !found {
			runtime.RemoveUser(runtime.AuthUsers[i])
		}
	}
	for i := 0; i < len(users); i++ {
		runtime.UpdateUser(users[i].User, users[i])
	}
	// ctx.JSON(struct{ Data string }{Data: "ok"})
	return struct{ Data string }{Data: "ok"}
}
