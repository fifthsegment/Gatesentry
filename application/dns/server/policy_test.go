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
		ID: "kids", Name: "Kids", Action: gatesentryPolicy.ActionBlock,
		Domains: []string{"*.games.example"},
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
		ID: "adults", Action: gatesentryPolicy.ActionAllow, Domains: []string{"ads.example"},
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
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"games.example"},
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
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"games.example"},
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
		ID: "kids", Action: gatesentryPolicy.ActionBlock, Domains: []string{"games.example"},
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
	if err := migrateLegacyDevicePolicy(svc, settings); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	if len(snap.Groups) != 1 {
		t.Fatalf("groups after first migration = %d, want 1", len(snap.Groups))
	}
	// Simulate an API edit after migration.
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{ID: "edited", Action: gatesentryPolicy.ActionBlock, Domains: []string{"x.example"}}}); err != nil {
		t.Fatal(err)
	}
	// Re-running migration must not clobber the edit.
	if err := migrateLegacyDevicePolicy(svc, settings); err != nil {
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
