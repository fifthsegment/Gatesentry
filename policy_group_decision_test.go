package main

import (
	"os"
	"testing"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

type decisionTestResolver struct{}

func (decisionTestResolver) ResolveDeviceByIP(ip string) (string, bool, bool) {
	if ip == "192.0.2.10" {
		return "device-1", false, false
	}
	return "", false, false
}

func newDecisionTestService(t *testing.T) *gatesentryPolicy.Service {
	t.Helper()
	dir := t.TempDir()
	oldBase := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(oldBase) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := gatesentryPolicy.NewService(store, decisionTestResolver{})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestPolicyGroupDecisionBlock(t *testing.T) {
	svc := newDecisionTestService(t)
	original := gatesentryDnsServer.GetPolicyService()
	gatesentryDnsServer.SetPolicyServiceForTests(svc)
	t.Cleanup(func() { gatesentryDnsServer.SetPolicyServiceForTests(original) })

	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	match, handled := policyGroupDecision("192.0.2.10", "", "chess.games.example")
	if !handled || !match.Matched || !match.ShouldBlock {
		t.Fatalf("match = %+v, handled = %v, want handled block", match, handled)
	}
}

func TestPolicyGroupDecisionFallsThroughToUserRules(t *testing.T) {
	svc := newDecisionTestService(t)
	original := gatesentryDnsServer.GetPolicyService()
	gatesentryDnsServer.SetPolicyServiceForTests(svc)
	t.Cleanup(func() { gatesentryDnsServer.SetPolicyServiceForTests(original) })

	// A group exists but neither its users nor its devices match this
	// request: the decision must fall through so the existing user-scoped
	// rule engine keeps its behavior unchanged.
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "other", Action: gatesentryPolicy.ActionBlock, Domains: []string{"other.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	match, handled := policyGroupDecision("192.0.2.10", "dana", "legacy-rule.example")
	if handled {
		t.Fatalf("handled = true, want fall-through to the rule engine")
	}
	if match.Matched {
		t.Fatalf("match = %+v, want untouched rule-engine result", match)
	}
}

func TestPolicyGroupDecisionIPAddressUserFallbackUsesDeviceIdentity(t *testing.T) {
	svc := newDecisionTestService(t)
	original := gatesentryDnsServer.GetPolicyService()
	gatesentryDnsServer.SetPolicyServiceForTests(svc)
	t.Cleanup(func() { gatesentryDnsServer.SetPolicyServiceForTests(original) })

	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{
		{ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"games.example"}},
		{ID: "user-group", Action: gatesentryPolicy.ActionBlock, Domains: []string{"games.example"}, Users: []string{"192.0.2.10"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	// The unauthenticated proxy passes the client IP as the user fallback.
	// It must resolve to the device identity, never the user group keyed by
	// that IP string.
	match, handled := policyGroupDecision("192.0.2.10", "192.0.2.10", "games.example")
	if !handled || !match.Matched || !match.ShouldBlock {
		t.Fatalf("match = %+v, handled = %v, want device identity block", match, handled)
	}
}
