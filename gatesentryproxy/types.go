package gatesentryproxy

import GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"

type GSProxyPassthru struct {
	UserData         interface{}
	DontTouch        bool
	User             string
	ProxyActionToLog GatesentryTypes.ProxyAction
}

type GSResponder struct {
	Changed bool
	Data    []byte
}

type GSHandler struct {
	Id     string
	Handle func(*GSContentFilterData)
}

type GSUserCached struct {
	User string
	Pass string
}

type GSProxy struct {
	AuthHandler        func(authheader string) bool
	ContentHandler     func(*GSContentFilterData)
	ContentTypeHandler func(*GSContentTypeFilterData)
	ContentSizeHandler func(GSContentSizeFilterData)
	UserAccessHandler  func(*GSUserAccessFilterData)
	TimeAccessHandler  func(*GSTimeAccessFilterData)
	UrlAccessHandler   func(*GSUrlFilterData)
	ProxyErrorHandler  func(*GSProxyErrorData)
	DoMitm             func(host string) bool
	IsExceptionUrl     func(url string) bool
	IsAuthEnabled      func() bool
	LogHandler         func(GSLogData)
	Handlers           map[string][]*GSHandler
	UsersCache         map[string]GSUserCached
	RuleHandler        func(*GatesentryTypes.GSRuleFilterParam)
}

// For the refactored filter input
type GSContentFilterData struct {
	Url                  string
	ContentType          string
	Content              []byte
	FilterResponse       []byte
	FilterResponseAction GatesentryTypes.ProxyAction
}

type GSContentTypeFilterData struct {
	Url                  string
	ContentType          string
	FilterResponseAction GatesentryTypes.ProxyAction
	FilterResponse       []byte
}

type GSContentSizeFilterData struct {
	Url         string
	ContentType string
	ContentSize int64
	User        string
}

type GSUserAccessFilterData struct {
	User                 string
	FilterResponseAction GatesentryTypes.ProxyAction
	FilterResponse       []byte
}

type GSTimeAccessFilterData struct {
	Url                  string
	ContentType          string
	User                 string
	FilterResponseAction string
	FilterResponse       []byte
}

type GSLogData struct {
	Url         string
	ContentType string
	User        string
	Action      GatesentryTypes.ProxyAction
}

type GSUrlFilterData struct {
	Url                  string
	User                 string
	FilterResponseAction GatesentryTypes.ProxyAction
	FilterResponse       []byte
}

type GSProxyErrorData struct {
	Error          string
	FilterResponse []byte
}
