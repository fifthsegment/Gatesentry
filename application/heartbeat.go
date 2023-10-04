package gatesentryf

import (
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"time"

	gscommonweb "bitbucket.org/abdullah_irfan/gatesentryf/commonweb"
	gstransport "bitbucket.org/abdullah_irfan/gatesentryf/securetransport"
)

var NONALIVES int

func UpdateSentryAliveStatus(R *GSRuntime, alive bool, message string) {
	if alive {
		if NONALIVES != 0 {
			NONALIVES = 0
			R.GSSettings.Update("NonAlives", "0")
			R.OnHeartbeat()
		}
	}
	if !alive {
		NONALIVES++
		num := strconv.Itoa(NONALIVES)
		R.GSSettings.Update("NonAlives", num)
	}
	CheckActionSentryNotAlive(R, message)
}

func KeepSentryAlive() (bool, error) {
	ka := gscommonweb.GSKeepAliver{Id: INSTALLATIONID, Version: GSVer}
	kaj, err := json.Marshal(ka)
	if err != nil {
		return false, err
	}
	log.Println("Sending a keep alive request")
	resp, err := gstransport.SendEncryptedData("/keepalive", kaj, INSTALLATIONID)
	if err != nil {
		log.Println("Error in sending Keep alive request")
		log.Println(err)
		return false, err
	}

	var kar gscommonweb.GSKeepAliveResponse
	json.Unmarshal([]byte(resp), &kar)
	// fmt.Println( kar )

	if kar.Ok {
		log.Println("Keep alive response = Ok")
		return true, nil
	} else {
		log.Println("Keep alive response = Not Ok")
	}
	log.Println("Error Unmarshalling Keep Alive response")
	return false, errors.New(kar.Message)
}

func (R *GSRuntime) KeepAliveMonitor() {
	go func() {
		if R.GSKeepSentryAliveRunning {
			log.Println("Keep alive monitor already running")

			return
		} else {
			gstransport.SetbaseEndpoint(GSAPIBASEPOINT)
			num, err := strconv.Atoi(R.GSSettings.Get("NonAlives"))
			log.Println("Previous Non Keep Alive Count = " + R.GSSettings.Get("NonAlives"))
			if err != nil {
				NONALIVES = 0
			} else {
				NONALIVES = num
			}
			log.Println("Starting keep alive monitor")
			status, err := KeepSentryAlive()
			message := ""
			if err != nil {
				message = err.Error()
			}
			if status == true {
				NONALIVES = 0
				R.OnHeartbeat()
			}
			CheckActionSentryNotAlive(R, message)
			SentryNotAlive(status, message)
			R.GSKeepSentryAliveRunning = true
		}
		t := time.NewTicker(time.Second * GSKEEPALIVETIMEOUT)
		for {
			tt, err := KeepSentryAlive()
			if err != nil {
				log.Println(err.Error())
				UpdateSentryAliveStatus(R, tt, err.Error())
			} else {
				UpdateSentryAliveStatus(R, tt, "")
			}

			if tt {
				// fmt.Println("Sentry is currently alive")
			} else {
				// fmt.Println("Sentry is currently NOT alive")
			}
			<-t.C
		}
	}()
}

func CheckActionSentryNotAlive(R *GSRuntime, message string) {
	if NONALIVES > NONALIVESBEFOREKILL {
		// ActionSentryNotAlive()
		SentryNotAlive(false, message)
	}
}

func SentryNotAlive(status bool, message string) {
	if !status {
		// panic(errors.New("Sentry isnt alive"))
		R.OnNoHeartbeat(message)
	}
}
