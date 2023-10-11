package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"fmt"
	"log"

	structures "bitbucket.org/abdullah_irfan/gatesentryf/structures"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

	"github.com/kataras/iris/v12"
)

type UserEndpointJson struct {
	Users []structures.GSUser `json:"users"`
}

type UserEndpointJsonOk struct {
	Ok bool `json:"ok"`
}

func GSApiUsersGET(ctx iris.Context, runtime *gatesentryWebserverTypes.TemporaryRuntime, usersString string) {
	users := []structures.GSUser{}
	json.Unmarshal([]byte(usersString), &users)

	ctx.JSON(UserEndpointJson{Users: users})
}

func GSApiUsersPOST(ctx iris.Context, settingsStore *gatesentryWebserverTypes.SettingsStore) {
	var jsonData UserEndpointJson
	err := ctx.ReadJSON(&jsonData)
	if err != nil {
		log.Println(fmt.Sprintf("Error reading json: %s", err.Error()))

		ctx.StatusCode(iris.StatusBadRequest)
		return
	}
	var users []structures.GSUser = jsonData.Users

	usersString, err := json.Marshal(users)
	if err != nil {
		log.Println(fmt.Sprintf("Error marshalling users: %s", err.Error()))
		ctx.StatusCode(iris.StatusBadRequest)
		return
	}

	settingsStore.SetSettings("authusers", string(usersString))

	ctx.JSON(UserEndpointJsonOk{Ok: true})

}
