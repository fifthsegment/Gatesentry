package policy

import (
	"strings"
	"testing"
	"time"
)

func groupWithRules(action PolicyAction, domains []string, rules ...GroupRule) PolicyGroup {
	return PolicyGroup{ID: "g1", Name: "Group", Action: action, Domains: domains, Rules: rules}
}

func TestNormalizeGroupRulesAssignsIDsAndTrimsValues(t *testing.T) {
	rules, err := NormalizeGroupRules([]GroupRule{{
		Name:                "  Block video  ",
		Enabled:             true,
		Action:              ActionBlock,
		MITMAction:          " enable ",
		URLRegexes:          []string{" ^/media/ ", ""},
		BlockedContentTypes: []string{" VIDEO/ ", ""},
		Users:               []string{" alice ", ""},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(rules))
	}
	rule := rules[0]
	if rule.ID == "" {
		t.Fatal("rule was stored without an id")
	}
	if rule.Name != "Block video" {
		t.Fatalf("name = %q", rule.Name)
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

func TestNormalizeGroupRulesKeepsStableIDs(t *testing.T) {
	rules, err := NormalizeGroupRules([]GroupRule{{ID: "rule-1", Action: ActionBlock}})
	if err != nil {
		t.Fatal(err)
	}
	if rules[0].ID != "rule-1" {
		t.Fatalf("id = %q, want rule-1", rules[0].ID)
	}
}

func TestNormalizeGroupRulesRejectsMalformedRecords(t *testing.T) {
	cases := map[string]GroupRule{
		"no action":         {Name: "x"},
		"bad action":        {Action: "deny"},
		"bad mitm":          {Action: ActionBlock, MITMAction: "inspect"},
		"bad regex":         {Action: ActionBlock, URLRegexes: []string{"(unclosed"}},
		"bad schedule":      {Action: ActionBlock, Schedule: &Schedule{Windows: []TimeWindow{{From: "25:00", To: "07:00"}}}},
		"allow with url":    {Action: ActionAllow, URLRegexes: []string{"^/a"}},
		"allow with mime":   {Action: ActionAllow, BlockedContentTypes: []string{"video/"}},
		"url with no mitm":  {Action: ActionBlock, MITMAction: MITMActionDisable, URLRegexes: []string{"^/a"}},
		"mime with no mitm": {Action: ActionBlock, MITMAction: MITMActionDisable, BlockedContentTypes: []string{"video/"}},
	}
	for name, rule := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NormalizeGroupRules([]GroupRule{rule}); err == nil {
				t.Fatal("expected a rejection, got none")
			}
		})
	}
}

func TestEnabledGroupRulesSortsByPriorityThenID(t *testing.T) {
	rules := EnabledGroupRules(groupWithRules(ActionBlock, nil,
		GroupRule{ID: "b", Enabled: true, Priority: 5, Action: ActionBlock},
		GroupRule{ID: "a", Enabled: true, Priority: 5, Action: ActionBlock},
		GroupRule{ID: "c", Enabled: true, Priority: 1, Action: ActionBlock},
		GroupRule{ID: "d", Enabled: false, Priority: 0, Action: ActionBlock},
	))
	got := make([]string, 0, len(rules))
	for _, rule := range rules {
		got = append(got, rule.ID)
	}
	if strings.Join(got, ",") != "c,a,b" {
		t.Fatalf("order = %v, want c,a,b", got)
	}
}

func TestGroupRulesOverrideTheGroupAction(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	group := groupWithRules(ActionBlock, []string{"example.com"},
		GroupRule{ID: "r1", Enabled: true, Action: ActionAllow, Users: []string{"alice"}},
	)
	if outcome := evaluateGroupRulesAt(group, "example.com", "", "alice", now, LayerDNS); outcome.Action != ActionAllow {
		t.Fatalf("alice outcome = %+v, want allow", outcome)
	}
	// The rule's user scope excludes everyone else, so the group action stands.
	if outcome := evaluateGroupRulesAt(group, "example.com", "", "bob", now, LayerDNS); outcome.Action != ActionBlock {
		t.Fatalf("bob outcome = %+v, want block", outcome)
	}
	if outcome := evaluateGroupRulesAt(group, "example.com", "", "", now, LayerDNS); outcome.Action != ActionBlock {
		t.Fatalf("anonymous outcome = %+v, want block", outcome)
	}
}

func TestGroupRuleScheduleScopesTheRule(t *testing.T) {
	group := groupWithRules(ActionBlock, []string{"example.com"},
		GroupRule{ID: "r1", Enabled: true, Action: ActionAllow, Schedule: &Schedule{
			Timezone: "UTC",
			Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
		}},
	)
	inside := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	outside := time.Date(2026, 9, 20, 20, 0, 0, 0, time.UTC)
	if outcome := evaluateGroupRulesAt(group, "example.com", "", "", inside, LayerDNS); outcome.Action != ActionAllow {
		t.Fatalf("inside outcome = %+v, want allow", outcome)
	}
	if outcome := evaluateGroupRulesAt(group, "example.com", "", "", outside, LayerDNS); outcome.Action != ActionBlock {
		t.Fatalf("outside outcome = %+v, want block", outcome)
	}
}

func TestDisabledGroupRuleIsIgnored(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	group := groupWithRules(ActionBlock, []string{"example.com"},
		GroupRule{ID: "r1", Enabled: false, Action: ActionAllow},
	)
	outcome := evaluateGroupRulesAt(group, "example.com", "", "", now, LayerDNS)
	if outcome.Action != ActionBlock || outcome.RuleID != "" {
		t.Fatalf("outcome = %+v, want the group action with no rule", outcome)
	}
}

func TestDNSReportsProxyOnlyConditionsAsUnresolved(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	group := groupWithRules(ActionNone, []string{"example.com"},
		GroupRule{ID: "r1", Name: "No video", Enabled: true, Action: ActionBlock,
			MITMAction: MITMActionEnable, URLRegexes: []string{"^/media/"}, BlockedContentTypes: []string{"video/"}},
	)
	outcome := evaluateGroupRulesAt(group, "example.com", "", "", now, LayerDNS)
	if outcome.Action != ActionBlock {
		t.Fatalf("action = %q, want block: DNS still blocks the domain", outcome.Action)
	}
	if strings.Join(outcome.Unresolved, ",") != "url_regex,content_type" {
		t.Fatalf("unresolved = %v, want url_regex,content_type", outcome.Unresolved)
	}
	if outcome.Reason != "group rule No video" {
		t.Fatalf("reason = %q", outcome.Reason)
	}

	proxy := evaluateGroupRulesAt(group, "example.com", "", "", now, LayerExplicitProxy)
	if len(proxy.Unresolved) != 0 {
		t.Fatalf("proxy unresolved = %v, want none", proxy.Unresolved)
	}
	if !proxy.ShouldMITM || len(proxy.BlockURLRegexes) != 1 || len(proxy.BlockContentTypes) != 1 {
		t.Fatalf("proxy outcome = %+v, want inspection with both conditions", proxy)
	}
}

func TestProxyLayerHonorsDisableInspectionOnABlockRule(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	group := groupWithRules(ActionNone, []string{"example.com"},
		GroupRule{ID: "r1", Enabled: true, Action: ActionBlock, MITMAction: MITMActionDisable},
	)
	outcome := evaluateGroupRulesAt(group, "example.com", "", "", now, LayerExplicitProxy)
	if outcome.Action != ActionBlock || outcome.ShouldMITM {
		t.Fatalf("outcome = %+v, want a block without inspection", outcome)
	}
}

func TestRuleReasonNamesTheCategory(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	group := groupWithRules(ActionNone, nil,
		GroupRule{ID: "r1", Name: "No social", Enabled: true, Action: ActionBlock},
	)
	outcome := evaluateGroupRulesAt(group, "category:social", "social", "", now, LayerDNS)
	if outcome.Reason != "group rule No social on category social" {
		t.Fatalf("reason = %q", outcome.Reason)
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
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	group := groups[0]
	if group.ID != "legacy-rule-r1" {
		t.Fatalf("id = %q", group.ID)
	}
	if group.Action != ActionBlock || len(group.Domains) != 1 || group.Domains[0] != "*.games.example" {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(group.Rules))
	}
	rule := group.Rules[0]
	if !rule.Enabled || rule.Action != ActionBlock || rule.MITMAction != MITMActionEnable {
		t.Fatalf("rule = %+v", rule)
	}
	if len(rule.URLRegexes) != 1 || len(rule.BlockedContentTypes) != 1 {
		t.Fatalf("conditions = %+v", rule)
	}
	if rule.Schedule == nil || rule.Schedule.Timezone != "America/New_York" {
		t.Fatalf("schedule = %+v", rule.Schedule)
	}
	if strings.Contains(group.Description, "could not be represented") {
		t.Fatalf("description dropped a representable window: %q", group.Description)
	}
}

func TestMigrateLegacyRulesKeepsADisabledRuleFromEnforcing(t *testing.T) {
	groups, err := MigrateLegacyRules([]LegacyRule{{
		ID: "r1", Domain: "example.com", Action: "block", Enabled: false,
	}}, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	group := groups[0]
	if group.Action != ActionNone {
		t.Fatalf("group action = %q, want none", group.Action)
	}
	if group.Rules[0].Enabled {
		t.Fatal("disabled rule imported enabled")
	}
	// The imported group must not enforce anything until it is edited.
	if outcome := evaluateGroupRulesAt(group, "example.com", "", "", time.Now(), LayerDNS); outcome.Action != ActionNone {
		t.Fatalf("outcome = %+v, want none", outcome)
	}
}

func TestMigrateLegacyRulesOnlyImportsConditionsTheOldEngineApplied(t *testing.T) {
	// URL and media-type conditions only reached the proxy when the rule
	// inspected TLS, and only for a block. Importing them otherwise would
	// invent enforcement that never happened.
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

	// An allow rule kept its patterns but the old proxy never tested them.
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

func TestEvaluateProxyReturnsTheGroupsRuleConditions(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", Action: ActionBlock,
		Domains: []string{"videos.example"},
		Rules: []GroupRule{{
			ID: "r1", Name: "No video", Enabled: true, Action: ActionBlock,
			MITMAction: MITMActionEnable, URLRegexes: []string{"^/watch/"},
			BlockedContentTypes: []string{"video/"},
		}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	identity := svc.ResolveIdentity("192.0.2.10", "")

	match := svc.EvaluateProxy(identity, "videos.example")
	if !match.Matched || !match.ShouldBlock || match.RuleID != "r1" {
		t.Fatalf("match = %+v", match)
	}
	if !match.ShouldMITM || len(match.BlockURLRegexes) != 1 || len(match.BlockContentTypes) != 1 {
		t.Fatalf("match = %+v, want inspection with both conditions", match)
	}

	// Traffic the group does not cover keeps the gateway's own settings.
	if outside := svc.EvaluateProxy(identity, "other.example"); outside.Matched {
		t.Fatalf("uncovered domain matched: %+v", outside)
	}
}

func TestEvaluateProxyTreatsAnAllowRuleAsAnException(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", Action: ActionBlock,
		Domains: []string{"example.com"},
		Rules:   []GroupRule{{ID: "r1", Enabled: true, Action: ActionAllow, Users: []string{"alice"}}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}

	if match := svc.EvaluateProxy(svc.ResolveIdentity("192.0.2.10", "alice"), "example.com"); match.Matched {
		t.Fatalf("allow rule did not exempt the request: %+v", match)
	}
	if match := svc.EvaluateProxy(svc.ResolveIdentity("192.0.2.10", ""), "example.com"); !match.Matched || !match.ShouldBlock {
		t.Fatalf("group block did not apply to another user: %+v", match)
	}
}
