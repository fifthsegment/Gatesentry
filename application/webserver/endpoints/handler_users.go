package gatesentryWebserverEndpoints

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"strings"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
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

func GSApiUsersGET(users []GatesentryTypes.GSUser) interface{} {
	if users == nil {
		users = []GatesentryTypes.GSUser{}
	}
	return UserEndpointJson{Users: users}
}

func GSApiUserCreate(userJson UserInputJsonSingle, settingsStore *gatesentry2storage.MapStore) interface{} {

	// check if username and password are greater than 3 characters
	if !ValidateUserInputJsonSingle(userJson) {
		return UserEndpointJsonError{Ok: false, Error: ERROR_FAILED_VALIDATION}
	}

	normalizedUsername := strings.ToLower(userJson.Username)

	var newUser = GatesentryTypes.GSUser{
		User:         normalizedUsername,
		Pass:         "",
		Base64String: base64.StdEncoding.EncodeToString([]byte(normalizedUsername + ":" + userJson.Password)),
		AllowAccess:  userJson.AllowAccess,
	}

	var existingJson = settingsStore.Get("authusers")
	var existingUsers []GatesentryTypes.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// check if user exists
	for _, user := range existingUsers {
		if user.User == newUser.User {
			return UserEndpointJsonError{Ok: false, Error: "User already exists"}
		}
	}

	var newUsers = append(existingUsers, newUser)

	usersString, err := json.Marshal(newUsers)
	if err != nil {
		return UserEndpointJsonError{Ok: false, Error: err.Error()}
	}

	log.Printf("Users: %s", usersString)
	settingsStore.Update("authusers", string(usersString))
	return UserEndpointJsonOk{Ok: true}
}

func GSApiUserPUT(settingsStore *gatesentry2storage.MapStore, userJson UserInputJsonSingle) interface{} {

	if len(userJson.Password) > 0 && !ValidateUserInputJsonSingle(userJson) {
		return UserEndpointJsonError{Ok: false, Error: ERROR_FAILED_VALIDATION}
	}

	normalizedUsername := strings.ToLower(userJson.Username)

	var existingJson = settingsStore.Get("authusers")
	var existingUsers []GatesentryTypes.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// update the user in existing users
	found := false
	var users []GatesentryTypes.GSUser
	for _, user := range existingUsers {
		if user.User == normalizedUsername {
			found = true
			user.AllowAccess = userJson.AllowAccess
			if len(userJson.Password) > 0 {
				user.Base64String = base64.StdEncoding.EncodeToString([]byte(normalizedUsername + ":" + userJson.Password))
			}
		}
		users = append(users, user)
	}

	if !found {
		return UserEndpointJsonError{Ok: false, Error: "User not found"}
	}

	usersString, err := json.Marshal(users)
	if err != nil {
		log.Printf("Error marshalling users: %s", err.Error())
		return UserEndpointJsonError{Ok: false, Error: err.Error()}
	}
	log.Printf("Users: %s", usersString)
	settingsStore.Update("authusers", string(usersString))

	return UserEndpointJsonOk{Ok: true}
}

func GSApiUserDELETE(username string, settingsStore *gatesentry2storage.MapStore) interface{} {

	normalizedUsername := strings.ToLower(username)

	var existingJson = settingsStore.Get("authusers")
	var existingUsers []GatesentryTypes.GSUser
	json.Unmarshal([]byte(existingJson), &existingUsers)

	// remove the user from existing users
	found := false
	var users []GatesentryTypes.GSUser
	for _, user := range existingUsers {
		if user.User != normalizedUsername {
			users = append(users, user)
		} else {
			found = true
		}
	}

	if !found {
		return UserEndpointJsonError{Ok: false, Error: "User not found"}
	}

	usersString, err := json.Marshal(users)
	if err != nil {
		return UserEndpointJsonError{Ok: false, Error: err.Error()}
	}

	settingsStore.Update("authusers", string(usersString))
	return UserEndpointJsonOk{Ok: true}
}
