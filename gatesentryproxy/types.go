package gatesentryproxy

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
	ProxyErrorHandler  func(string)
	DoMitm             func(host string) bool
	IsExceptionUrl     func(url string) bool
	IsAuthEnabled      func() bool
	LogHandler         func(GSLogData)
	Handlers           map[string][]*GSHandler
	UsersCache         map[string]GSUserCached
}

// For the refactored filter input
type GSContentFilterData struct {
	Url                  string
	ContentType          string
	Content              []byte
	FilterResponse       []byte
	FilterResponseAction ProxyAction
}

type GSContentTypeFilterData struct {
	Url                  string
	ContentType          string
	FilterResponseAction ProxyAction
}

type GSContentSizeFilterData struct {
	Url         string
	ContentType string
	ContentSize int64
}

type GSUserAccessFilterData struct {
	User                 string
	FilterResponseAction string
}

type GSTimeAccessFilterData struct {
	Url                  string
	ContentType          string
	User                 string
	FilterResponseAction string
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
}
