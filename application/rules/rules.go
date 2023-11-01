package gatesentryrules

import "encoding/json"

type TimeRestriction struct {
	Action string `json:"action"`
	To     string `json:"to"`
	From   string `json:"from"`
}

type Rule struct {
	TimeRestriction TimeRestriction `json:"timeRestriction"`
	ContentType     string          `json:"contentType"`
	ContentSize     int64           `json:"contentSize"`
	User            string          `json:"user"`
	Domain          string          `json:"domain"`
}

type Rules []Rule

func ParseRules(jsonStr string) (rules []Rule, err error) {
	err = json.Unmarshal([]byte(jsonStr), &rules)
	return
}
