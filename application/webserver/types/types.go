package gatesentryWebserverTypes

import (
	"time"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

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
	AdminPassword string `json:"admin_password,omitempty"`
	AdminUser     string `json:"admin_username,omitempty"`
}

type Datareceiver struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TemporaryRuntime struct {
	GetUserGetJSON          func() []byte
	AuthUsers               []GatesentryTypes.GSUser
	RemoveUser              func(GatesentryTypes.GSUser)
	UpdateUser              func(string, GatesentryTypes.GSUserPublic)
	GetInstallationId       func() string
	GetTotalConsumptionData func() (string, string)
	GetApplicationVersion   func() string
	GetProxyTraffic         func() (uploadBytes, downloadBytes uint64, startedAt time.Time)
	Logger                  *gatesentryLogger.Log
	Reload                  func()
}

type InputArgs struct {
	GetUserGetJSON          func() []byte
	AuthUsers               []GatesentryTypes.GSUser
	RemoveUser              func(GatesentryTypes.GSUser)
	UpdateUser              func(string, GatesentryTypes.GSUserPublic)
	GetInstallationId       func() string
	GetTotalConsumptionData func() (string, string)
	GetApplicationVersion   func() string
	GetProxyTraffic         func() (uploadBytes, downloadBytes uint64, startedAt time.Time)
	Reload                  func()
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
		GetProxyTraffic:         args.GetProxyTraffic,
		Reload:                  args.Reload,
	}
}
