package gatesentryproxy

import "sync"

type GSProxyPassthru struct {
	UserData         interface{}
	DontTouch        bool
	User             string
	ProxyActionToLog ProxyAction
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
	User     string
	Pass     string
	CachedAt int64
}

type GSProxy struct {
	AuthHandler          func(authheader string) bool
	ContentHandler       func(*GSContentFilterData)
	ContentSizeHandler   func(GSContentSizeFilterData)
	UserAccessHandler    func(*GSUserAccessFilterData)
	TimeAccessHandler    func(*GSTimeAccessFilterData)
	UrlAccessHandler     func(*GSUrlFilterData)
	ProxyErrorHandler    func(*GSProxyErrorData)
	DoMitm               func(host string) bool
	IsExceptionUrl       func(url string) bool
	IsAuthEnabled        func() bool
	LogHandler           func(GSLogData)
	RuleMatchHandler     func(domain string, user string) interface{} // Returns RuleMatch
	RuleBlockPageHandler func(domain string, ruleName string) []byte  // Build HTML block page for rule-based domain blocks
	Handlers             map[string][]*GSHandler
	UsersCache           sync.Map
}

// For the refactored filter input
type GSContentFilterData struct {
	Url                  string
	ContentType          string
	Content              []byte
	FilterResponse       []byte
	FilterResponseAction ProxyAction
}

type GSContentSizeFilterData struct {
	Url         string
	ContentType string
	ContentSize int64
	User        string
}

type GSUserAccessFilterData struct {
	User                 string
	FilterResponseAction ProxyAction
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
	Action      ProxyAction
}

type GSUrlFilterData struct {
	Url                  string
	User                 string
	FilterResponseAction ProxyAction
	FilterResponse       []byte
}

type GSProxyErrorData struct {
	Error          string
	FilterResponse []byte
}
