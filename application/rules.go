package gatesentryf

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

func (R *GSRuntime) LoadRules() {
	rules := R.GSSettings.Get("rules")
	log.Println("Loading rules")
	rules_parsed := []GatesentryTypes.GSRule{}
	err := json.Unmarshal([]byte(rules), &rules_parsed)

	if err != nil {
		log.Println("Error parsing rules ", err)
		return
	}

	R.RuleList = rules_parsed

	log.Println("Number of rules loaded : ", len(R.RuleList))
}

func (R *GSRuntime) RunRuleHandler(testCase *GatesentryTypes.GSRuleFilterParam) {
	// get current system time hour
	currentTime := time.Now()
	currentHour := currentTime.Hour()
	log.Println("Running rule handler for request type =  " + strconv.FormatBool(testCase.IsDnsRequest) + " for url = " + testCase.Url + " time now = " + strconv.Itoa(currentHour) + " user = " + testCase.User + " content type = " + testCase.ContentType + " size = " + strconv.Itoa(testCase.ContentSize))
	for _, rule := range R.RuleList {
		log.Println("Rule domain = " + rule.Domain + " == " + testCase.Url)
		if rule.Domain == testCase.Url {
			testCase.Action = GatesentryTypes.ProxyActionBlocked
		}
	}

}
