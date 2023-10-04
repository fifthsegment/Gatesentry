package gatesentryWebserverEndpoints

import (
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

var (
	settingsStore *gatesentryWebserverTypes.SettingsStore
)

func Init(settings *gatesentryWebserverTypes.SettingsStore) {
	settingsStore = settings
}
