package gatesentryWebserverEndpoints

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"

	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

const ERROR_FAILED_VALIDATION = "Username or password too short. Username must be at least 3 characters and password must be at least 10 characters"

var errUserExists = errors.New("user already exists")

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

func GSApiUserCreate(userJson UserInputJsonSingle, settingsStore *gatesentry2storage.MapStore) (interface{}, error) {

	// check if username and password are greater than 3 characters
	if ValidateUserInputJsonSingle(userJson) == false {
		// HandleError(ctx, ERROR_FAILED_VALIDATION)
		// return
		return struct{ Error string }{Error: ERROR_FAILED_VALIDATION}, nil
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

	err := settingsStore.UpdateValue("authusers", func(existingJSON string) (string, error) {
		var existingUsers []GatesentryTypes.GSUser
		if existingJSON != "" {
			if err := json.Unmarshal([]byte(existingJSON), &existingUsers); err != nil {
				return "", err
			}
		}
		for _, user := range existingUsers {
			if user.User == newUser.User {
				return "", errUserExists
			}
		}
		usersJSON, err := json.Marshal(append(existingUsers, newUser))
		return string(usersJSON), err
	})
	if errors.Is(err, errUserExists) {
		return struct{ Error string }{Error: "User already exists"}, nil
	}
	if err != nil {
		return nil, err
	}
	// ctx.JSON(UserEndpointJsonOk{Ok: true})
	return UserEndpointJsonOk{Ok: true}, nil
}

func GSApiUserPUT(settingsStore *gatesentry2storage.MapStore, userJson UserInputJsonSingle) (interface{}, error) {

	if len(userJson.Password) > 0 && ValidateUserInputJsonSingle(userJson) == false {
		return struct{ Error string }{Error: ERROR_FAILED_VALIDATION}, nil
	}

	err := settingsStore.UpdateValue("authusers", func(existingJSON string) (string, error) {
		var existingUsers []GatesentryTypes.GSUser
		if existingJSON != "" {
			if err := json.Unmarshal([]byte(existingJSON), &existingUsers); err != nil {
				return "", err
			}
		}
		for i := range existingUsers {
			if existingUsers[i].User == userJson.Username {
				existingUsers[i].AllowAccess = userJson.AllowAccess
				if len(userJson.Password) > 0 {
					existingUsers[i].Base64String = base64.StdEncoding.EncodeToString([]byte(userJson.Username + ":" + userJson.Password))
				}
			}
		}
		usersJSON, err := json.Marshal(existingUsers)
		return string(usersJSON), err
	})
	if err != nil {
		return nil, err
	}

	return UserEndpointJsonOk{Ok: true}, nil
}

func GSApiUserDELETE(username string, settingsStore *gatesentry2storage.MapStore) (interface{}, error) {

	err := settingsStore.UpdateValue("authusers", func(existingJSON string) (string, error) {
		var existingUsers []GatesentryTypes.GSUser
		if existingJSON != "" {
			if err := json.Unmarshal([]byte(existingJSON), &existingUsers); err != nil {
				return "", err
			}
		}
		users := make([]GatesentryTypes.GSUser, 0, len(existingUsers))
		for _, user := range existingUsers {
			if user.User != username {
				users = append(users, user)
			}
		}
		usersJSON, err := json.Marshal(users)
		return string(usersJSON), err
	})
	if err != nil {
		return nil, err
	}
	// ctx.JSON(UserEndpointJsonOk{Ok: true})
	return UserEndpointJsonOk{Ok: true}, nil
}
