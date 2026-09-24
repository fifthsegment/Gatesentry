package policy

import (
	"testing"
	"time"
)

// TestPreviewMatchesLiveEvaluation proves preview/live parity for
// deterministic cases: the pure evaluator core is the single decision path,
// so Preview and EvaluateDNS cannot diverge for the same identity and domain.
func TestPreviewMatchesLiveEvaluation(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	// Rebind the resolver so device-1 is known.
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	// Swap in the device-aware resolver the handler tests use.
	svc.devices = resolver

	for _, domain := range []string{"chess.games.example", "safe.example", "board.games.example"} {
		identity := svc.ResolveIdentityForDNS("192.0.2.10", "")
		live := svc.EvaluateDNS(identity, domain)

		result := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: domain, Protocol: "dns"})
		if result.Active.Action != live.Action {
			t.Fatalf("domain %s: preview active action %s != live %s", domain, result.Active.Action, live.Action)
		}
		if result.Active.GroupID != live.GroupID {
			t.Fatalf("domain %s: preview group %s != live group %s", domain, result.Active.GroupID, live.GroupID)
		}
		if result.Active.MatchedDomain != live.MatchedDomain {
			t.Fatalf("domain %s: preview matched %s != live matched %s", domain, result.Active.MatchedDomain, live.MatchedDomain)
		}
		if result.Active.Reason != live.Reason {
			t.Fatalf("domain %s: preview reason %q != live reason %q", domain, result.Active.Reason, live.Reason)
		}
	}
}

// TestPreviewProposedPolicyComparison verifies the acceptance criterion
// "compare a proposed policy with the active revision without persisting."
func TestPreviewProposedPolicyComparison(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	domain := "chess.games.example"

	// Active: device-1 is in kids, which blocks *.games.example.
	result := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: domain})
	if result.Active.Action != ActionBlock {
		t.Fatalf("active action = %s, want block", result.Active.Action)
	}
	if result.Changed {
		t.Fatalf("changed = true, want false with no proposed policy")
	}

	// Proposed: move device-1 to an allow group that allows *.games.example.
	proposed := &ProposedPolicy{
		Groups: []PolicyGroup{
			{ID: "unrestricted", Name: "Unrestricted", AllowedDomains: []string{"*.games.example"}},
		},
		Assignments: []DeviceAssignment{{DeviceID: "device-1", GroupID: "unrestricted"}},
	}
	result = svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: domain, Proposed: proposed})
	if result.Active.Action != ActionBlock {
		t.Fatalf("active still block = %s, want block (proposed must not persist)", result.Active.Action)
	}
	if result.Proposed.Action != ActionAllow {
		t.Fatalf("proposed action = %s, want allow", result.Proposed.Action)
	}
	if result.Proposed.GroupID != "unrestricted" {
		t.Fatalf("proposed group = %s, want unrestricted", result.Proposed.GroupID)
	}
	if !result.Changed {
		t.Fatalf("changed = false, want true when proposed alters the decision")
	}

	// Active policy must be untouched by preview.
	snap := svc.Snapshot()
	if _, ok := snap.Groups["unrestricted"]; ok {
		t.Fatal("proposed group persisted; preview must not write")
	}
	if snap.Assignments["device-1"] != "kids" {
		t.Fatalf("active assignment = %s, want kids (preview must not persist)", snap.Assignments["device-1"])
	}
}

// TestPreviewShowsMatchedRulesTrail verifies the precedence stages surface the
// matched pattern and each precedence decision.
func TestPreviewShowsMatchedRulesTrail(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	result := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: "chess.games.example"})
	trace := result.Active.Trace
	if len(trace) == 0 {
		t.Fatal("preview returned no trace")
	}
	last := trace[len(trace)-1]
	if last.Stage != "blocked_domains" || !last.Applied || last.Detail != "*.games.example" {
		t.Fatalf("deciding step = %+v, want the blocked domain pattern", last)
	}
	if result.Active.GroupName != "Kids" {
		t.Fatalf("group name = %q, want Kids", result.Active.GroupName)
	}
}

// TestPreviewProxyLayerTestsTheURL verifies that a proxy preview reports the
// URL conditions of the rules that apply and whether the supplied URL matches,
// while a DNS preview reports the same rule as decided by the proxy.
func TestPreviewProxyLayerTestsTheURL(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids",
		Rules: []GroupRule{{
			ID: "shorts", Enabled: true, Action: ActionBlock,
			Target: RuleTarget{Domains: []string{"youtube.com"}}, URLRegexes: []string{"/shorts/"},
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	dns := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: "www.youtube.com"})
	if dns.Active.Action != ActionNone || len(dns.Active.ProxyOnly) != 1 {
		t.Fatalf("dns preview = %+v, want no DNS block and one proxy-only condition", dns.Active)
	}

	blocked := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Protocol: "proxy", URL: "https://www.youtube.com/shorts/abc"})
	if !blocked.Active.URLBlocked || len(blocked.Active.BlockURLRegexes) != 1 {
		t.Fatalf("proxy preview of a shorts URL = %+v, want the URL blocked", blocked.Active)
	}
	allowed := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Protocol: "proxy", URL: "https://www.youtube.com/watch?v=abc"})
	if allowed.Active.URLBlocked {
		t.Fatalf("proxy preview of a watch URL = %+v, want it allowed", allowed.Active)
	}
}

// TestPreviewExceptionOverridesGroupBlock verifies the preview honors live
// exceptions (device > group > installation) without persistence.
func TestPreviewExceptionOverridesGroupBlock(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	exc, err := svc.CreateException(CreateExceptionInput{
		Domain: "chess.games.example", Scope: ScopeDevice, DeviceID: "device-1",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}

	result := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: "chess.games.example"})
	if result.Active.Action != ActionAllow {
		t.Fatalf("active action = %s, want allow via exception", result.Active.Action)
	}
	if result.Active.Trace[0].Stage != "exception" || !result.Active.Trace[0].Applied {
		t.Fatalf("exception step not applied: %+v", result.Active.Trace)
	}
	_ = exc
}

// TestPreviewScheduleInactiveRespectsTimeOverride verifies the preview honors a
// supplied time so an admin can test schedule windows without waiting.
func TestPreviewScheduleInactiveRespectsTimeOverride(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	// School-day schedule: active 09:00-17:00 Mon-Fri.
	school := &Schedule{
		Timezone: "America/New_York",
		Weekdays: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	if err := svc.SaveGroups([]PolicyGroup{
		scheduledBlock("kids", school, "games.example"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	// Monday 12:00 -> schedule active -> block applies.
	activeTime := mustTime(loc, "2026-03-16 12:00:00")
	result := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: "chess.games.example", Time: activeTime.Format(time.RFC3339)})
	if result.Active.Action != ActionBlock {
		t.Fatalf("active during schedule = %s, want block", result.Active.Action)
	}
	// Sunday 12:00 -> schedule inactive -> no opinion.
	inactiveTime := mustTime(loc, "2026-03-15 12:00:00")
	result = svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: "chess.games.example", Time: inactiveTime.Format(time.RFC3339)})
	if result.Active.Action != ActionNone {
		t.Fatalf("active off-schedule = %s, want none", result.Active.Action)
	}
}
