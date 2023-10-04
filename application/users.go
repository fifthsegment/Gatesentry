package gatesentryf

import (
	"encoding/base64"
	"encoding/json"

	// "fmt"
	"log"
	"strings"
	"time"

	structures "bitbucket.org/abdullah_irfan/gatesentryf/structures"
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
			// log.Println("Updating user data for user = " +username)
			R.AuthUsers[i].DataConsumed += data
			// fmt.Println(R.AuthUsers[i])
		}
	}
}

func (R *GSRuntime) GSUserGetDataJSON() []byte {
	temp := []structures.GSUserPublic{}
	for i := 0; i < len(R.AuthUsers); i++ {
		tuser := structures.GSUserPublic{User: R.AuthUsers[i].User, DataConsumed: R.AuthUsers[i].DataConsumed, AllowAccess: R.AuthUsers[i].AllowAccess}
		temp = append(temp, tuser)
	}
	b, err := json.Marshal(temp)
	if err != nil {
		return nil
	}
	return b
}

func (R *GSRuntime) LoadUsers() {
	log.Println("Loading users")
	usersString := R.GSSettings.Get("authusers")
	// fmt.Println( usersString );
	users := []structures.GSUser{}
	json.Unmarshal([]byte(usersString), &users)
	// fmt.Println( users )
	R.AuthUsers = users
	for i := 0; i < len(R.AuthUsers); i++ {
		user := R.AuthUsers[i]
		auth := user.User + ":" + user.Pass
		R.AuthUsers[i].Base64String = base64.StdEncoding.EncodeToString([]byte(auth))
		// log.Println("Setting Base64String = "+R.AuthUsers[i].Base64String)
	}
}

func (R *GSRuntime) RemoveUser(data structures.GSUser) {
	// R.LoadUsers();
	// found := false;
	log.Println("Removing username = " + data.User)
	newusers := []structures.GSUser{}
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User != data.User {
			newusers = append(newusers, R.AuthUsers[i])
		}
	}
	R.AuthUsers = newusers
	// if ( !found ){
	// 	R.AddUser(data.User, data.Password);
	// }
	R.GSUserDataSaver()
}

func (R *GSRuntime) UpdatePassword(username string, password string) {
	log.Println("Updating Password")
	for i := 0; i < len(R.AuthUsers); i++ {
		if R.AuthUsers[i].User == username {
			R.AuthUsers[i].Pass = password
		}
	}
	R.GSUserDataSaver()
	R.init()
}

func (R *GSRuntime) UpdateUser(username string, data structures.GSUserPublic) {
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
	Guser := structures.GSUser{User: user, Pass: pass}
	tempusers := append(R.AuthUsers, Guser)

	b, err := json.Marshal(tempusers)
	if err != nil {
		return false
	}
	log.Println(string(b))
	R.GSSettings.Update("authusers", string(b))
	R.LoadUsers()
	return true
	// R.GSSettings.Update("authusers", string(b))
}

func (R *GSRuntime) IsUserValid(base64string string) bool {
	authheader := strings.SplitN(base64string, " ", 2)
	if len(authheader) != 2 || authheader[0] != "Basic" {
		return false
	}
	base64string = authheader[1]
	for i := 0; i < len(R.AuthUsers); i++ {
		user := R.AuthUsers[i]
		log.Printf(user.Base64String + " == " + base64string)
		if user.Base64String == base64string {
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
