package gatesentry2logger

import (
	"testing"
	"time"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
)

// newTestLogDecision seeds a logger with decisions covering the filter
// dimensions PER-37 surfaces (action, layer, reason, domain, group, IP).
func newTestLogDecision(t *testing.T) *Log {
	t.Helper()
	l := newActivityTestLogger(t)
	now := time.Now().Unix()
	seed := func(action gatesentryPolicy.DecisionAction, layer gatesentryPolicy.DecisionLayer, domain, ip, group, rule, reason string, ts int64) {
		d := gatesentryPolicy.NewDecision(action, layer, domain)
		d.URL = domain
		d.ClientIP = ip
		d.GroupID = group
		d.MatchedRule = rule
		d.Reason = reason
		d.Timestamp = time.Unix(ts, 0)
		l.LogDecision(d)
	}
	// Two blocks for ads.example from the same IP in the kids group.
	seed(gatesentryPolicy.ActionDecisionBlock, gatesentryPolicy.LayerDNS, "ads.example", "192.0.2.10", "kids", "blocklist", "global blocklist", now-10)
	seed(gatesentryPolicy.ActionDecisionBlock, gatesentryPolicy.LayerDNS, "ads.example", "192.0.2.10", "kids", "blocklist", "global blocklist", now-5)
	// A proxy URL block from a different IP.
	seed(gatesentryPolicy.ActionDecisionBlock, gatesentryPolicy.LayerExplicitProxy, "tracker.example", "192.0.2.20", "guests", "url filter", "blocked URL", now-8)
	// An allow and a bypass so the summary has mixed actions.
	seed(gatesentryPolicy.ActionDecisionAllow, gatesentryPolicy.LayerDNS, "safe.example", "192.0.2.10", "kids", "", "no matching rule", now-3)
	seed(gatesentryPolicy.ActionDecisionBypass, gatesentryPolicy.LayerDNS, "exc.example", "192.0.2.10", "kids", "exception", "exception domain", now-2)
	// An inspection failure (error) to exercise InspectionFailures.
	seed(gatesentryPolicy.ActionDecisionError, gatesentryPolicy.LayerContent, "broken.example", "192.0.2.20", "", "filter", "filter error", now-1)
	return l
}

func TestQueryDecisionsFiltersByAction(t *testing.T) {
	l := newTestLogDecision(t)
	// LogDecision writes asynchronously; wait briefly for records.
	time.Sleep(150 * time.Millisecond)
	entries, err := l.QueryDecisions(DecisionFilter{Action: string(gatesentryPolicy.ActionDecisionBlock)})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d block entries, want 3", len(entries))
	}
	for _, e := range entries {
		if e.Action != string(gatesentryPolicy.ActionDecisionBlock) {
			t.Fatalf("non-block entry returned: %+v", e)
		}
	}
}

func TestQueryDecisionsFiltersByLayerAndGroup(t *testing.T) {
	l := newTestLogDecision(t)
	time.Sleep(150 * time.Millisecond)
	entries, err := l.QueryDecisions(DecisionFilter{Layer: string(gatesentryPolicy.LayerDNS), GroupID: "kids"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	// kids DNS decisions: 2 blocks (ads.example), 1 allow, 1 bypass = 4.
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
}

func TestQueryDecisionsFiltersByDomainAndReason(t *testing.T) {
	l := newTestLogDecision(t)
	time.Sleep(150 * time.Millisecond)
	entries, err := l.QueryDecisions(DecisionFilter{Domain: "ads.example", Reason: "blocklist"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 for ads.example+blocklist", len(entries))
	}
}

func TestQueryDecisionsRespectsLimitAndOffset(t *testing.T) {
	l := newTestLogDecision(t)
	time.Sleep(150 * time.Millisecond)
	all, err := l.QueryDecisions(DecisionFilter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(all) != 6 {
		t.Fatalf("seeded %d entries, want 6", len(all))
	}
	// Offset skips the newest entry; limit caps at one older entry.
	page, err := l.QueryDecisions(DecisionFilter{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("got %d entries, want 1", len(page))
	}
	// The page must differ from the newest (offset 1 skips it).
	if page[0].Time == all[0].Time && page[0].URL == all[0].URL {
		t.Fatalf("offset did not skip the newest entry")
	}
}

func TestQueryDecisionsEnforcesMaxLimit(t *testing.T) {
	l := newTestLogDecision(t)
	time.Sleep(150 * time.Millisecond)
	page, err := l.QueryDecisions(DecisionFilter{Limit: 99999})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(page) > maxDecisionLimit {
		t.Fatalf("returned %d entries, exceeds max %d", len(page), maxDecisionLimit)
	}
}

func TestDecisionSummaryAggregatesCounts(t *testing.T) {
	l := newTestLogDecision(t)
	time.Sleep(150 * time.Millisecond)
	s, err := l.DecisionSummary(DecisionFilter{})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.Total != 6 {
		t.Fatalf("total = %d, want 6", s.Total)
	}
	if s.ByAction[string(gatesentryPolicy.ActionDecisionBlock)] != 3 {
		t.Fatalf("block count = %d, want 3", s.ByAction[string(gatesentryPolicy.ActionDecisionBlock)])
	}
	if s.ByAction[string(gatesentryPolicy.ActionDecisionError)] != 1 {
		t.Fatalf("error count = %d, want 1", s.ByAction[string(gatesentryPolicy.ActionDecisionError)])
	}
	if s.InspectionFailures != 1 {
		t.Fatalf("inspection failures = %d, want 1", s.InspectionFailures)
	}
	if len(s.TopBlockedDomains) == 0 || s.TopBlockedDomains[0].Key != "ads.example" {
		t.Fatalf("top blocked domain = %+v, want ads.example first", s.TopBlockedDomains)
	}
	if s.TopBlockedDomains[0].Count != 2 {
		t.Fatalf("ads.example count = %d, want 2", s.TopBlockedDomains[0].Count)
	}
}

func TestDecisionSummaryAffectedAddresses(t *testing.T) {
	l := newTestLogDecision(t)
	time.Sleep(150 * time.Millisecond)
	s, err := l.DecisionSummary(DecisionFilter{})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	// 192.0.2.10 has 4 decisions, 192.0.2.20 has 2.
	if len(s.AffectedAddresses) < 2 {
		t.Fatalf("affected addresses = %+v, want at least 2", s.AffectedAddresses)
	}
	if s.AffectedAddresses[0].Key != "192.0.2.10" || s.AffectedAddresses[0].Count != 4 {
		t.Fatalf("top affected address = %+v, want 192.0.2.10 (4)", s.AffectedAddresses[0])
	}
}

func TestQueryDecisionsNilLogger(t *testing.T) {
	var l *Log
	entries, err := l.QueryDecisions(DecisionFilter{})
	if err != nil {
		t.Fatalf("nil logger query: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("nil logger returned %d entries, want 0", len(entries))
	}
}

func TestDecisionSummaryNilLogger(t *testing.T) {
	var l *Log
	s, err := l.DecisionSummary(DecisionFilter{})
	if err != nil {
		t.Fatalf("nil logger summary: %v", err)
	}
	if s.Total != 0 {
		t.Fatalf("nil logger summary total = %d, want 0", s.Total)
	}
}

func TestDecisionSummaryReportsReachAndTimeline(t *testing.T) {
	l := newTestLogDecision(t)
	time.Sleep(150 * time.Millisecond)
	now := time.Now().Unix()
	s, err := l.DecisionSummary(DecisionFilter{From: now - 86400})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if s.UniqueClients != 2 || s.UniqueDomains != 5 {
		t.Fatalf("clients=%d domains=%d, want 2 and 5", s.UniqueClients, s.UniqueDomains)
	}
	if len(s.TopDomains) == 0 || s.TopDomains[0].Key != "ads.example" {
		t.Fatalf("top domains = %+v, want ads.example first", s.TopDomains)
	}
	if len(s.BlocksByGroup) != 2 || s.BlocksByGroup[0].Key != "kids" || s.BlocksByGroup[0].Count != 2 {
		t.Fatalf("blocks by group = %+v, want kids=2 first", s.BlocksByGroup)
	}
	total, blocked := 0, 0
	for _, b := range s.Timeline {
		total += b.Total
		blocked += b.Blocked
	}
	if len(s.Timeline) < 24 || total != 6 || blocked != 3 {
		t.Fatalf("timeline buckets=%d total=%d blocked=%d, want >=24, 6, 3", len(s.Timeline), total, blocked)
	}
}

func TestDecisionHostStripsSchemeAndPath(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.Example.com:443/a?b": "www.example.com",
		"http://site.example/path":        "site.example",
		"dns.example.":                    "dns.example",
	} {
		if got := decisionHost(in); got != want {
			t.Fatalf("decisionHost(%q) = %q, want %q", in, got, want)
		}
	}
}
