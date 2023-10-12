package gatesentryWebserverEndpoints

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	structures "bitbucket.org/abdullah_irfan/gatesentryf/structures"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"

	"github.com/kataras/iris/v12"
)

const ERROR_FAILED_VALIDATION = "Username or password too short. Username must be at least 3 characters and password must be at least 10 characters"

type UserEndpointJson struct {
	Users []structures.GSUser `json:"users"`
}

type UserInputJsonSingle struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	AllowAccess bool   `json:"allowaccess"`
}

type UserEndpointJsonOk struct {
	Ok bool `json:"ok"`
}

type UserEndpointJsonError struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
}

func ValidateUserInputJsonSingle(userJson UserInputJsonSingle) bool {
	if len(userJson.Username) < 3 || len(userJson.Password) < 10 {
		return false
	}
	return true
}

func HandleError(ctx iris.Context, errorMessage string) {
	ctx.JSON(UserEndpointJsonError{Ok: false, Error: errorMessage})
	ctx.StatusCode(iris.StatusBadRequest)
}

func GSApiUsersGET(ctx iris.Context, runtime *gatesentryWebserverTypes.TemporaryRuntime, usersString string) {
	users := []structures.GSUser{}
	json.Unmarshal([]byte(usersString), &users)

	ctx.JSON(UserEndpointJson{Users: users})
}

func GSApiUserCreate(ctx iris.Context, settingsStore *gatesentryWebserverTypes.SettingsStore) {
	var userJson UserInputJsonSingle
	err := ctx.ReadJSON(&userJson)
	// check if username and password are greater than 3 characters
	if ValidateUserInputJsonSingle(userJson) == false {
		HandleError(ctx, ERROR_FAILED_VALIDATION)
		return
	}

	if err != nil {
		HandleError(ctx, err.Error())
		return
	}

	var newUser = structures.GSUser{
		// make the username lowercase
		User:         strings.ToLower(userJson.Username),
		Pass:         "",
		Base64String: base64.StdEncoding.EncodeToString([]byte(userJson.Password)),
		AllowAccess:  userJson.AllowAccess,
	}

	var existingJson = settingsStore.GetSettings("authusers")
	var existingUsers []structures.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// check if user exists
	for _, user := range existingUsers {
		if user.User == newUser.User {
			HandleError(ctx, "User already exists")
			return
		}
	}

	var newUsers = append(existingUsers, newUser)

	usersString, err := json.Marshal(newUsers)

	if err != nil {
		HandleError(ctx, err.Error())
		return
	}

	log.Println(fmt.Sprintf("Users: %s", usersString))
	settingsStore.SetSettings("authusers", string(usersString))
	ctx.JSON(UserEndpointJsonOk{Ok: true})
}

func GSApiUserPUT(ctx iris.Context, settingsStore *gatesentryWebserverTypes.SettingsStore) {
	var userJson UserInputJsonSingle
	err := ctx.ReadJSON(&userJson)

	if err != nil {
		HandleError(ctx, err.Error())
		return
	}

	if len(userJson.Password) > 0 && ValidateUserInputJsonSingle(userJson) == false {
		HandleError(ctx, ERROR_FAILED_VALIDATION)
		return
	}

	var existingJson = settingsStore.GetSettings("authusers")
	var existingUsers []structures.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// update the user in existing users
	var users []structures.GSUser
	for _, user := range existingUsers {
		if user.User == userJson.Username {
			user.AllowAccess = userJson.AllowAccess
			if len(userJson.Password) > 0 {
				user.Base64String = base64.StdEncoding.EncodeToString([]byte(userJson.Password))
			}
		}
		users = append(users, user)
	}

	usersString, err := json.Marshal(users)
	if err != nil {
		log.Println(fmt.Sprintf("Error marshalling users: %s", err.Error()))
		HandleError(ctx, err.Error())
		return
	}
	log.Printf("Users: %s", usersString)
	settingsStore.SetSettings("authusers", string(usersString))

	ctx.JSON(UserEndpointJsonOk{Ok: true})
}

func GSApiUserDELETE(ctx iris.Context, settingsStore *gatesentryWebserverTypes.SettingsStore) {
	var username = ctx.Params().Get("username")

	var existingJson = settingsStore.GetSettings("authusers")
	var existingUsers []structures.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// update the user in existing users
	var users []structures.GSUser
	for _, user := range existingUsers {
		if user.User != username {
			users = append(users, user)
		}
	}

	usersString, err := json.Marshal(users)
	if err != nil {
		HandleError(ctx, err.Error())
		return
	}

	settingsStore.SetSettings("authusers", string(usersString))
	ctx.JSON(UserEndpointJsonOk{Ok: true})
}
