package GatesentryTypes

// format : "[{\"timeRestriction\":{\"from\":\"02\",\"to\":\"03\"},\"action\":\"block\",\"contentType\":\"\",\"contentSize\":2,\"user\":\"abdullah\",\"domain\":\"ask.com\"}]"

type GSRule struct {
	TimeRestriction struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"timeRestriction"`
	Action      string `json:"action"`
	ContentType string `json:"contentType"`
	ContentSize int    `json:"contentSize"` // in MB
	User        string `json:"user"`
	Domain      string `json:"domain"`
}

type GSRuleFilterParam struct {
	Url         string
	ContentType string
	User        string
	Action      ProxyAction
}

type ProxyAction string
