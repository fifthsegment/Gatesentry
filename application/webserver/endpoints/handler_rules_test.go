package gatesentryWebserverEndpoints

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

type failingRuleManager struct{}

func (failingRuleManager) GetRules() ([]GatesentryTypes.Rule, error) {
	return nil, errors.New("persistence failed")
}
func (failingRuleManager) GetRule(string) (*GatesentryTypes.Rule, error) {
	return nil, errors.New("persistence failed")
}
func (failingRuleManager) AddRule(rule GatesentryTypes.Rule) (GatesentryTypes.Rule, error) {
	return rule, errors.New("persistence failed")
}
func (failingRuleManager) UpdateRule(string, GatesentryTypes.Rule) error {
	return errors.New("persistence failed")
}
func (failingRuleManager) DeleteRule(string) error { return errors.New("persistence failed") }
func (failingRuleManager) MatchRule(string, string) GatesentryTypes.RuleMatch {
	return GatesentryTypes.RuleMatch{}
}

func TestRuleCreateReturnsPersistenceFailure(t *testing.T) {
	old := ruleManager
	InitRuleManager(failingRuleManager{})
	t.Cleanup(func() { ruleManager = old })
	req := httptest.NewRequest(http.MethodPost, "/api/rules", strings.NewReader(`{"domain":"example.com"}`))
	w := httptest.NewRecorder()
	GSApiRuleCreate(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(w.Body.String(), "persistence failed") {
		t.Fatalf("body = %q", w.Body.String())
	}
}
