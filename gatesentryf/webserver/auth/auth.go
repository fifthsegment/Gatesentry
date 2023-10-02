package gatesentryWebserverAuth

import (
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/kataras/iris/v12"
)

var (
	GSSigningKey = "USSEHERE"
)

func VerifyAdminUser(username string, password string, settingsStore *gatesentryWebserverTypes.SettingsStore) bool {
	validusername := settingsStore.WebGet("username")
	validpassword := settingsStore.WebGet("pass")
	if validusername == username && validpassword == password {
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
