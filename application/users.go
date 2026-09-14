package gatesentryf

import (
	"encoding/json"
	"fmt"
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
	R.usersMu.RLock()
	consumption := make(map[string]uint64, len(R.AuthUsers))
	for _, user := range R.AuthUsers {
		consumption[user.User] = user.DataConsumed
	}
	R.usersMu.RUnlock()
	log.Println("Saving user data")
	// Merge only the runtime-owned counters. Durable membership, credentials,
	// and access flags remain owned by the atomic API/storage transaction.
	if err := R.GSSettings.UpdateValue("authusers", func(current string) (string, error) {
		users := []GatesentryTypes.GSUser{}
		if current != "" {
			if err := json.Unmarshal([]byte(current), &users); err != nil {
				return "", fmt.Errorf("parse durable users: %w", err)
			}
		}
		for i := range users {
			if consumed, ok := consumption[users[i].User]; ok {
				users[i].DataConsumed = consumed
			}
		}
		b, err := json.Marshal(users)
		return string(b), err
	}); err != nil {
		log.Printf("Unable to save user data: %v", err)
	}
}

func (R *GSRuntime) UpdateUserData(username string, data uint64) {
	R.usersMu.Lock()
	defer R.usersMu.Unlock()
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User == username {
			R.AuthUsers[i].DataConsumed += data
		}
	}
}

func (R *GSRuntime) GSUserGetDataJSON() []byte {
	R.usersMu.RLock()
	defer R.usersMu.RUnlock()
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

func (R *GSRuntime) LoadUsers() error {
	log.Println("Load users")
	usersString, err := R.GSSettings.GetE("authusers")
	if err != nil {
		return fmt.Errorf("read users: %w", err)
	}

	users := []GatesentryTypes.GSUser{}
	if err := json.Unmarshal([]byte(usersString), &users); err != nil {
		return fmt.Errorf("parse users: %w", err)
	}

	R.usersMu.Lock()
	R.AuthUsers = users
	R.usersMu.Unlock()
	return nil
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
	if err := R.GSSettings.Update("authusers", string(b)); err != nil {
		log.Printf("Unable to add user: %v", err)
		return false
	}
	if err := R.LoadUsers(); err != nil {
		log.Printf("Unable to reload users: %v", err)
	}
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
