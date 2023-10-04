package gatesentryWebserverEndpoints

import (
	"encoding/json"

	structures "bitbucket.org/abdullah_irfan/gatesentryf/structures"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

	"github.com/kataras/iris/v12"
)

func GSApiConsumptionGET(ctx iris.Context, settings *gatesentryWebserverTypes.SettingsStore, runtime *gatesentryWebserverTypes.TemporaryRuntime) {
	data := string(runtime.GetUserGetJSON())
	temp := settings.Get("EnableUsers")
	enableusers := false
	if temp == "true" {
		enableusers = true
	}
	ctx.JSON(struct {
		EnableUsers bool
		Data        string
	}{Data: data, EnableUsers: enableusers})
}

func GSApiConsumptionPOST(ctx iris.Context, settings *gatesentryWebserverTypes.SettingsStore, runtime *gatesentryWebserverTypes.TemporaryRuntime) {
	// data := string(R.GSUserGetDataJSON())
	// ctx.JSON(200, struct{Data string}{Data: data})
	type Datareceiver struct {
		EnableUsers bool   `json:EnableUsers`
		Data        string `json:Data`
	}
	var temp Datareceiver
	err := ctx.ReadJSON(&temp)
	if err != nil {

	}
	enableusersstring := "false"
	if temp.EnableUsers {
		enableusersstring = "true"
	}
	settings.Set("EnableUsers", enableusersstring)
	users := []structures.GSUserPublic{}
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
	ctx.JSON(struct{ Data string }{Data: "ok"})
}
