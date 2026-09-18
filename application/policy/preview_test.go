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
		{ID: "kids", Name: "Kids", Action: ActionBlock, Domains: []string{"*.games.example"}},
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
		{ID: "kids", Name: "Kids", Action: ActionBlock, Domains: []string{"*.games.example"}},
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
			{ID: "unrestricted", Name: "Unrestricted", Action: ActionAllow, Domains: []string{"*.games.example"}},
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
		{ID: "kids", Name: "Kids", Action: ActionBlock, Domains: []string{"*.games.example"}},
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
	names := make([]string, len(result.Active.Stages))
	for i, s := range result.Active.Stages {
		names[i] = s.Name
	}
	// Expect exception, group, schedule, pause, group_domain in order.
	want := []string{"exception", "group", "schedule", "pause", "group_domain"}
	for i, w := range want {
		if i >= len(names) {
			t.Fatalf("stage %d missing; have %v", i, names)
		}
		if names[i] != w {
			t.Fatalf("stage %d = %s, want %s (trail=%v)", i, names[i], w, names)
		}
	}
	// The group_domain stage should be applied and carry the matched pattern.
	gd := result.Active.Stages[4]
	if !gd.Applied || gd.Detail != "*.games.example" {
		t.Fatalf("group_domain stage = %+v, want applied with *.games.example", gd)
	}
}

// TestPreviewSurfacesInapplicableAndUnevaluatedConditions verifies that DNS
// inapplicable conditions are listed and that the proxy protocol surfaces them
// as unevaluated (the policy service does not run URL/MIME/inspection).
func TestPreviewSurfacesInapplicableAndUnevaluatedConditions(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Name: "Kids", Action: ActionBlock, Domains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	// DNS: url_regex, content_type, mitm are inapplicable.
	dnsResult := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: "chess.games.example", Protocol: "dns"})
	if len(dnsResult.Active.InapplicableConditions) != 3 {
		t.Fatalf("dns inapplicable = %v, want 3", dnsResult.Active.InapplicableConditions)
	}
	if len(dnsResult.Active.UnevaluatedConditions) != 0 {
		t.Fatalf("dns unevaluated = %v, want 0", dnsResult.Active.UnevaluatedConditions)
	}

	// Proxy: the policy service does not evaluate URL/MIME/inspection.
	proxyResult := svc.Preview(PreviewRequest{ClientIP: "192.0.2.10", Domain: "chess.games.example", Protocol: "proxy", URL: "/play", MIMEType: "text/html"})
	if len(proxyResult.Active.InapplicableConditions) != 0 {
		t.Fatalf("proxy inapplicable = %v, want 0", proxyResult.Active.InapplicableConditions)
	}
	if len(proxyResult.Active.UnevaluatedConditions) != 3 {
		t.Fatalf("proxy unevaluated = %v, want 3", proxyResult.Active.UnevaluatedConditions)
	}
}

// TestPreviewExceptionOverridesGroupBlock verifies the preview honors live
// exceptions (device > group > installation) without persistence.
func TestPreviewExceptionOverridesGroupBlock(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Name: "Kids", Action: ActionBlock, Domains: []string{"*.games.example"}},
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
	if result.Active.Stages[0].Name != "exception" || !result.Active.Stages[0].Applied {
		t.Fatalf("exception stage not applied: %+v", result.Active.Stages[0])
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
		{ID: "kids", Name: "Kids", Action: ActionBlock, Domains: []string{"*.games.example"}, Schedule: school},
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
