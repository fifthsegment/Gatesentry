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
	Handle func(*[]byte, *GSResponder, *GSProxyPassthru)
}

type GSUserCached struct {
	User string
	Pass string
}

type GSProxy struct {
	Handlers   map[string][]*GSHandler
	UsersCache map[string]GSUserCached
}
