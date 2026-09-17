package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

type mapResolver struct {
	devices map[string]string // ip -> device id
	shared  map[string]bool   // ips claimed by multiple devices
	stales  map[string]bool   // ips whose observation is stale
}

func (m *mapResolver) ResolveDeviceByIP(ip string) (string, bool, bool) {
	return m.devices[ip], m.shared[ip], m.stales[ip]
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(store, &mapResolver{})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestMissingPolicyKeyPreservesBaseline(t *testing.T) {
	svc := newTestService(t)
	identity := svc.ResolveIdentity("192.0.2.10", "")
	if identity.Source != SourceUnknown {
		t.Fatalf("identity source = %s, want unknown", identity.Source)
	}
	decision := svc.EvaluateDNS(identity, "example.com")
	if decision.Action != ActionNone {
		t.Fatalf("decision action = %s, want none", decision.Action)
	}
}

func TestDeviceAssignmentBlocksDNSForGroup(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", Action: ActionBlock,
		Domains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	svc.devices = resolver
	identity := svc.ResolveIdentity("192.0.2.10", "")
	if identity.Source != SourceDevice || identity.DeviceID != "device-1" || identity.GroupID != "kids" {
		t.Fatalf("identity = %+v", identity)
	}
	decision := svc.EvaluateDNS(identity, "chess.games.example")
	if decision.Action != ActionBlock || decision.GroupID != "kids" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.MatchedDomain != "*.games.example" {
		t.Fatalf("matched domain = %s", decision.MatchedDomain)
	}
	if len(decision.InapplicableConditions) != 3 ||
		decision.InapplicableConditions[0] != "url_regex" ||
		decision.InapplicableConditions[1] != "content_type" ||
		decision.InapplicableConditions[2] != "mitm" {
		t.Fatalf("inapplicable conditions = %v", decision.InapplicableConditions)
	}
}

func TestNATAmbiguityFallsBackToUnknown(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{
		devices: map[string]string{"192.0.2.10": "device-1"},
		shared:  map[string]bool{"192.0.2.10": true},
	}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Action: ActionBlock, Domains: []string{"example.com"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := svc.ResolveIdentity("192.0.2.10", "")
	if identity.Source != SourceUnknownNAT || identity.GroupID != "" {
		t.Fatalf("identity = %+v, want unknown_nat with no group", identity)
	}
	decision := svc.EvaluateDNS(identity, "example.com")
	if decision.Action != ActionNone {
		t.Fatalf("decision = %+v, want none: NAT ambiguity must not borrow another device's policy", decision)
	}
}

func TestAuthenticatedUserKeptSeparateFromDevice(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "work", Name: "Work", Action: ActionBlock, Domains: []string{"social.example"}, Users: []string{"dana"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "other"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := svc.ResolveIdentity("192.0.2.10", "dana")
	if identity.Source != SourceAuthUser || identity.DeviceID != "" || identity.GroupID != "work" {
		t.Fatalf("identity = %+v", identity)
	}
	decision := svc.EvaluateDNS(identity, "social.example")
	if decision.Action != ActionBlock {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestMigrationFromOwnerCategory(t *testing.T) {
	groups, assignments, err := MigrateLegacyDevices(map[string]LegacyDevice{
		"device-1": {ID: "device-1", Owner: "Dana", Category: "kids"},
		"device-2": {ID: "device-2", Owner: "", Category: "iot"},
		"device-3": {ID: "device-3", Owner: "Dana", Category: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3 (kids, iot, owner-Dana)", len(groups))
	}
	if len(assignments) != 3 {
		t.Fatalf("assignments = %d, want 3", len(assignments))
	}
	byGroup := map[string]string{}
	for _, a := range assignments {
		byGroup[a.DeviceID] = a.GroupID
	}
	if byGroup["device-1"] != "category-kids" {
		t.Fatalf("device-1 group = %s, want category-kids", byGroup["device-1"])
	}
	if byGroup["device-2"] != "category-iot" {
		t.Fatalf("device-2 group = %s, want category-iot", byGroup["device-2"])
	}
	if !strings.HasPrefix(byGroup["device-3"], "owner-") {
		t.Fatalf("device-3 group = %s, want owner-*", byGroup["device-3"])
	}
}

func TestConcurrentPolicyUpdatesAndReads(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Action: ActionBlock, Domains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			group := PolicyGroup{ID: "kids", Action: ActionBlock, Domains: []string{fmt.Sprintf("games-%d.example", i)}}
			if err := svc.SaveGroups([]PolicyGroup{group}); err != nil {
				t.Errorf("save groups: %v", err)
			}
		}(i)
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identity := svc.ResolveIdentity("192.0.2.10", "")
			_ = svc.EvaluateDNS(identity, "games.example")
		}()
	}
	wg.Wait()
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	if len(snap.Groups) != 1 {
		t.Fatalf("groups after concurrent writes = %d, want 1", len(snap.Groups))
	}
}

func TestStorageReadErrorRetainsSnapshot(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Action: ActionBlock, Domains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	before := svc.Snapshot()
	// Corrupt the persisted value; a read error must not clear live policy.
	if err := svc.storage.Update(StorageKey, "{not json"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err == nil {
		t.Fatal("expected reload error")
	}
	after := svc.Snapshot()
	if len(after.Groups) != len(before.Groups) {
		t.Fatalf("snapshot groups = %d, want %d retained", len(after.Groups), len(before.Groups))
	}
}

func TestUnknownGroupIDIsIgnored(t *testing.T) {
	svc := newTestService(t)
	identity := Identity{GroupID: "missing", Source: SourceDevice, DeviceID: "device-1"}
	decision := svc.EvaluateDNS(identity, "anything.example")
	if decision.Action != ActionNone || decision.GroupID != "" {
		t.Fatalf("decision = %+v, want none with no group", decision)
	}
}

func TestAllowGroupOverridesBlocklistForDomain(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "adults", Action: ActionAllow, Domains: []string{"tracker.example"}, Users: []string{"dana"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := svc.ResolveIdentity("", "dana")
	decision := svc.EvaluateDNS(identity, "tracker.example")
	if decision.Action != ActionAllow {
		t.Fatalf("decision = %+v, want allow", decision)
	}
}

func TestIPAddressUserFallbackIsNotAuthenticatedIdentity(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Action: ActionBlock, Domains: []string{"games.example"}},
		{ID: "user-group", Action: ActionBlock, Domains: []string{"anything.example"}, Users: []string{"192.0.2.10"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	// The unauthenticated explicit proxy passes the client address as the
	// user fallback. It must resolve to the device identity, never to a
	// user-scoped group keyed by that address string.
	identity := svc.ResolveIdentity("192.0.2.10", "192.0.2.10")
	if identity.Source != SourceDevice || identity.DeviceID != "device-1" || identity.GroupID != "kids" {
		t.Fatalf("identity = %+v, want device-1/kids", identity)
	}
	// host:port form must also be rejected as an identity
	identity = svc.ResolveIdentity("192.0.2.10", "192.0.2.10:45123")
	if identity.Source != SourceDevice {
		t.Fatalf("host:port identity = %+v, want device", identity)
	}
}

func TestStaleDeviceObservationFallsBackToDefault(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{
		devices: map[string]string{"192.0.2.10": "device-1"},
		stales:  map[string]bool{"192.0.2.10": true},
	}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Action: ActionBlock, Domains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := svc.ResolveIdentity("192.0.2.10", "")
	if identity.Source != SourceStaleDevice || identity.GroupID != "" {
		t.Fatalf("identity = %+v, want stale_device with no group", identity)
	}
	decision := svc.EvaluateDNS(identity, "games.example")
	if decision.Action != ActionNone {
		t.Fatalf("decision = %+v, want none: stale observations must not enforce policy", decision)
	}
}

func TestStaleDeviceObservationStillResolvedForLiveDNSQuery(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{
		devices: map[string]string{"192.0.2.10": "device-1"},
		stales:  map[string]bool{"192.0.2.10": true},
	}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Action: ActionBlock, Domains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	// A live DNS query from the address is direct evidence the address is
	// active, so stale LastSeen alone must not downgrade DNS identity.
	identity := svc.ResolveIdentityForDNS("192.0.2.10", "")
	if identity.Source != SourceDevice || identity.GroupID != "kids" {
		identityBytes, _ := json.Marshal(identity)
		t.Fatalf("identity = %s, want device-1/kids", identityBytes)
	}
}

func TestSaveMigrationIsOneShot(t *testing.T) {
	svc := newTestService(t)
	groups := []PolicyGroup{{ID: "category-kids", Name: "kids", Action: ActionNone}}
	assignments := []DeviceAssignment{{DeviceID: "device-1", GroupID: "category-kids"}}
	if err := svc.SaveMigration(groups, assignments, "legacy_device_metadata"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	if len(snap.Groups) != 1 || len(snap.Assignments) != 1 {
		t.Fatalf("snapshot = %+v, want one group and one assignment", snap)
	}
	// An API edit after migration must survive a repeated migration call.
	if err := svc.SaveGroups([]PolicyGroup{{ID: "edited", Action: ActionBlock, Domains: []string{"x.example"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveMigration(groups, assignments, "legacy_device_metadata"); err != nil {
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

func TestSnapshotIsImmutableView(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Action: ActionBlock, Domains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	snap.Groups["injected"] = PolicyGroup{ID: "injected"}
	fresh := svc.Snapshot()
	if _, exists := fresh.Groups["injected"]; exists {
		t.Fatal("snapshot mutation leaked into service state")
	}
}

func TestDocumentRoundTripAndVersion(t *testing.T) {
	svc := newTestService(t)
	now := time.Now().UTC()
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Name: "Kids", Action: ActionBlock, Domains: []string{"games.example"}, CreatedAt: now, UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	raw, err := svc.storage.GetE(StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "\"version\":1") {
		t.Fatalf("document = %s", raw)
	}
}
