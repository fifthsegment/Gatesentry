package gatesentryf

import (
	"strconv"
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

func (R *GSRuntime) KeepAliveMonitor() {

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
