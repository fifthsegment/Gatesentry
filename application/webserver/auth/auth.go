package gatesentryWebserverAuth

import (
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/kataras/iris/v12"
)

var (
	GSSigningKey = "USSEHERE"
)

func VerifyAdminUser(username string, password string, settingsStore *gatesentryWebserverTypes.SettingsStore) bool {
	if settingsStore.GetAdminUser() == username && settingsStore.GetAdminPassword() == password {
		return true
	}
	return false
}

func VerifyToken(ctx iris.Context) {
	ctx.JSON(struct {
		Validated bool
		Jwtoken   string
		Message   string
	}{Validated: true, Jwtoken: "", Message: ""})
}
