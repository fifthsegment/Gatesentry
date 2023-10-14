package GatesentryTypes

type GSDataUpdater struct {
	Email string
	Id    string
}

type GSConsumptionUpdater struct {
	Id             string
	Consumption    int64
	Message        string
	AdditionalInfo string
	Time           string
}

type GSKeepAliver struct {
	Id      string
	Version float32
}

type GSKeepAliveResponse struct {
	Ok      bool
	Error   bool
	Message string
}

type GSConsumptionUpdaterResponse struct {
	Ok             bool
	Error          bool
	Message        string
	AdditionalInfo string
}

type GSWebServerCommunicator struct {
	Action string
}

type GSUser struct {
	User         string `json:"username"`
	Pass         string `json:"password"`
	Base64String string
	DataConsumed uint64 `json:"dataconsumed"`
	AllowAccess  bool   `json:"allowaccess"`
	ToDelete     bool   `json:"todelete"`
}

type GSUserPublic struct {
	User         string `json:"user"`
	DataConsumed uint64 `json:"dataconsumed"`
	AllowAccess  bool   `json:"allowaccess"`
	Password     string `json:"password"`
}
