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
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"*.games.example"},
	})

	match := policyProxyDecision("192.0.2.10", "", "chess.games.example")
	proxy, ok := match.(gatesentryPolicy.ProxyMatch)
	if !ok {
		t.Fatalf("match = %#v, want a policy.ProxyMatch", match)
	}
	if !proxy.Matched || !proxy.ShouldBlock {
		t.Fatalf("match = %+v, want a block", proxy)
	}
	if proxy.GroupID != "kids" || proxy.MatchedDomain != "*.games.example" {
		t.Fatalf("match = %+v, want the kids group and the matched pattern", proxy)
	}
}

// A request no group covers must return nil so the proxy keeps the gateway's
// own settings; a group never changes inspection for traffic it does not own.
func TestPolicyProxyDecisionWithoutAGroupIsUnhandled(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "other", Action: gatesentryPolicy.ActionBlock, Domains: []string{"other.example"},
	})

	if match := policyProxyDecision("192.0.2.10", "dana", "uncovered.example"); match != nil {
		t.Fatalf("match = %#v, want nil for an uncovered domain", match)
	}
}

// An allow rule inside a block group is an exception: the proxy forwards the
// request and the gateway defaults apply.
func TestPolicyProxyDecisionAllowRuleIsAnException(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"*.games.example"},
		Rules: []gatesentryPolicy.GroupRule{{
			ID: "allow-chess", Enabled: true, Action: gatesentryPolicy.ActionAllow,
		}},
	})

	if match := policyProxyDecision("192.0.2.10", "", "chess.games.example"); match != nil {
		t.Fatalf("match = %#v, want nil for a rule exception", match)
	}
}

// An allow group with no rules exempts the domain: the proxy forwards it.
func TestPolicyProxyDecisionAllowGroupIsAnException(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "work", Action: gatesentryPolicy.ActionAllow, Domains: []string{"approved.example"},
	})

	if match := policyProxyDecision("192.0.2.10", "", "approved.example"); match != nil {
		t.Fatalf("match = %#v, want nil for an allow group", match)
	}
}

// The unauthenticated proxy passes the client address as the user fallback.
// It must resolve to the device identity, never to a group keyed by that
// address string.
func TestPolicyProxyDecisionIPAddressUserFallbackUsesDeviceIdentity(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc,
		gatesentryPolicy.PolicyGroup{ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"games.example"}},
		gatesentryPolicy.PolicyGroup{ID: "user-group", Action: gatesentryPolicy.ActionBlock, Domains: []string{"games.example"}, Users: []string{"192.0.2.10"}},
	)

	match, ok := policyProxyDecision("192.0.2.10", "192.0.2.10", "games.example").(gatesentryPolicy.ProxyMatch)
	if !ok || !match.Matched || !match.ShouldBlock || match.GroupID != "kids" {
		t.Fatalf("match = %+v, ok = %v, want the device group", match, ok)
	}
}

// A rule that carries URL patterns narrows its group's block to a path: the
// proxy receives the patterns and the inspection requirement instead of a
// whole-host denial.
func TestPolicyProxyDecisionRuleCarriesURLPatterns(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"media.example"},
		Rules: []gatesentryPolicy.GroupRule{{
			ID: "no-ads", Name: "No ads", Enabled: true, Action: gatesentryPolicy.ActionBlock,
			URLRegexes: []string{"/ads/.*"},
		}},
	})

	match, ok := policyProxyDecision("192.0.2.10", "", "media.example").(gatesentryPolicy.ProxyMatch)
	if !ok || !match.Matched || !match.ShouldBlock {
		t.Fatalf("match = %+v, ok = %v, want a block", match, ok)
	}
	if !match.ShouldMITM {
		t.Fatal("URL conditions exist only inside a decrypted request, so the match must require inspection")
	}
	if len(match.BlockURLRegexes) != 1 || match.BlockURLRegexes[0] != "/ads/.*" {
		t.Fatalf("patterns = %v, want the rule's pattern", match.BlockURLRegexes)
	}
	if match.RuleID != "no-ads" {
		t.Fatalf("rule = %q, want no-ads", match.RuleID)
	}
}

// A rule scoped to one user must not decide another user's request; the group
// action still applies to the domain. The group reaches both logins by
// membership, so only the rule's own scope separates them.
func TestPolicyProxyDecisionRuleUserScopeFallsBackToGroupAction(t *testing.T) {
	svc := newDecisionTestService(t)
	saveDecisionGroups(t, svc, gatesentryPolicy.PolicyGroup{
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"media.example"},
		Users: []string{"dana", "vivienne"},
		Rules: []gatesentryPolicy.GroupRule{{
			ID: "no-ads", Enabled: true, Action: gatesentryPolicy.ActionBlock,
			URLRegexes: []string{"/ads/.*"}, Users: []string{"vivienne"},
		}},
	})

	other, ok := policyProxyDecision("192.0.2.10", "dana", "media.example").(gatesentryPolicy.ProxyMatch)
	if !ok || !other.Matched || !other.ShouldBlock {
		t.Fatalf("match = %+v, ok = %v, want the group action", other, ok)
	}
	if other.RuleID != "" || len(other.BlockURLRegexes) != 0 {
		t.Fatalf("match = %+v, want no rule: the rule is scoped to another user", other)
	}

	scoped, ok := policyProxyDecision("192.0.2.10", "vivienne", "media.example").(gatesentryPolicy.ProxyMatch)
	if !ok || !scoped.Matched || !scoped.ShouldBlock {
		t.Fatalf("match = %+v, ok = %v, want the rule's block", scoped, ok)
	}
	if scoped.RuleID != "no-ads" || len(scoped.BlockURLRegexes) != 1 {
		t.Fatalf("match = %+v, want the rule's own pattern", scoped)
	}
}
