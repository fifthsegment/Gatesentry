package main

import (
	"os"
	"strings"
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

// saveDecisionGroups installs the policy service and persists the supplied
// groups, so a test states only the policy it is about.
func saveDecisionGroups(t *testing.T, svc *gatesentryPolicy.Service, groups ...gatesentryPolicy.PolicyGroup) {
	t.Helper()
	original := gatesentryDnsServer.GetPolicyService()
	gatesentryDnsServer.SetPolicyServiceForTests(svc)
	t.Cleanup(func() { gatesentryDnsServer.SetPolicyServiceForTests(original) })
	if err := svc.SaveGroups(groups); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: "device-1", GroupID: groups[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyProxyDecisionBlocksAssignedDevice(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"games.example"},
	})

	decision := policyProxyDecision("192.0.2.10", "", "chess.games.example")
	if decision == nil || !decision.Block {
		t.Fatalf("decision = %+v, want a block", decision)
	}
	if !strings.Contains(decision.Reason, "Kids") || !strings.Contains(decision.Reason, "games.example") {
		t.Fatalf("reason = %q, want the policy and the matched domain named", decision.Reason)
	}
}

// A request no policy covers returns nil so the proxy keeps the gateway's own
// settings; a policy never changes inspection for traffic it does not own.
func TestPolicyProxyDecisionWithoutAMatchIsUnhandled(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "other", Name: "Other", BlockedDomains: []string{"other.example"},
	})

	if decision := policyProxyDecision("192.0.2.10", "", "uncovered.example"); decision != nil {
		t.Fatalf("decision = %+v, want nil for an uncovered domain", decision)
	}
}

// An allow rule ahead of the blocked list is an exception: the proxy forwards
// the request and also skips the gateway-wide URL blocklist.
func TestPolicyProxyDecisionAllowRuleIsAnException(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"games.example"},
		Rules: []gatesentryPolicy.GroupRule{{
			ID: "allow-chess", Enabled: true, Action: gatesentryPolicy.ActionAllow,
			Target: gatesentryPolicy.RuleTarget{Domains: []string{"chess.games.example"}},
		}},
	})

	decision := policyProxyDecision("192.0.2.10", "", "chess.games.example")
	if decision == nil || decision.Block || !decision.Allow {
		t.Fatalf("decision = %+v, want an allow", decision)
	}
	if blocked := policyProxyDecision("192.0.2.10", "", "poker.games.example"); blocked == nil || !blocked.Block {
		t.Fatalf("decision = %+v, want the rest of the blocked domain blocked", blocked)
	}
}

// The unauthenticated proxy passes the client address as the user fallback.
// It must resolve to the device identity.
func TestPolicyProxyDecisionIPAddressUserFallbackUsesDeviceIdentity(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc,
		gatesentryPolicy.PolicyGroup{ID: "kids", Name: "Kids", BlockedDomains: []string{"games.example"}},
	)

	decision := policyProxyDecision("192.0.2.10", "192.0.2.10", "games.example")
	if decision == nil || !decision.Block || !strings.Contains(decision.Reason, "Kids") {
		t.Fatalf("decision = %+v, want the device's policy", decision)
	}
}

// A rule that carries URL patterns narrows its block to a path: the proxy
// receives the patterns and an inspection requirement instead of a whole-host
// denial.
func TestPolicyProxyDecisionRuleCarriesURLPatterns(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "kids", Name: "Kids",
		Rules: []gatesentryPolicy.GroupRule{{
			ID: "no-ads", Name: "No ads", Enabled: true, Action: gatesentryPolicy.ActionBlock,
			Target:     gatesentryPolicy.RuleTarget{Domains: []string{"media.example"}},
			URLRegexes: []string{"/ads/.*"},
		}},
	})

	decision := policyProxyDecision("192.0.2.10", "", "media.example")
	if decision == nil || decision.Block || !decision.Inspect {
		t.Fatalf("decision = %+v, want inspection without a whole-host block", decision)
	}
	if len(decision.BlockURLRegexes) != 1 || decision.BlockURLRegexes[0] != "/ads/.*" {
		t.Fatalf("patterns = %v, want the rule's pattern", decision.BlockURLRegexes)
	}
	if !decision.BlocksURL("http://media.example/ads/banner") || decision.BlocksURL("http://media.example/article") {
		t.Fatal("URL pattern did not narrow the block to the matching path")
	}
}

// A rule scoped to one user decides only that user's request.
func TestPolicyProxyDecisionRuleUserScope(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "kids", Name: "Kids",
		Users: []string{"dana", "vivienne"},
		Rules: []gatesentryPolicy.GroupRule{{
			ID: "no-media", Enabled: true, Action: gatesentryPolicy.ActionBlock,
			Target: gatesentryPolicy.RuleTarget{Domains: []string{"media.example"}},
			Users:  []string{"vivienne"},
		}},
	})

	if other := policyProxyDecision("192.0.2.10", "dana", "media.example"); other != nil {
		t.Fatalf("decision = %+v, want nil: the rule is scoped to another user", other)
	}
	if scoped := policyProxyDecision("192.0.2.10", "vivienne", "media.example"); scoped == nil || !scoped.Block {
		t.Fatalf("decision = %+v, want the rule's block", scoped)
	}
}
