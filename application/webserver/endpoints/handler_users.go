package gatesentryWebserverEndpoints

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"

	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

const ERROR_FAILED_VALIDATION = "Username or password too short. Username must be at least 3 characters and password must be at least 10 characters"

type UserEndpointJson struct {
	Users []GatesentryTypes.GSUser `json:"users"`
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

// func HandleError(ctx iris.Context, errorMessage string) {
// 	ctx.JSON(UserEndpointJsonError{Ok: false, Error: errorMessage})
// 	ctx.StatusCode(iris.StatusBadRequest)
// }

func GSApiUsersGET(runtime *gatesentryWebserverTypes.TemporaryRuntime, usersString string) interface{} {
	users := []GatesentryTypes.GSUser{}
	json.Unmarshal([]byte(usersString), &users)

	return UserEndpointJson{Users: users}
}

func GSApiUserCreate(userJson UserInputJsonSingle, settingsStore *gatesentry2storage.MapStore) interface{} {

	// check if username and password are greater than 3 characters
	if ValidateUserInputJsonSingle(userJson) == false {
		// HandleError(ctx, ERROR_FAILED_VALIDATION)
		// return
		return struct{ Error string }{Error: ERROR_FAILED_VALIDATION}
	}

	// if err != nil {
	// 	HandleError(ctx, err.Error())
	// 	return
	// }

	var newUser = GatesentryTypes.GSUser{
		// make the username lowercase
		User:         strings.ToLower(userJson.Username),
		Pass:         "",
		Base64String: base64.StdEncoding.EncodeToString([]byte(userJson.Username + ":" + userJson.Password)),
		AllowAccess:  userJson.AllowAccess,
	}

	var existingJson = settingsStore.Get("authusers")
	var existingUsers []GatesentryTypes.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// check if user exists
	for _, user := range existingUsers {
		if user.User == newUser.User {
			// HandleError(ctx, "User already exists")
			// return
			return struct{ Error string }{Error: "User already exists"}
		}
	}

	var newUsers = append(existingUsers, newUser)

	usersString, err := json.Marshal(newUsers)

	if err != nil {
		// HandleError(ctx, err.Error())
		// return
		return struct{ Error string }{Error: err.Error()}
	}

	log.Println(fmt.Sprintf("Users: %s", usersString))
	settingsStore.Update("authusers", string(usersString))
	// ctx.JSON(UserEndpointJsonOk{Ok: true})
	return UserEndpointJsonOk{Ok: true}
}

func GSApiUserPUT(settingsStore *gatesentry2storage.MapStore, userJson UserInputJsonSingle) interface{} {

	if len(userJson.Password) > 0 && ValidateUserInputJsonSingle(userJson) == false {
		return struct{ Error string }{Error: ERROR_FAILED_VALIDATION}
	}

	var existingJson = settingsStore.Get("authusers")
	var existingUsers []GatesentryTypes.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// update the user in existing users
	var users []GatesentryTypes.GSUser
	for _, user := range existingUsers {
		if user.User == userJson.Username {
			user.AllowAccess = userJson.AllowAccess
			if len(userJson.Password) > 0 {
				user.Base64String = base64.StdEncoding.EncodeToString([]byte(userJson.Username + ":" + userJson.Password))
			}
		}
		users = append(users, user)
	}

	usersString, err := json.Marshal(users)
	if err != nil {
		log.Println(fmt.Sprintf("Error marshalling users: %s", err.Error()))
		return struct{ Error string }{Error: err.Error()}
	}
	log.Printf("Users: %s", usersString)
	settingsStore.Update("authusers", string(usersString))

	return UserEndpointJsonOk{Ok: true}
}

func GSApiUserDELETE(username string, settingsStore *gatesentry2storage.MapStore) interface{} {

	var existingJson = settingsStore.Get("authusers")
	var existingUsers []GatesentryTypes.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// update the user in existing users
	var users []GatesentryTypes.GSUser
	for _, user := range existingUsers {
		if user.User != username {
			users = append(users, user)
		}
	}

	usersString, err := json.Marshal(users)
	if err != nil {
		// HandleError(ctx, err.Error())
		// return
		return struct{ Error string }{Error: err.Error()}
	}

	settingsStore.Update("authusers", string(usersString))
	// ctx.JSON(UserEndpointJsonOk{Ok: true})
	return UserEndpointJsonOk{Ok: true}
}
