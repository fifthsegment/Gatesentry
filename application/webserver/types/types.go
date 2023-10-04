package gatesentryWebserverTypes

import (
	"encoding/json"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	structures "bitbucket.org/abdullah_irfan/gatesentryf/structures"
)

type GetSettings func(string) string

type SetSettings func(string, string)

type SettingsStore struct {
	GetSettings           GetSettings
	SetSettings           SetSettings
	WebGetSettings        GetSettings
	WebSetSettings        SetSettings
	WebSetDefaultSettings SetSettings
	InitGatesentry        func()
}

// create an initializer for above struct
func NewSettingsStore(getSettings GetSettings, setSettings SetSettings, webGetSettings GetSettings, webSetSettings SetSettings, initGatesentry func()) *SettingsStore {
	return &SettingsStore{
		GetSettings:    getSettings,
		SetSettings:    setSettings,
		WebGetSettings: webGetSettings,
		WebSetSettings: webSetSettings,
		InitGatesentry: initGatesentry,
	}
}

func (s *SettingsStore) Get(key string) string {
	return s.GetSettings(key)
}

func (s *SettingsStore) Set(key string, value string) {
	s.SetSettings(key, value)
}

func (s *SettingsStore) WebSetDefault(key string, value string) {
	s.SetSettings(key, value)
}

func (s *SettingsStore) WebGet(key string) string {
	return s.WebGetSettings(key)
}

func (s *SettingsStore) WebSet(key string, value string) {
	s.WebSetSettings(key, value)
}

func (s *SettingsStore) GetAdminPassword() string {
	general_settings := s.Get("general_settings")
	general_settings_parsed := GSGeneral_Settings{}
	json.Unmarshal([]byte(general_settings), &general_settings_parsed)
	return general_settings_parsed.AdminPassword
}

func (s *SettingsStore) GetAdminUser() string {
	general_settings := s.Get("general_settings")
	general_settings_parsed := GSGeneral_Settings{}
	json.Unmarshal([]byte(general_settings), &general_settings_parsed)
	return general_settings_parsed.AdminUser
}

func (s *SettingsStore) OnMajorSettingsChange() {
	s.InitGatesentry()
}

type User struct {
	Name string `json: "name"`
	Mail string `json: "mail"`
	Pass string `json: "pass"`
}
type Login struct {
	Username string `json: "username"`
	Pass     string `json: "pass"`
}

type GSGeneral_Settings struct {
	LogLocation   string `json:"log_location"`
	AdminPassword string `json:"admin_password"`
	AdminUser     string `json:"admin_username"`
}

type Datareceiver struct {
	Key   string `json:key`
	Value string `json:value`
}

type TemporaryRuntime struct {
	GetUserGetJSON          func() []byte
	AuthUsers               []structures.GSUser
	RemoveUser              func(structures.GSUser)
	UpdateUser              func(string, structures.GSUserPublic)
	GetInstallationId       func() string
	GetTotalConsumptionData func() (string, string)
	GetApplicationVersion   func() string
	Logger                  *gatesentryLogger.Log
}

type InputArgs struct {
	GetUserGetJSON          func() []byte
	AuthUsers               []structures.GSUser
	RemoveUser              func(structures.GSUser)
	UpdateUser              func(string, structures.GSUserPublic)
	GetInstallationId       func() string
	GetTotalConsumptionData func() (string, string)
	GetApplicationVersion   func() string
}

func NewTemporaryRuntime(args InputArgs) *TemporaryRuntime {
	return &TemporaryRuntime{
		GetUserGetJSON:          args.GetUserGetJSON,
		AuthUsers:               args.AuthUsers,
		RemoveUser:              args.RemoveUser,
		UpdateUser:              args.UpdateUser,
		GetInstallationId:       args.GetInstallationId,
		GetTotalConsumptionData: args.GetTotalConsumptionData,
		GetApplicationVersion:   args.GetApplicationVersion,
	}
}
