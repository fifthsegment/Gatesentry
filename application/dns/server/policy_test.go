package gatesentryDnsServer

import (
	"testing"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	"github.com/miekg/dns"
	"os"
)

func setupTestPolicyServer(t *testing.T) (*gatesentryPolicy.Service, func()) {
	t.Helper()
	cleanup := setupTestServer(t)
	dir := t.TempDir()
	oldBase := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(oldBase) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	svc, err := gatesentryPolicy.NewService(store, deviceResolver{})
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	origPolicy := policyService
	policyService = svc
	return svc, func() {
		policyService = origPolicy
		cleanup()
	}
}

func TestDNSPolicyGroupBlocksForAssignedDevice(t *testing.T) {
	svc, cleanup := setupTestPolicyPolicyServerForLegacy(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"kids-laptop"},
		IPv4:      "192.0.2.10",
	})
	devices := deviceStore.GetAllDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d", len(devices))
	}
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: devices[0].ID, GroupID: "kids"}}); err != nil {
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
		t.Fatalf("expected NXDOMAIN for group-blocked domain, got %+v", w.msg)
	}
}

func TestDNSPolicyAllowExemptsGlobalBlocklist(t *testing.T) {
	svc, cleanup := setupTestPolicyPolicyServerForLegacy(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{Hostnames: []string{"adult-pc"}, IPv4: "192.0.2.20"})
	devices := deviceStore.GetAllDevices()
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "adults", AllowedDomains: []string{"ads.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: devices[0].ID, GroupID: "adults"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	blockedDomains["ads.example"] = true

	req := new(dns.Msg)
	req.SetQuestion("ads.example.", dns.TypeA)
	w := newMockResponseWriter("192.0.2.20")
	handleDNSRequest(w, req)
	// The allow decision must not block; the mock writer's response is the
	// forwarded upstream answer (empty here is fine — it must not be NXDOMAIN).
	if w.msg != nil && w.msg.Rcode == dns.RcodeNameError {
		t.Fatal("group allow must exempt the global blocklist")
	}
}

func TestDNSPolicyUnknownClientKeepsGlobalBehavior(t *testing.T) {
	svc, cleanup := setupTestPolicyPolicyServerForLegacy(t)
	defer cleanup()

	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "kids", BlockedDomains: []string{"games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	blockedDomains["games.example"] = true

	req := new(dns.Msg)
	req.SetQuestion("games.example.", dns.TypeA)
	w := newMockResponseWriter("198.51.100.99")
	handleDNSRequest(w, req)
	if w.msg == nil || w.msg.Rcode != dns.RcodeNameError {
		t.Fatalf("unknown client must keep the global blocklist decision")
	}
}

func TestDNSPolicyAmbiguousIPDoesNotBorrowGroupPolicy(t *testing.T) {
	svc, cleanup := setupTestPolicyPolicyServerForLegacy(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{Hostnames: []string{"one"}, IPv4: "192.0.2.30"})
	deviceStore.UpsertDevice(&discovery.Device{Hostnames: []string{"two"}, IPv4: "192.0.2.30"})
	devices := deviceStore.GetAllDevices()
	if len(devices) != 2 {
		t.Fatalf("devices = %d", len(devices))
	}
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "kids", BlockedDomains: []string{"games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: devices[0].ID, GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	req := new(dns.Msg)
	req.SetQuestion("games.example.", dns.TypeA)
	w := newMockResponseWriter("192.0.2.30")
	handleDNSRequest(w, req)
	// Ambiguous identity: group policy must not be borrowed. Without a global
	// blocklist entry, this is not blocked.
	if w.msg != nil && w.msg.Rcode == dns.RcodeNameError {
		t.Fatal("ambiguous IP must not borrow an assigned device's group policy")
	}
}

func TestDNSPolicyStaleDeviceStillResolvedForLiveQuery(t *testing.T) {
	svc, cleanup := setupTestPolicyPolicyServerForLegacy(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"old-laptop"},
		IPv4:      "192.0.2.40",
	})
	devices := deviceStore.GetAllDevices()
	if len(devices) != 1 {
		t.Fatalf("devices = %d", len(devices))
	}
	stale := devices[0]
	stale.LastSeen = time.Now().Add(-2 * time.Hour)
	deviceStore.UpsertDevice(&stale)
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "kids", BlockedDomains: []string{"games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: devices[0].ID, GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	req := new(dns.Msg)
	req.SetQuestion("games.example.", dns.TypeA)
	w := newMockResponseWriter("192.0.2.40")
	handleDNSRequest(w, req)
	// A live DNS query from the address confirms the address is active, so
	// a stale LastSeen must not silently disable an explicit assignment.
	if w.msg == nil || w.msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN for stale device's live query, got %+v", w.msg)
	}
}

func TestMigrateLegacyDevicePolicyRunsOnlyOnce(t *testing.T) {
	svc, cleanup := setupTestPolicyPolicyServerForLegacy(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{
		Hostnames: []string{"kids-laptop"},
		IPv4:      "192.0.2.10",
		Owner:     "Dana",
		Category:  "kids",
	})
	settings, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyPolicy(svc, settings); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	if len(snap.Groups) != 2 {
		t.Fatalf("groups after first migration = %d, want the migrated one and the default policy", len(snap.Groups))
	}
	// Simulate an API edit after migration.
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{ID: "edited", BlockedDomains: []string{"x.example"}}}); err != nil {
		t.Fatal(err)
	}
	// Re-running migration must not clobber the edit.
	if err := migrateLegacyPolicy(svc, settings); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap = svc.Snapshot()
	if _, exists := snap.Groups["edited"]; !exists {
		t.Fatalf("repeated migration clobbered API edits: groups = %v", snap.Groups)
	}
}

// setupTestPolicyPolicyServerForLegacy is the legacy name kept for tests that
// predate the service-based setup helper.
func setupTestPolicyPolicyServerForLegacy(t *testing.T) (*gatesentryPolicy.Service, func()) {
	return setupTestPolicyServer(t)
}

// The retired rule store held standalone rules. Those records have to arrive as
// groups that enforce what the old engine enforced, and they must arrive
// unassigned so importing them cannot change any device's filtering on its own.
func TestMigrateLegacyRulesBecomesUnassignedGroups(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()

	settings, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Update("timezone", "Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	legacyRules := `{"rules":[{"id":"block-games","name":"No games","enabled":true,"priority":0,"domain":"games.example","action":"block","mitm_action":"enable","block_type":"url_regex","url_regex_patterns":["/play"],"time_restriction":{"from":"20:00","to":"07:00"}}]}`
	if err := settings.Update("rules", legacyRules); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyPolicy(svc, settings); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	snapshot := svc.Snapshot()
	if len(snapshot.Groups) != 2 {
		t.Fatalf("groups after rule migration = %d, want the imported one and the default policy", len(snapshot.Groups))
	}
	var imported gatesentryPolicy.PolicyGroup
	for _, group := range snapshot.Groups {
		if group.ID != gatesentryPolicy.DefaultGroupID {
			imported = group
		}
	}
	if len(snapshot.Assignments) != 0 {
		t.Fatalf("imported rules were assigned: %+v", snapshot.Assignments)
	}
	if imported.Name != "No games" {
		t.Fatalf("imported group = %+v", imported)
	}
	if len(imported.Rules) != 1 {
		t.Fatalf("imported rules = %d, want 1", len(imported.Rules))
	}
	rule := imported.Rules[0]
	if rule.Action != gatesentryPolicy.ActionBlock || len(rule.Target.Domains) != 1 || rule.Target.Domains[0] != "games.example" {
		t.Fatalf("imported rule = %+v", rule)
	}
	if len(rule.URLRegexes) != 1 || rule.URLRegexes[0] != "/play" {
		t.Fatalf("imported URL conditions = %v", rule.URLRegexes)
	}
	// The old window was evaluated in the gateway's time zone, so the schedule
	// keeps that zone rather than defaulting to UTC.
	if rule.Schedule == nil || rule.Schedule.Timezone != "Europe/Berlin" {
		t.Fatalf("imported schedule = %+v", rule.Schedule)
	}

	// A second run must not duplicate the imported groups.
	if err := migrateLegacyPolicy(svc, settings); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if count := len(svc.Snapshot().Groups); count != 2 {
		t.Fatalf("groups after repeated migration = %d, want the imported one and the default policy", count)
	}
}

func TestDNSGatewayCategoryBlocksSubdomains(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()

	// A gateway-wide category is the default policy for every client, and its
	// feed lists the registrable domain. The subdomain has to be blocked too, or
	// selecting "Social media" would not block real traffic.
	index := gatesentryPolicy.NewCategoryIndex()
	index.Replace("social", []string{"facebook.com"}, time.Now())
	svc.SetCategoryIndex(index)
	if err := svc.SetEnabledCategories([]string{"social"}); err != nil {
		t.Fatal(err)
	}
	if blockedDomains["www.facebook.com"] {
		t.Fatal("test precondition: the global blocklist must not cover the subdomain")
	}

	req := new(dns.Msg)
	req.SetQuestion("www.facebook.com.", dns.TypeA)
	w := newMockResponseWriter("198.51.100.7")
	handleDNSRequest(w, req)
	if w.msg == nil || w.msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN for a gateway-category subdomain, got %+v", w.msg)
	}
	if len(w.msg.Answer) != 1 {
		t.Fatalf("answers = %d, want the blocked.local explanation", len(w.msg.Answer))
	}
	if cname, ok := w.msg.Answer[0].(*dns.CNAME); !ok || cname.Target != "blocked.local." {
		t.Fatalf("block explanation = %+v, want blocked.local CNAME", w.msg.Answer[0])
	}
}

func TestDNSGatewayCategoryIgnoresUnselectedCategories(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()

	index := gatesentryPolicy.NewCategoryIndex()
	index.Replace("social", []string{"facebook.com"}, time.Now())
	svc.SetCategoryIndex(index)

	req := new(dns.Msg)
	req.SetQuestion("www.facebook.com.", dns.TypeA)
	w := newMockResponseWriter("198.51.100.8")
	handleDNSRequest(w, req)
	if w.msg != nil && w.msg.Rcode == dns.RcodeNameError {
		t.Fatal("an unselected category must not block anything")
	}
}

func TestDNSPolicyGroupAllowExemptsGatewayCategory(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()

	deviceStore.UpsertDevice(&discovery.Device{Hostnames: []string{"adults-pc"}, IPv4: "192.0.2.60"})
	devices := deviceStore.GetAllDevices()
	index := gatesentryPolicy.NewCategoryIndex()
	index.Replace("social", []string{"facebook.com"}, time.Now())
	svc.SetCategoryIndex(index)
	if err := svc.SetEnabledCategories([]string{"social"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID: "adults", Name: "Adults", AllowedDomains: []string{"facebook.com"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]gatesentryPolicy.DeviceAssignment{{DeviceID: devices[0].ID, GroupID: "adults"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	req := new(dns.Msg)
	req.SetQuestion("www.facebook.com.", dns.TypeA)
	w := newMockResponseWriter("192.0.2.60")
	handleDNSRequest(w, req)
	// The assigned group allows the category, so the gateway-wide default must
	// not override the explicit assignment.
	if w.msg != nil && w.msg.Rcode == dns.RcodeNameError {
		t.Fatal("group allow must exempt a gateway-wide category")
	}
}

// startFakeUpstream serves A answers for the safe-search endpoints so the
// rewrite can be tested without the network.
func startFakeUpstream(t *testing.T) {
	t.Helper()
	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		if r.Question[0].Name == "restrict.youtube.com." {
			rr, _ := dns.NewRR("restrict.youtube.com. 300 IN A 216.239.38.120")
			m.Answer = append(m.Answer, rr)
		}
		w.WriteMsg(m)
	})
	server := &dns.Server{Addr: "127.0.0.1:0", Net: "udp", Handler: mux}
	ready := make(chan struct{})
	server.NotifyStartedFunc = func() { close(ready) }
	go server.ListenAndServe()
	<-ready
	previous := externalResolver
	externalResolver = server.PacketConn.LocalAddr().String()
	t.Cleanup(func() {
		externalResolver = previous
		server.Shutdown()
	})
}

func TestDNSDefaultPolicyBlocksUnassignedClients(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()
	if err := svc.UpdateGroup(gatesentryPolicy.DefaultGroupID, gatesentryPolicy.PolicyGroup{
		Name: "Default", BlockedDomains: []string{"tiktok.com"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	req := new(dns.Msg)
	req.SetQuestion("www.tiktok.com.", dns.TypeA)
	w := newMockResponseWriter("198.51.100.44")
	handleDNSRequest(w, req)
	if w.msg == nil || w.msg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected the default policy to block an unknown client, got %+v", w.msg)
	}
	if ttl := w.msg.Answer[0].Header().Ttl; ttl > 60 {
		t.Fatalf("block answer TTL = %d, want a short TTL so schedules apply promptly", ttl)
	}
}

func TestDNSSafeSearchRewritesYouTube(t *testing.T) {
	svc, cleanup := setupTestPolicyServer(t)
	defer cleanup()
	startFakeUpstream(t)
	if err := svc.UpdateGroup(gatesentryPolicy.DefaultGroupID, gatesentryPolicy.PolicyGroup{
		Name: "Default", SafeSearch: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	req := new(dns.Msg)
	req.SetQuestion("www.youtube.com.", dns.TypeA)
	w := newMockResponseWriter("198.51.100.44")
	handleDNSRequest(w, req)
	if w.msg == nil || len(w.msg.Answer) < 2 {
		t.Fatalf("answer = %+v, want a CNAME and the target's address", w.msg)
	}
	cname, ok := w.msg.Answer[0].(*dns.CNAME)
	if !ok || cname.Target != "restrict.youtube.com." {
		t.Fatalf("first answer = %v, want a CNAME to restrict.youtube.com", w.msg.Answer[0])
	}
	if a, ok := w.msg.Answer[1].(*dns.A); !ok || a.A.String() != "216.239.38.120" {
		t.Fatalf("second answer = %v, want the restricted endpoint's address", w.msg.Answer[1])
	}
}
