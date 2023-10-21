package gatesentryf

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

var GSUserDataSaverRunning bool

func (R *GSRuntime) GSUserRunDataSaver() {
	if R.GSUserDataSaverRunning {
		log.Printf("Data saver is already running")
		return
	}
	log.Println("Starting data saver")
	R.GSUserDataSaverRunning = true
	go R.GSUserDataSaverMonitor()
}

func (R *GSRuntime) GSUserDataSaverMonitor() {
	t := time.NewTicker(time.Second * 60 * 5)
	for {
		R.GSUserDataSaver()
		<-t.C
	}
}

/**
* Saves user bandwidth data to the disk
 */
func (R *GSRuntime) GSUserDataSaver() {
	tempusers := R.AuthUsers
	b, err := json.Marshal(tempusers)

	if err != nil {
		return
	}
	log.Println("Saving user data")
	// log.Println("A save of user data was succesful")
	R.GSSettings.Update("authusers", string(b))
}

func (R *GSRuntime) UpdateUserData(username string, data uint64) {
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User == username {
			R.AuthUsers[i].DataConsumed += data
		}
	}
}

func (R *GSRuntime) GSUserGetDataJSON() []byte {
	temp := []GatesentryTypes.GSUserPublic{}
	for i := 0; i < len(R.AuthUsers); i++ {
		tuser := GatesentryTypes.GSUserPublic{User: R.AuthUsers[i].User, DataConsumed: R.AuthUsers[i].DataConsumed, AllowAccess: R.AuthUsers[i].AllowAccess}
		temp = append(temp, tuser)
	}
	b, err := json.Marshal(temp)
	if err != nil {
		return nil
	}
	return b
}

func (R *GSRuntime) LoadUsers() {
	log.Println("Load users")
	usersString := R.GSSettings.Get("authusers")

	users := []GatesentryTypes.GSUser{}
	json.Unmarshal([]byte(usersString), &users)

	R.AuthUsers = users
}

func (R *GSRuntime) RemoveUser(data GatesentryTypes.GSUser) {
	log.Println("Removing username = " + data.User)

	newusers := []GatesentryTypes.GSUser{}
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User != data.User {
			newusers = append(newusers, R.AuthUsers[i])
		}
	}

	R.AuthUsers = newusers
	R.GSUserDataSaver()
}

func (R *GSRuntime) UpdatePassword(username string, password string) {
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User == username {
			R.AuthUsers[i].Pass = password
		}
	}
	R.GSUserDataSaver()
	R.Init()
}

func (R *GSRuntime) UpdateUser(username string, data GatesentryTypes.GSUserPublic) {
	// R.LoadUsers();
	found := false
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User == username {
			R.AuthUsers[i].AllowAccess = data.AllowAccess
			if data.Password != "" {
				R.UpdatePassword(data.User, data.Password)
			}
			found = true
		}
	}
	if !found {
		R.AddUser(data.User, data.Password)
	}
	R.GSUserDataSaver()
}

func (R *GSRuntime) AddUser(user string, pass string) bool {
	log.Println("Adding a new user")
	// R.LoadUsers();
	for i := 0; i < len(R.AuthUsers); i++ {
		auser := R.AuthUsers[i]
		if user == auser.User {
			log.Println("User already exists")
			return false
		}
	}
	if len(user) == 0 || len(pass) == 0 {
		return false
	}
	Guser := GatesentryTypes.GSUser{User: user, Pass: pass}
	tempusers := append(R.AuthUsers, Guser)

	b, err := json.Marshal(tempusers)
	if err != nil {
		return false
	}
	R.GSSettings.Update("authusers", string(b))
	R.LoadUsers()
	return true
	// R.GSSettings.Update("authusers", string(b))
}

func (R *GSRuntime) IsUserValid(base64string string) bool {
	base64Parts := strings.Split(base64string, " ")
	base64Main := base64Parts[1]
	for i := 0; i < len(R.AuthUsers); i++ {
		user := R.AuthUsers[i]
		if user.Base64String == base64Main {
			return true
		}
	}
	return false
}

func (R *GSRuntime) IsUserActive(username string) bool {
	log.Println("Checking if user = " + username + " is valid")
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User == username {
			return R.AuthUsers[i].AllowAccess
		}
	}
	return false
}

func (R *GSRuntime) UserExists(username string) bool {
	log.Println("Checking if user = " + username + " exists")
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User == username {
			return true
		}
	}
	return false
}
