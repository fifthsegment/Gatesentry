package policy

import (
	"encoding/json"
	"errors"
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

func newTestService(t testing.TB) *Service {
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
		ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"},
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
	if len(decision.ProxyOnly) != 0 {
		t.Fatalf("proxy-only conditions = %v, want none for a plain domain block", decision.ProxyOnly)
	}
}

func TestNATAmbiguityFallsBackToUnknown(t *testing.T) {
	svc := newTestService(t)
	resolver := &mapResolver{
		devices: map[string]string{"192.0.2.10": "device-1"},
		shared:  map[string]bool{"192.0.2.10": true},
	}
	svc.devices = resolver
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"example.com"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := svc.ResolveIdentity("192.0.2.10", "")
	if identity.Source != SourceUnknownNAT || identity.GroupID != DefaultGroupID {
		t.Fatalf("identity = %+v, want unknown_nat on the default policy", identity)
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
		ID: "work", Name: "Work", BlockedDomains: []string{"social.example"}, Users: []string{"dana"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "other"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	// The login decides the policy; the device is still recorded so logs
	// and the device page can attribute the request.
	identity := svc.ResolveIdentity("192.0.2.10", "dana")
	if identity.Source != SourceAuthUser || identity.DeviceID != "device-1" || identity.GroupID != "work" {
		t.Fatalf("identity = %+v", identity)
	}
	if decision := svc.EvaluateDNS(identity, "social.example"); decision.Action != ActionBlock {
		t.Fatalf("decision = %+v", decision)
	}
	// A login no policy names falls back to the device's own assignment.
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "work", Name: "Work", Users: []string{"dana"}},
		{ID: "kids", Name: "Kids"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := svc.ResolveIdentity("192.0.2.10", "erin").GroupID; got != "kids" {
		t.Fatalf("unlisted user on an assigned device = %q, want the device's policy", got)
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
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"games.example"}}}); err != nil {
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
			group := PolicyGroup{ID: "kids", BlockedDomains: []string{fmt.Sprintf("games-%d.example", i)}}
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
	if len(snap.Groups) != 2 {
		t.Fatalf("groups after concurrent writes = %d, want kids and the default policy", len(snap.Groups))
	}
}

func TestStorageReadErrorRetainsSnapshot(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"games.example"}}}); err != nil {
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

func TestUnknownGroupIDFallsBackToTheDefaultPolicy(t *testing.T) {
	svc := newTestService(t)
	identity := Identity{GroupID: "missing", Source: SourceDevice, DeviceID: "device-1"}
	decision := svc.EvaluateDNS(identity, "anything.example")
	if decision.Action != ActionNone || decision.GroupID != DefaultGroupID {
		t.Fatalf("decision = %+v, want none from the default policy", decision)
	}
}

func TestDefaultPolicyAppliesToUnassignedDevices(t *testing.T) {
	// The bug this fixes: an unassigned, unknown, or NATed client had no policy
	// at all, so the only way to filter it was the global blocklist.
	svc := newTestService(t)
	if err := svc.UpdateGroup(DefaultGroupID, PolicyGroup{Name: "Default", BlockedDomains: []string{"tiktok.com"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := svc.ResolveIdentityForDNS("198.51.100.7", "")
	if identity.GroupID != DefaultGroupID {
		t.Fatalf("identity = %+v, want the default policy", identity)
	}
	if got := svc.EvaluateDNS(identity, "www.tiktok.com").Action; got != ActionBlock {
		t.Fatalf("action = %s, want block from the default policy", got)
	}
}

func TestDefaultPolicyCannotBeDeletedOrGivenUsers(t *testing.T) {
	svc := newTestService(t)
	if err := svc.DeleteGroup(DefaultGroupID); !errors.Is(err, ErrDefaultGroup) {
		t.Fatalf("delete default = %v, want ErrDefaultGroup", err)
	}
	if err := svc.UpdateGroup(DefaultGroupID, PolicyGroup{Name: "Default", Users: []string{"dana"}}); err == nil {
		t.Fatal("expected users on the default policy to be rejected")
	}
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Name: "Kids"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Snapshot().Groups[DefaultGroupID]; !ok {
		t.Fatal("replacing the group set dropped the default policy")
	}
}

func TestUserOnTwoPoliciesIsRejected(t *testing.T) {
	svc := newTestService(t)
	if err := svc.CreateGroup(PolicyGroup{ID: "a", Name: "A", Users: []string{"dana"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateGroup(PolicyGroup{ID: "b", Name: "B", Users: []string{"dana"}}); !errors.Is(err, ErrUserConflict) {
		t.Fatalf("second listing = %v, want ErrUserConflict", err)
	}
}

func TestAssigningTheDefaultPolicyClearsTheAssignment(t *testing.T) {
	svc := newTestService(t)
	if err := svc.CreateGroup(PolicyGroup{ID: "kids", Name: "Kids"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", DefaultGroupID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, ok := svc.Snapshot().Assignments["device-1"]; ok {
		t.Fatal("an assignment to the default policy was stored")
	}
}

func TestVersion1DocumentUpgradesAndKeepsEnforcement(t *testing.T) {
	svc := newTestService(t)
	v1 := `{"version":1,"groups":[
		{"id":"kids","name":"Kids","action":"block","domains":["*.games.example"],"categories":["social"],
		 "rules":[{"id":"r1","enabled":true,"action":"allow","priority":0,
		           "schedule":{"timezone":"UTC","windows":[{"from":"16:00","to":"18:00"}]}}]},
		{"id":"work","name":"Work","action":"allow","domains":["tracker.example"],"users":["dana"]},
		{"id":"night","name":"Night","action":"block","domains":["video.example"],
		 "schedule":{"timezone":"UTC","windows":[{"from":"20:00","to":"07:00"}]}}
	],"assignments":[{"device_id":"device-1","group_id":"kids"}]}`
	if err := svc.storage.Update(StorageKey, v1); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	kids := snap.Groups["kids"]
	if len(kids.BlockedDomains) != 1 || kids.BlockedDomains[0] != "games.example" || len(kids.BlockedCategories) != 1 {
		t.Fatalf("kids = %+v, want the block carried into the blocked lists", kids)
	}
	if len(kids.Rules) != 1 || kids.Rules[0].Action != ActionAllow || len(kids.Rules[0].Target.Domains) != 1 {
		t.Fatalf("kids rules = %+v, want the allow rule targeting the old coverage", kids.Rules)
	}
	if work := snap.Groups["work"]; len(work.AllowedDomains) != 1 || len(work.Users) != 1 {
		t.Fatalf("work = %+v", work)
	}
	night := snap.Groups["night"]
	if len(night.BlockedDomains) != 0 || len(night.Rules) != 1 || night.Rules[0].Schedule == nil {
		t.Fatalf("night = %+v, want the scheduled group carried as a scheduled rule", night)
	}
	if _, ok := snap.Groups[DefaultGroupID]; !ok {
		t.Fatal("upgrade did not add the default policy")
	}
	if snap.Assignments["device-1"] != "kids" {
		t.Fatalf("assignments = %v", snap.Assignments)
	}

	identity := Identity{DeviceID: "device-1", GroupID: "kids", Source: SourceDevice}
	svc.SetClock(time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC))
	if got := svc.EvaluateDNS(identity, "chess.games.example").Action; got != ActionBlock {
		t.Fatalf("noon = %s, want block", got)
	}
	svc.SetClock(time.Date(2026, 9, 23, 17, 0, 0, 0, time.UTC))
	if got := svc.EvaluateDNS(identity, "chess.games.example").Action; got != ActionAllow {
		t.Fatalf("17:00 = %s, want the scheduled allow rule", got)
	}

	// Reading never rewrites storage; the next policy write persists v2.
	raw, _ := svc.storage.GetE(StorageKey)
	if !strings.Contains(raw, `"version":1`) {
		t.Fatal("a read rewrote the stored document")
	}
	if err := svc.SetDeviceAssignment("device-2", "work"); err != nil {
		t.Fatal(err)
	}
	raw, _ = svc.storage.GetE(StorageKey)
	if !strings.Contains(raw, `"version":2`) {
		t.Fatalf("document after a write = %s, want version 2", raw)
	}
}

func TestAllowGroupOverridesBlocklistForDomain(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "adults", AllowedDomains: []string{"tracker.example"}, Users: []string{"dana"},
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
		{ID: "kids", BlockedDomains: []string{"games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	// An address is refused as a policy user outright.
	if err := svc.CreateGroup(PolicyGroup{ID: "user-group", Name: "U", Users: []string{"192.0.2.10"}}); err == nil {
		t.Fatal("expected an address to be rejected as a proxy user")
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
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: "kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := svc.ResolveIdentity("192.0.2.10", "")
	if identity.Source != SourceStaleDevice || identity.GroupID != DefaultGroupID {
		t.Fatalf("identity = %+v, want stale_device on the default policy", identity)
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
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"games.example"}}}); err != nil {
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
	groups := []PolicyGroup{{ID: "category-kids", Name: "kids"}}
	assignments := []DeviceAssignment{{DeviceID: "device-1", GroupID: "category-kids"}}
	if err := svc.SaveMigration(groups, assignments, "legacy_device_metadata"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	if len(snap.Groups) != 2 || len(snap.Assignments) != 1 {
		t.Fatalf("snapshot = %+v, want the migrated group, the default policy, and one assignment", snap)
	}
	// An API edit after migration must survive a repeated migration call.
	if err := svc.SaveGroups([]PolicyGroup{{ID: "edited", BlockedDomains: []string{"x.example"}}}); err != nil {
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
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"games.example"}}}); err != nil {
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
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", Name: "Kids", BlockedDomains: []string{"games.example"}, CreatedAt: now, UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	raw, err := svc.storage.GetE(StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "\"version\":2") {
		t.Fatalf("document = %s", raw)
	}
}

func TestSetDeviceAssignmentSetClearAndUnknownGroup(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
		{ID: "adults", Name: "Adults"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := svc.Snapshot().Assignments["device-1"]; got != "kids" {
		t.Fatalf("assignment = %q, want kids", got)
	}

	// Reassignment replaces the previous group instead of stacking entries.
	if err := svc.SetDeviceAssignment("device-1", "adults"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if got := svc.Snapshot().Assignments["device-1"]; got != "adults" {
		t.Fatalf("reassignment = %q, want adults", got)
	}

	// An empty group id clears the assignment.
	if err := svc.SetDeviceAssignment("device-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, exists := svc.Snapshot().Assignments["device-1"]; exists {
		t.Fatal("cleared assignment still present after reload")
	}

	// An unknown group is rejected and leaves nothing behind.
	if err := svc.SetDeviceAssignment("device-1", "missing"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("error = %v, want ErrUnknownGroup", err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	if _, exists := svc.Snapshot().Assignments["device-1"]; exists {
		t.Fatal("rejected assignment leaked into the snapshot")
	}
}

func TestSetDeviceAssignmentPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewService(store, &mapResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"*.games.example"}}}); err != nil {
		t.Fatal(err)
	}
	if err := first.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}

	// A service instance created after restart must load the same assignment.
	restarted, err := NewService(store, &mapResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.Snapshot().Assignments["device-1"]; got != "kids" {
		t.Fatalf("assignment after restart = %q, want kids", got)
	}
	identity := restarted.ResolveIdentityForDNS("192.0.2.10", "")
	if identity.DeviceID != "" || identity.GroupID != DefaultGroupID {
		t.Fatalf("restart leaked a resolver from the previous instance: %+v", identity)
	}
}

func TestSetDeviceAssignmentConcurrentWritesPreserveOthers(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := svc.SetDeviceAssignment(fmt.Sprintf("device-%d", i), "kids"); err != nil {
				t.Errorf("set assignment: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	snap := svc.Snapshot()
	if len(snap.Assignments) != 20 {
		t.Fatalf("assignments = %d, want 20: per-device writes must not replace the full set", len(snap.Assignments))
	}
}

func TestUpdateTransformErrorAborts(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{{ID: "kids", BlockedDomains: []string{"games.example"}}}); err != nil {
		t.Fatal(err)
	}
	before, err := svc.storage.GetE(StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("boom")
	if err := svc.update(func(snap *PolicySnapshot) error { return boom }); err == nil {
		t.Fatal("expected transform error to propagate")
	}
	after, err := svc.storage.GetE(StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("aborted transform must leave the persisted document unchanged")
	}
}
