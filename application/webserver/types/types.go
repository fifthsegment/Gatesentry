package gatesentryWebserverTypes

import (
	"encoding/json"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

// create an initializer for above struct

// func (s *SettingsStore) GetAdminPassword() string {
// 	general_settings := s.Get("general_settings")
// 	general_settings_parsed := GSGeneral_Settings{}
// 	json.Unmarshal([]byte(general_settings), &general_settings_parsed)
// 	return general_settings_parsed.AdminPassword
// }

// func (s *SettingsStore) GetAdminUser() string {
// 	general_settings := s.Get("general_settings")
// 	general_settings_parsed := GSGeneral_Settings{}
// 	json.Unmarshal([]byte(general_settings), &general_settings_parsed)
// 	return general_settings_parsed.AdminUser
// }

func GetAdminUser(s *gatesentry2storage.MapStore) string {
	general_settings := s.Get("general_settings")
	general_settings_parsed := GSGeneral_Settings{}
	json.Unmarshal([]byte(general_settings), &general_settings_parsed)
	return general_settings_parsed.AdminUser
}

func GetAdminPassword(s *gatesentry2storage.MapStore) string {
	general_settings := s.Get("general_settings")
	general_settings_parsed := GSGeneral_Settings{}
	json.Unmarshal([]byte(general_settings), &general_settings_parsed)
	return general_settings_parsed.AdminPassword
}

type User struct {
	Name string `json:"name"`
	Mail string `json:"mail"`
	Pass string `json:"pass"`
}
type Login struct {
	Username string `json:"username"`
	Pass     string `json:"pass"`
}

type GSGeneral_Settings struct {
	LogLocation   string `json:"log_location"`
	AdminPassword string `json:"admin_password"`
	AdminUser     string `json:"admin_username"`
}

type Datareceiver struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TemporaryRuntime struct {
	GetUserGetJSON          func() []byte
	GetAuthUsers            func() []GatesentryTypes.GSUser
	AuthUsers               []GatesentryTypes.GSUser
	RemoveUser              func(GatesentryTypes.GSUser)
	UpdateUser              func(string, GatesentryTypes.GSUserPublic)
	GetInstallationId       func() string
	GetTotalConsumptionData func() (string, string)
	GetApplicationVersion   func() string
	Logger                  *gatesentryLogger.Log
	Reload                  func()
}

type InputArgs struct {
	GetUserGetJSON          func() []byte
	GetAuthUsers            func() []GatesentryTypes.GSUser
	AuthUsers               []GatesentryTypes.GSUser
	RemoveUser              func(GatesentryTypes.GSUser)
	UpdateUser              func(string, GatesentryTypes.GSUserPublic)
	GetInstallationId       func() string
	GetTotalConsumptionData func() (string, string)
	GetApplicationVersion   func() string
	Reload                  func()
}

func NewTemporaryRuntime(args InputArgs) *TemporaryRuntime {
	return &TemporaryRuntime{
		GetUserGetJSON:          args.GetUserGetJSON,
		GetAuthUsers:            args.GetAuthUsers,
		AuthUsers:               args.AuthUsers,
		RemoveUser:              args.RemoveUser,
		UpdateUser:              args.UpdateUser,
		GetInstallationId:       args.GetInstallationId,
		GetTotalConsumptionData: args.GetTotalConsumptionData,
		GetApplicationVersion:   args.GetApplicationVersion,
		Reload:                  args.Reload,
	}
}
