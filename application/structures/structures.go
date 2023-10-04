package gatesentry2structures

type GSWebServerCommunicator struct {
	Action string
}

type GSUser struct {
	User         string `json:user`
	Pass         string `json:pass`
	Base64String string
	DataConsumed uint64 `json:dataconsumed`
	AllowAccess  bool   `json:allowaccess`
	ToDelete     bool   `json:todelete`
}

type GSUserPublic struct {
	User         string `json:user`
	DataConsumed uint64 `json:dataconsumed`
	AllowAccess  bool   `json:allowaccess`
	Password     string `json:password`
}
