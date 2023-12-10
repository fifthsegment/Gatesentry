package gatesentryf

import (
	"encoding/json"
	"log"

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

func (R *GSRuntime) RunRuleHandler(rule *GatesentryTypes.GSRuleFilterParam) {
	log.Println("Running rule handler")

}
