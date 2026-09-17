package gatesentryDnsServer

import (
	"testing"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	"github.com/miekg/dns"
)

// pollDecisionEntry waits briefly for the async LogDecision goroutine to
// persist a decision record, then returns the matching LogEntry.
func pollDecisionEntry(t *testing.T, url string) gatesentryLogger.LogEntry {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		entries, err := logger.GetDeviceActivity([]string{"192.0.2.10", "192.0.2.20"}, 60, 50)
		if err != nil {
			t.Fatalf("read device activity: %v", err)
		}
		for _, e := range entries {
			if e.URL == url {
				return e
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("decision record for %s never appeared", url)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDNSDecisionProvenanceBlock(t *testing.T) {
	svc, cleanup := setupTestPolicyPolicyServerForLegacy(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"kids-laptop"},
		IPv4:      "192.0.2.10",
	})
	devices := deviceStore.GetAllDevices()
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "kids", Name: "Kids", Action: gatesentryPolicy.ActionBlock,
		Domains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{
		{DeviceID: devices[0].ID, GroupID: "kids"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	req := new(dns.Msg)
	req.SetQuestion("chess.games.example.", dns.TypeA)
	w := newMockResponseWriter("192.0.2.10")
	handleDNSRequest(w, req)

	if w.msg == nil || w.msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN, got %+v", w.msg)
	}

	entry := pollDecisionEntry(t, "chess.games.example")
	if entry.Action != string(gatesentryPolicy.ActionDecisionBlock) {
		t.Fatalf("action = %s, want %s", entry.Action, gatesentryPolicy.ActionDecisionBlock)
	}
	if entry.Layer != string(gatesentryPolicy.LayerDNS) {
		t.Fatalf("layer = %s, want %s", entry.Layer, gatesentryPolicy.LayerDNS)
	}
	if entry.DNSResponseType != "blocked" {
		t.Fatalf("dns response type = %s, want blocked", entry.DNSResponseType)
	}
	if entry.GroupID != "kids" {
		t.Fatalf("group id = %s, want kids", entry.GroupID)
	}
	if entry.MatchedRule == "" {
		t.Fatal("matched rule is empty, want policy group reference")
	}
	if entry.PolicyRevision == 0 {
		t.Fatal("policy revision = 0, want non-zero")
	}
}

func TestDNSDecisionProvenanceForward(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	req := new(dns.Msg)
	req.SetQuestion("clean.forward.example.", dns.TypeA)
	w := newMockResponseWriter("192.0.2.10")
	handleDNSRequest(w, req)

	entry := pollDecisionEntry(t, "clean.forward.example")
	if entry.Action != string(gatesentryPolicy.ActionDecisionAllow) {
		t.Fatalf("action = %s, want %s", entry.Action, gatesentryPolicy.ActionDecisionAllow)
	}
	if entry.Layer != string(gatesentryPolicy.LayerDNS) {
		t.Fatalf("layer = %s, want %s", entry.Layer, gatesentryPolicy.LayerDNS)
	}
	if entry.DNSResponseType != "forward" {
		t.Fatalf("dns response type = %s, want forward", entry.DNSResponseType)
	}
	if entry.GroupID != "" {
		t.Fatalf("group id = %s, want empty for unknown identity", entry.GroupID)
	}
}

func TestDNSDecisionProvenanceGlobalBlocklist(t *testing.T) {
	cleanup := setupTestServer(t)
	defer cleanup()

	AddBlockedDomainForTest("blocked.example")

	req := new(dns.Msg)
	req.SetQuestion("blocked.example.", dns.TypeA)
	w := newMockResponseWriter("192.0.2.10")
	handleDNSRequest(w, req)

	if w.msg == nil || w.msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN, got %+v", w.msg)
	}

	entry := pollDecisionEntry(t, "blocked.example")
	if entry.Action != string(gatesentryPolicy.ActionDecisionBlock) {
		t.Fatalf("action = %s, want %s", entry.Action, gatesentryPolicy.ActionDecisionBlock)
	}
	if entry.DNSResponseType != "blocked" {
		t.Fatalf("dns response type = %s, want blocked", entry.DNSResponseType)
	}
	if entry.MatchedRule != "blocklist" {
		t.Fatalf("matched rule = %s, want blocklist", entry.MatchedRule)
	}
	if entry.Reason != "global blocklist" {
		t.Fatalf("reason = %s, want global blocklist", entry.Reason)
	}
}
