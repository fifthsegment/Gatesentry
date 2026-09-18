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
	ProxyErrorHandler  func(*GSProxyErrorData)
	DoMitm             func(host string) bool
	IsExceptionUrl     func(url string) bool
	IsAuthEnabled      func() bool
	LogHandler         func(GSLogData)
	// RuleMatchHandler returns the RuleMatch-compatible struct for a request.
	// ClientIP is the observed source address, used only as a device lookup
	// key; it is never itself treated as the authenticated identity.
	RuleMatchHandler func(domain string, user string, clientIP string) interface{}
	Handlers         map[string][]*GSHandler
	UsersCache       map[string]GSUserCached
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
	// ClientIP is the observed source address, used as a device lookup
	// key by the LogHandler closure that translates GSLogData into a
	// policy.Decision. It is never itself treated as the authenticated
	// identity.
	ClientIP string
	// Layer identifies which proxy path produced this log entry
	// ("explicit_proxy", "transparent_proxy", or "content"). The closure
	// maps it to a policy.DecisionLayer.
	Layer string
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
