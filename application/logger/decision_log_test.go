package gatesentry2logger

import (
	"encoding/json"
	"testing"
	"time"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
)

// pollLogDecision waits briefly for the async LogDecision goroutine to
// persist a decision record, then returns the matching LogEntry.
func pollLogDecision(t *testing.T, l *Log, url string) LogEntry {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		entries, err := l.GetDeviceActivity([]string{"192.0.2.50"}, 60, 50)
		if err != nil {
			t.Fatalf("read activity: %v", err)
		}
		for _, e := range entries {
			if e.URL == url {
				return e
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("LogDecision record for %s never appeared", url)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLogDecisionStoresFullProvenance(t *testing.T) {
	l := newActivityTestLogger(t)
	d := gatesentryPolicy.NewDecision(gatesentryPolicy.ActionDecisionBlock,
		gatesentryPolicy.LayerDNS, "blocked.example").
		WithIdentity(gatesentryPolicy.Identity{
			DeviceID: "dev-1",
			GroupID:  "kids",
			Source:   gatesentryPolicy.SourceDevice,
		}).WithPolicyRevision(7)
	d.ClientIP = "192.0.2.50"
	d.ResponseType = "blocked"
	d.MatchedRule = "blocklist"
	d.Reason = "global blocklist"

	l.LogDecision(d)
	entry := pollLogDecision(t, l, "blocked.example")

	if entry.Type != "dns" {
		t.Fatalf("type = %s, want dns", entry.Type)
	}
	if entry.DNSResponseType != "blocked" {
		t.Fatalf("dns response type = %s, want blocked", entry.DNSResponseType)
	}
	if entry.Action != string(gatesentryPolicy.ActionDecisionBlock) {
		t.Fatalf("action = %s, want %s", entry.Action, gatesentryPolicy.ActionDecisionBlock)
	}
	if entry.Layer != string(gatesentryPolicy.LayerDNS) {
		t.Fatalf("layer = %s, want %s", entry.Layer, gatesentryPolicy.LayerDNS)
	}
	if entry.MatchedRule != "blocklist" {
		t.Fatalf("matched rule = %s, want blocklist", entry.MatchedRule)
	}
	if entry.Reason != "global blocklist" {
		t.Fatalf("reason = %s, want global blocklist", entry.Reason)
	}
	if entry.GroupID != "kids" {
		t.Fatalf("group id = %s, want kids", entry.GroupID)
	}
	if entry.DeviceID != "dev-1" {
		t.Fatalf("device id = %s, want dev-1", entry.DeviceID)
	}
	if entry.Source != string(gatesentryPolicy.SourceDevice) {
		t.Fatalf("source = %s, want %s", entry.Source, gatesentryPolicy.SourceDevice)
	}
	if entry.PolicyRevision != 7 {
		t.Fatalf("policy revision = %d, want 7", entry.PolicyRevision)
	}
	if entry.IP != "192.0.2.50" {
		t.Fatalf("ip = %s, want 192.0.2.50", entry.IP)
	}
}

func TestLogDecisionProxyLayerWritesProxyType(t *testing.T) {
	l := newActivityTestLogger(t)
	d := gatesentryPolicy.NewDecision(gatesentryPolicy.ActionDecisionBlock,
		gatesentryPolicy.LayerExplicitProxy, "ads.example").
		WithPolicyRevision(1)
	d.ClientIP = "192.0.2.50"
	d.URL = "http://ads.example/banner"
	d.ResponseType = "blocked_url"

	l.LogDecision(d)
	entry := pollLogDecision(t, l, "http://ads.example/banner")

	if entry.Type != "proxy" {
		t.Fatalf("type = %s, want proxy", entry.Type)
	}
	if entry.ProxyResponseType != "blocked_url" {
		t.Fatalf("proxy response type = %s, want blocked_url", entry.ProxyResponseType)
	}
	if entry.DNSResponseType != "" {
		t.Fatalf("dns response type = %s, want empty for proxy layer", entry.DNSResponseType)
	}
}

func TestLogDecisionNilGuard(t *testing.T) {
	// A nil receiver must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LogDecision on nil receiver panicked: %v", r)
		}
	}()
	var l *Log
	l.LogDecision(gatesentryPolicy.Decision{})
}

func TestLegacyLogDNSStillParses(t *testing.T) {
	l := newActivityTestLogger(t)
	l.LogDNS("forward.example", "192.0.2.50", "forward")
	entry := pollLogDecision(t, l, "forward.example")

	if entry.Type != "dns" {
		t.Fatalf("type = %s, want dns", entry.Type)
	}
	if entry.DNSResponseType != "forward" {
		t.Fatalf("dns response type = %s, want forward", entry.DNSResponseType)
	}
	// Legacy entries have no structured provenance.
	if entry.Action != "" || entry.Layer != "" || entry.MatchedRule != "" {
		t.Fatalf("legacy entry has structured fields: %+v", entry)
	}
}

func TestLogDecisionStoresValidJSON(t *testing.T) {
	l := newActivityTestLogger(t)
	d := gatesentryPolicy.NewDecision(gatesentryPolicy.ActionDecisionAllow,
		gatesentryPolicy.LayerTransparentProxy, "cdn.example").
		WithIdentity(gatesentryPolicy.Identity{
			GroupID: "guests",
			Source:  gatesentryPolicy.SourceUnknown,
		}).WithPolicyRevision(3)
	d.ClientIP = "192.0.2.50"
	d.URL = "https://cdn.example/resource"
	d.ResponseType = "ssldirect"

	l.LogDecision(d)
	entry := pollLogDecision(t, l, "https://cdn.example/resource")

	// Re-marshal the entry to confirm it round-trips cleanly.
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	var reparsed LogEntry
	if err := json.Unmarshal(raw, &reparsed); err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if reparsed.Action != string(gatesentryPolicy.ActionDecisionAllow) {
		t.Fatalf("reparsed action = %s, want %s", reparsed.Action, gatesentryPolicy.ActionDecisionAllow)
	}
}
