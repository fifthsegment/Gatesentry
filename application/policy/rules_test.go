package policy

import (
	"strings"
	"testing"
	"time"
)

// evalFor evaluates one policy for one domain with no pause, no categories,
// and a fixed noon clock, so a test states only the policy it is about.
func evalFor(group PolicyGroup, domain string, layer DecisionLayer) groupOutcome {
	return evaluateGroup(group, evalRequest{
		domain: domain,
		now:    time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		layer:  layer,
	})
}

// scheduledBlock is a policy that blocks the given domains during a schedule,
// which is how a time-limited block is written now that schedules belong to
// rules.
func scheduledBlock(id string, schedule *Schedule, domains ...string) PolicyGroup {
	return PolicyGroup{ID: id, Name: id, Rules: []GroupRule{{
		ID: id + "-schedule", Name: "Scheduled block", Enabled: true, Action: ActionBlock,
		Target: RuleTarget{Domains: domains}, Schedule: schedule,
	}}}
}

func TestMatchDomainPlainDomainCoversSubdomains(t *testing.T) {
	cases := []struct {
		pattern, domain string
		want            bool
	}{
		{"tiktok.com", "tiktok.com", true},
		{"tiktok.com", "www.tiktok.com", true},
		{"tiktok.com", "a.b.tiktok.com", true},
		{"tiktok.com", "nottiktok.com", false},
		{"*.example.com", "example.com", false},
		{"*.example.com", "www.example.com", true},
		{"Example.COM", "WWW.example.com.", true},
	}
	for _, c := range cases {
		if got := matchDomain(c.pattern, c.domain); got != c.want {
			t.Errorf("matchDomain(%q, %q) = %v, want %v", c.pattern, c.domain, got, c.want)
		}
	}
}

func TestNormalizeDomainPatternStripsURLParts(t *testing.T) {
	cases := map[string]string{
		" https://WWW.Example.com/path?q=1 ": "www.example.com",
		"example.com:443":                    "example.com",
		"*.example.com.":                     "*.example.com",
	}
	for in, want := range cases {
		got, err := NormalizeDomainPattern(in)
		if err != nil || got != want {
			t.Errorf("NormalizeDomainPattern(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"exa mple.com", "foo*.com", "*."} {
		if _, err := NormalizeDomainPattern(bad); err == nil {
			t.Errorf("NormalizeDomainPattern(%q) accepted a non-domain", bad)
		}
	}
}

func TestNormalizeGroupRulesAssignsIDsAndTrimsValues(t *testing.T) {
	rules, err := NormalizeGroupRules([]GroupRule{{
		Name:                "  Block video  ",
		Enabled:             true,
		Action:              ActionBlock,
		Target:              RuleTarget{Domains: []string{" Videos.Example "}},
		URLRegexes:          []string{" ^/media/ ", ""},
		BlockedContentTypes: []string{" VIDEO/ ", ""},
		Users:               []string{" alice ", ""},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rule := rules[0]
	if rule.ID == "" || rule.Name != "Block video" {
		t.Fatalf("rule = %+v", rule)
	}
	if len(rule.Target.Domains) != 1 || rule.Target.Domains[0] != "videos.example" {
		t.Fatalf("target = %+v", rule.Target)
	}
	if len(rule.URLRegexes) != 1 || rule.URLRegexes[0] != "^/media/" {
		t.Fatalf("url regexes = %v", rule.URLRegexes)
	}
	if len(rule.BlockedContentTypes) != 1 || rule.BlockedContentTypes[0] != "video/" {
		t.Fatalf("content types = %v", rule.BlockedContentTypes)
	}
	if len(rule.Users) != 1 || rule.Users[0] != "alice" {
		t.Fatalf("users = %v", rule.Users)
	}
}

func TestNormalizeGroupRulesAllTrafficDropsLists(t *testing.T) {
	rules, err := NormalizeGroupRules([]GroupRule{{
		Enabled: true, Action: ActionBlock,
		Target: RuleTarget{AllTraffic: true, Domains: []string{"x.example"}, Categories: []string{"social"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules[0].Target.Domains) != 0 || len(rules[0].Target.Categories) != 0 {
		t.Fatalf("target = %+v, want all traffic only", rules[0].Target)
	}
}

func TestNormalizeGroupRulesRejectsMalformedRecords(t *testing.T) {
	target := RuleTarget{Domains: []string{"example.com"}}
	cases := map[string]GroupRule{
		"no action":            {Name: "x", Enabled: true, Target: target},
		"bad action":           {Action: "deny", Enabled: true, Target: target},
		"no target":            {Action: ActionBlock, Enabled: true},
		"bad regex":            {Action: ActionBlock, Enabled: true, Target: target, URLRegexes: []string{"("}},
		"allow with url":       {Action: ActionAllow, Enabled: true, Target: target, URLRegexes: []string{"^/x"}},
		"allow with type":      {Action: ActionAllow, Enabled: true, Target: target, BlockedContentTypes: []string{"video/"}},
		"unknown category":     {Action: ActionBlock, Enabled: true, Target: RuleTarget{Categories: []string{"nope"}}},
		"address as user":      {Action: ActionBlock, Enabled: true, Target: target, Users: []string{"192.0.2.1"}},
		"bad schedule":         {Action: ActionBlock, Enabled: true, Target: target, Schedule: &Schedule{Windows: []TimeWindow{{From: "25:00", To: "07:00"}}}},
		"bad domain in target": {Action: ActionBlock, Enabled: true, Target: RuleTarget{Domains: []string{"not a domain"}}},
	}
	for name, rule := range cases {
		if _, err := NormalizeGroupRules([]GroupRule{rule}); err == nil {
			t.Errorf("%s: expected a rejection", name)
		}
	}
}

func TestNormalizeGroupRulesAcceptsADisabledRuleWithoutATarget(t *testing.T) {
	// A half-written rule can be saved switched off and finished later.
	if _, err := NormalizeGroupRules([]GroupRule{{Action: ActionBlock}}); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyListsDecideWithoutRules(t *testing.T) {
	group := PolicyGroup{
		ID:             "kids",
		BlockedDomains: []string{"tiktok.com"},
		AllowedDomains: []string{"school.tiktok.com"},
	}
	if out := evalFor(group, "www.tiktok.com", LayerDNS); out.Action != ActionBlock {
		t.Fatalf("subdomain of a blocked domain = %+v, want block", out)
	}
	if out := evalFor(group, "school.tiktok.com", LayerDNS); out.Action != ActionAllow {
		t.Fatalf("allowed domain inside a blocked one = %+v, want allow", out)
	}
	if out := evalFor(group, "example.com", LayerDNS); out.Action != ActionNone {
		t.Fatalf("uncovered domain = %+v, want none", out)
	}
}

func TestAllowedDomainExemptsABlockedCategory(t *testing.T) {
	index := NewCategoryIndex()
	index.Replace("social", []string{"reddit.com"}, time.Now())
	group := PolicyGroup{BlockedCategories: []string{"social"}, AllowedDomains: []string{"reddit.com"}}
	out := evaluateGroup(group, evalRequest{domain: "old.reddit.com", layer: LayerDNS, index: index, now: time.Now()})
	if out.Action != ActionAllow {
		t.Fatalf("outcome = %+v, want the allowed domain to win over the category", out)
	}
}

func TestAllowedDomainExemptsAGatewayCategory(t *testing.T) {
	index := NewCategoryIndex()
	index.Replace("ads", []string{"tracker.example"}, time.Now())
	req := evalRequest{domain: "tracker.example", layer: LayerDNS, index: index, now: time.Now(), gatewayCategories: []string{"ads"}}
	if out := evaluateGroup(PolicyGroup{}, req); out.Action != ActionBlock {
		t.Fatalf("gateway category = %+v, want block for a policy that does not allow it", out)
	}
	if out := evaluateGroup(PolicyGroup{AllowedDomains: []string{"tracker.example"}}, req); out.Action != ActionAllow {
		t.Fatalf("allowed domain = %+v, want allow over the gateway category", out)
	}
}

func TestRuleTargetsItsOwnDomains(t *testing.T) {
	// The bug this model fixes: a policy could only hold one action for one
	// set of domains, so "block TikTok, allow YouTube" needed two policies and
	// a device can only have one.
	group := PolicyGroup{Rules: []GroupRule{
		{ID: "a", Enabled: true, Action: ActionAllow, Target: RuleTarget{Domains: []string{"youtube.com"}}},
		{ID: "b", Enabled: true, Action: ActionBlock, Target: RuleTarget{Domains: []string{"tiktok.com"}}},
	}}
	if out := evalFor(group, "www.youtube.com", LayerDNS); out.Action != ActionAllow || out.RuleID != "a" {
		t.Fatalf("youtube = %+v", out)
	}
	if out := evalFor(group, "www.tiktok.com", LayerDNS); out.Action != ActionBlock || out.RuleID != "b" {
		t.Fatalf("tiktok = %+v", out)
	}
	if out := evalFor(group, "example.com", LayerDNS); out.Action != ActionNone {
		t.Fatalf("other = %+v", out)
	}
}

func TestRulesAreFirstMatchInListOrder(t *testing.T) {
	group := PolicyGroup{Rules: []GroupRule{
		{ID: "allow-homework", Enabled: true, Action: ActionAllow, Target: RuleTarget{Domains: []string{"khanacademy.org"}}},
		{ID: "no-internet", Enabled: true, Action: ActionBlock, Target: RuleTarget{AllTraffic: true}},
	}}
	if out := evalFor(group, "www.khanacademy.org", LayerDNS); out.Action != ActionAllow {
		t.Fatalf("the earlier allow must win: %+v", out)
	}
	if out := evalFor(group, "example.com", LayerDNS); out.Action != ActionBlock || out.Matched != "*" {
		t.Fatalf("all traffic = %+v", out)
	}
}

func TestRulesComeBeforeTheLists(t *testing.T) {
	group := PolicyGroup{
		BlockedDomains: []string{"youtube.com"},
		Rules: []GroupRule{{
			ID: "weekend", Enabled: true, Action: ActionAllow,
			Target: RuleTarget{Domains: []string{"youtube.com"}},
		}},
	}
	if out := evalFor(group, "www.youtube.com", LayerDNS); out.Action != ActionAllow {
		t.Fatalf("outcome = %+v, want the rule to override the blocked list", out)
	}
}

func TestDisabledRuleIsIgnored(t *testing.T) {
	group := PolicyGroup{Rules: []GroupRule{{ID: "r", Enabled: false, Action: ActionBlock, Target: RuleTarget{AllTraffic: true}}}}
	if out := evalFor(group, "example.com", LayerDNS); out.Action != ActionNone {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestRuleScheduleScopesTheRule(t *testing.T) {
	bedtime := &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "20:00", To: "07:00"}}}
	group := PolicyGroup{Rules: []GroupRule{{ID: "bed", Enabled: true, Action: ActionBlock, Target: RuleTarget{AllTraffic: true}, Schedule: bedtime}}}
	req := evalRequest{domain: "example.com", layer: LayerDNS}
	req.now = time.Date(2026, 9, 23, 21, 0, 0, 0, time.UTC)
	if out := evaluateGroup(group, req); out.Action != ActionBlock {
		t.Fatalf("inside the window = %+v, want block", out)
	}
	req.now = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if out := evaluateGroup(group, req); out.Action != ActionNone {
		t.Fatalf("outside the window = %+v, want none", out)
	}
}

func TestPauseLiftsRuleAndListBlocksButNotAllows(t *testing.T) {
	// The bug this fixes: a pause only checked the old group-level action, so
	// a policy that blocked through a rule kept blocking while "paused".
	group := PolicyGroup{
		BlockedDomains: []string{"games.example"},
		AllowedDomains: []string{"school.example"},
		Rules:          []GroupRule{{ID: "r", Enabled: true, Action: ActionBlock, Target: RuleTarget{Domains: []string{"tiktok.com"}}}},
	}
	for _, domain := range []string{"tiktok.com", "games.example"} {
		out := evaluateGroup(group, evalRequest{domain: domain, layer: LayerDNS, paused: true, now: time.Now()})
		if out.Action == ActionBlock {
			t.Fatalf("%s paused = %+v, want no block", domain, out)
		}
	}
	out := evaluateGroup(group, evalRequest{domain: "school.example", layer: LayerDNS, paused: true, now: time.Now()})
	if out.Action != ActionAllow {
		t.Fatalf("allow while paused = %+v, want allow", out)
	}
}

func TestDNSSkipsARuleNarrowedToURLs(t *testing.T) {
	// The bug this fixes: a rule meant to block only youtube.com/shorts
	// blocked the whole of YouTube on DNS, because DNS applied the rule's
	// block to the domain and ignored the URL condition.
	group := PolicyGroup{Rules: []GroupRule{{
		ID: "shorts", Enabled: true, Action: ActionBlock,
		Target:     RuleTarget{Domains: []string{"youtube.com"}},
		URLRegexes: []string{"/shorts"},
	}}}
	out := evalFor(group, "www.youtube.com", LayerDNS)
	if out.Action != ActionNone {
		t.Fatalf("DNS outcome = %+v, want the domain left to the proxy", out)
	}
	if len(out.ProxyOnly) != 1 || out.ProxyOnly[0] != ConditionURL {
		t.Fatalf("proxy-only = %v", out.ProxyOnly)
	}
}

func TestDNSSkipsAUserScopedRule(t *testing.T) {
	group := PolicyGroup{Rules: []GroupRule{{
		ID: "alice", Enabled: true, Action: ActionBlock,
		Target: RuleTarget{AllTraffic: true}, Users: []string{"alice"},
	}}}
	if out := evalFor(group, "example.com", LayerDNS); out.Action != ActionNone {
		t.Fatalf("DNS outcome = %+v, want a user rule left to the proxy", out)
	}
}

func TestProxyCollectsConditionsAndContinues(t *testing.T) {
	group := PolicyGroup{
		AllowedDomains: []string{"youtube.com"},
		Rules: []GroupRule{{
			ID: "shorts", Name: "No shorts", Enabled: true, Action: ActionBlock,
			Target:     RuleTarget{Domains: []string{"youtube.com"}},
			URLRegexes: []string{"/shorts"},
		}},
	}
	out := evalFor(group, "www.youtube.com", LayerExplicitProxy)
	if out.Action != ActionAllow {
		t.Fatalf("decisive outcome = %+v, want the allowed domain", out)
	}
	if len(out.URLRegexes) != 1 || out.ConditionRuleID != "shorts" {
		t.Fatalf("conditions = %+v", out)
	}
}

func TestProxyAppliesUserScopedRuleOnlyToThatUser(t *testing.T) {
	group := PolicyGroup{Rules: []GroupRule{{
		ID: "alice", Enabled: true, Action: ActionBlock,
		Target: RuleTarget{AllTraffic: true}, Users: []string{"alice"},
	}}}
	req := evalRequest{domain: "example.com", layer: LayerExplicitProxy, now: time.Now()}
	req.user = "alice"
	if out := evaluateGroup(group, req); out.Action != ActionBlock {
		t.Fatalf("alice = %+v, want block", out)
	}
	req.user = "bob"
	if out := evaluateGroup(group, req); out.Action != ActionNone {
		t.Fatalf("bob = %+v, want none", out)
	}
}

func TestTraceExplainsTheDecision(t *testing.T) {
	group := PolicyGroup{
		BlockedDomains: []string{"tiktok.com"},
		Rules: []GroupRule{{
			ID: "bed", Name: "Bedtime", Enabled: true, Action: ActionBlock, Target: RuleTarget{AllTraffic: true},
			Schedule: &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "20:00", To: "07:00"}}},
		}},
	}
	out := evalFor(group, "tiktok.com", LayerDNS)
	if len(out.Trace) != 2 {
		t.Fatalf("trace = %+v, want the skipped rule and the deciding list", out.Trace)
	}
	if !strings.Contains(out.Trace[0].Detail, "outside its schedule") || out.Trace[1].Stage != "blocked_domains" || !out.Trace[1].Applied {
		t.Fatalf("trace = %+v", out.Trace)
	}
}

func TestDecodeLegacyRulesAcceptsBothStoredShapes(t *testing.T) {
	array, err := DecodeLegacyRules(`[{"id":"r1","domain":"example.com","action":"block"}]`)
	if err != nil {
		t.Fatal(err)
	}
	list, err := DecodeLegacyRules(`{"rules":[{"id":"r1","domain":"example.com","action":"block"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(array) != 1 || len(list) != 1 {
		t.Fatalf("array = %d, list = %d, want 1 each", len(array), len(list))
	}
	empty, err := DecodeLegacyRules("")
	if err != nil || empty != nil {
		t.Fatalf("empty = %v, err = %v", empty, err)
	}
}

func TestMigrateLegacyRulesCarriesEnforcementIntoAnUnassignedGroup(t *testing.T) {
	groups, err := MigrateLegacyRules([]LegacyRule{{
		ID: "r1", Name: "No games", Enabled: true, Domain: "*.games.example",
		Action: "block", MITMAction: "enable", BlockType: "both",
		URLRegexPatterns:    []string{"^/play/"},
		BlockedContentTypes: []string{"video/"},
		Users:               []string{"alice"},
		TimeRestriction:     &LegacyTimeRestriction{From: "20:00", To: "07:00"},
	}}, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	group := groups[0]
	if group.ID != "legacy-rule-r1" || len(group.Rules) != 1 {
		t.Fatalf("group = %+v", group)
	}
	rule := group.Rules[0]
	if !rule.Enabled || rule.Action != ActionBlock || len(rule.Target.Domains) != 1 || rule.Target.Domains[0] != "games.example" {
		t.Fatalf("rule = %+v", rule)
	}
	if len(rule.URLRegexes) != 1 || len(rule.BlockedContentTypes) != 1 || len(rule.Users) != 1 {
		t.Fatalf("conditions = %+v", rule)
	}
	if rule.Schedule == nil || rule.Schedule.Timezone != "America/New_York" {
		t.Fatalf("schedule = %+v", rule.Schedule)
	}
}

func TestMigrateLegacyRulesKeepsADisabledRuleFromEnforcing(t *testing.T) {
	groups, err := MigrateLegacyRules([]LegacyRule{{
		ID: "r1", Domain: "example.com", Action: "block", Enabled: false,
	}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if out := evalFor(groups[0], "example.com", LayerDNS); out.Action != ActionNone {
		t.Fatalf("outcome = %+v, want none", out)
	}
}

func TestMigrateLegacyRulesOnlyImportsConditionsTheOldEngineApplied(t *testing.T) {
	groups, err := MigrateLegacyRules([]LegacyRule{{
		ID: "r1", Domain: "example.com", Action: "block", Enabled: true,
		MITMAction: "disable", BlockType: "both",
		URLRegexPatterns: []string{"^/play/"}, BlockedContentTypes: []string{"video/"},
	}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if rule := groups[0].Rules[0]; len(rule.URLRegexes) != 0 || len(rule.BlockedContentTypes) != 0 {
		t.Fatalf("rule = %+v, want no conditions without inspection", rule)
	}
	groups, err = MigrateLegacyRules([]LegacyRule{{
		ID: "r2", Domain: "example.com", Action: "allow", Enabled: true,
		MITMAction: "enable", BlockType: "both",
		URLRegexPatterns: []string{"^/play/"}, BlockedContentTypes: []string{"video/"},
	}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if rule := groups[0].Rules[0]; len(rule.URLRegexes) != 0 || len(rule.BlockedContentTypes) != 0 {
		t.Fatalf("rule = %+v, want no conditions on an allow rule", rule)
	}
}

func TestMigrateLegacyRulesDropsAnUnrepresentableWindowAndSaysSo(t *testing.T) {
	groups, err := MigrateLegacyRules([]LegacyRule{{
		ID: "r1", Domain: "example.com", Action: "block", Enabled: true,
		TimeRestriction: &LegacyTimeRestriction{From: "25:00", To: "07:00"},
	}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if groups[0].Rules[0].Schedule != nil {
		t.Fatal("an invalid window was imported")
	}
	if !strings.Contains(groups[0].Description, "could not be represented") {
		t.Fatalf("description = %q, want the dropped window explained", groups[0].Description)
	}
}

func TestMigrateLegacyRulesRejectsARuleWithoutADomain(t *testing.T) {
	if _, err := MigrateLegacyRules([]LegacyRule{{ID: "r1", Action: "block"}}, "UTC"); err == nil {
		t.Fatal("expected a rejection for a rule with no domain")
	}
}

func TestEvaluateProxyBlocksOnlyMatchingURLs(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "tablet"}}
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids",
		Rules: []GroupRule{{
			ID: "shorts", Name: "No shorts", Enabled: true, Action: ActionBlock,
			Target:     RuleTarget{Domains: []string{"youtube.com"}},
			URLRegexes: []string{"/shorts"},
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("tablet", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	match := svc.EvaluateProxy(svc.ResolveIdentity("192.0.2.10", ""), "www.youtube.com")
	if !match.Matched || match.ShouldBlock || !match.ShouldMITM || len(match.BlockURLRegexes) != 1 || match.RuleID != "shorts" {
		t.Fatalf("match = %+v, want inspection with the URL condition and no domain block", match)
	}
	if dns := svc.EvaluateDNS(svc.ResolveIdentityForDNS("192.0.2.10", ""), "www.youtube.com"); dns.Action != ActionNone {
		t.Fatalf("DNS = %+v, want the domain left resolvable", dns)
	}
}

func TestEvaluateProxyReportsAnAllowSoTheBlocklistIsExempt(t *testing.T) {
	svc := newTestService(t)
	if err := svc.UpdateGroup(DefaultGroupID, PolicyGroup{Name: "Default", AllowedDomains: []string{"bank.example"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	match := svc.EvaluateProxy(svc.ResolveIdentity("198.51.100.1", ""), "online.bank.example")
	if !match.Matched || !match.Allowed || match.ShouldBlock || match.ShouldMITM {
		t.Fatalf("match = %+v, want an allow that leaves inspection to the gateway", match)
	}
}
