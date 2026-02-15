package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"

	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	"github.com/gorilla/mux"
)

// RuleManagerInterface defines the interface for rule management
type RuleManagerInterface interface {
	GetRules() ([]GatesentryTypes.Rule, error)
	GetRule(ruleID string) (*GatesentryTypes.Rule, error)
	AddRule(rule GatesentryTypes.Rule) (GatesentryTypes.Rule, error)
	UpdateRule(ruleID string, updatedRule GatesentryTypes.Rule) error
	DeleteRule(ruleID string) error
	MatchRule(domain, user string) GatesentryTypes.RuleMatch
}

var ruleManager RuleManagerInterface

// InitRuleManager initializes the rule manager with an implementation
func InitRuleManager(rm RuleManagerInterface) {
	ruleManager = rm
}

func GSApiRulesGetAll(w http.ResponseWriter, r *http.Request) {
	if ruleManager == nil {
		http.Error(w, "Rule manager not initialized", http.StatusInternalServerError)
		return
	}

	rules, err := ruleManager.GetRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rules": rules,
	})
}

// GSApiRuleGet returns a single rule by ID
func GSApiRuleGet(w http.ResponseWriter, r *http.Request) {
	if ruleManager == nil {
		http.Error(w, "Rule manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	ruleID := vars["id"]

	rule, err := ruleManager.GetRule(ruleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if rule == nil {
		http.Error(w, "Rule not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rule)
}

// GSApiRuleCreate creates a new rule
func GSApiRuleCreate(w http.ResponseWriter, r *http.Request) {
	if ruleManager == nil {
		http.Error(w, "Rule manager not initialized", http.StatusInternalServerError)
		return
	}

	var rule GatesentryTypes.Rule
	err := json.NewDecoder(r.Body).Decode(&rule)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	createdRule, err := ruleManager.AddRule(rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Rule created successfully",
		"rule":    createdRule,
	})
}

// GSApiRuleUpdate updates an existing rule
func GSApiRuleUpdate(w http.ResponseWriter, r *http.Request) {
	if ruleManager == nil {
		http.Error(w, "Rule manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	ruleID := vars["id"]

	var rule GatesentryTypes.Rule
	err := json.NewDecoder(r.Body).Decode(&rule)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	err = ruleManager.UpdateRule(ruleID, rule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Rule updated successfully",
		"rule":    rule,
	})
}

// GSApiRuleDelete deletes a rule
func GSApiRuleDelete(w http.ResponseWriter, r *http.Request) {
	if ruleManager == nil {
		http.Error(w, "Rule manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	ruleID := vars["id"]

	err := ruleManager.DeleteRule(ruleID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Rule deleted successfully",
	})
}

// GSApiRuleTest tests a rule match against a domain
func GSApiRuleTest(w http.ResponseWriter, r *http.Request) {
	if ruleManager == nil {
		http.Error(w, "Rule manager not initialized", http.StatusInternalServerError)
		return
	}

	var testRequest struct {
		Domain string `json:"domain"`
		User   string `json:"user"`
	}

	err := json.NewDecoder(r.Body).Decode(&testRequest)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	match := ruleManager.MatchRule(testRequest.Domain, testRequest.User)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(match)
}

func RegisterRuleEndpoints(router *mux.Router, rm RuleManagerInterface) {
	InitRuleManager(rm)

	router.HandleFunc("/api/rules", GSApiRulesGetAll).Methods("GET")
	router.HandleFunc("/api/rules", GSApiRuleCreate).Methods("POST")
	router.HandleFunc("/api/rules/{id}", GSApiRuleGet).Methods("GET")
	router.HandleFunc("/api/rules/{id}", GSApiRuleUpdate).Methods("PUT")
	router.HandleFunc("/api/rules/{id}", GSApiRuleDelete).Methods("DELETE")
	router.HandleFunc("/api/rules/test", GSApiRuleTest).Methods("POST")
}

// GetRuleManager returns the global rule manager instance
func GetRuleManager() RuleManagerInterface {
	return ruleManager
}
