package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func assignmentsStore(t *testing.T) (*gatesentry2storage.MapStore, string) {
	t.Helper()
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("GSDevices", true)
	if err != nil {
		t.Fatal(err)
	}
	return store, filepath.Join(dir, "GSDevices")
}

func TestDeviceAssignmentPersistAndRestore(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{
		ID:         "dev-1",
		ManualName: "Family laptop",
		Owner:      "Vivienne",
		Category:   "kids",
		Hostnames:  []string{"laptop.local"},
		MACs:       []string{"aa:bb:cc:dd:ee:ff"},
		IPv4:       "192.168.1.10",
		Persistent: true,
	}); err != nil {
		t.Fatal(err)
	}

	restored := NewDeviceStore("local")
	if err := restored.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	device := restored.GetDevice("dev-1")
	if device == nil {
		t.Fatal("device assignment was not restored")
	}
	if device.ManualName != "Family laptop" || device.Owner != "Vivienne" || device.Category != "kids" {
		t.Fatalf("assignment fields = %+v", device)
	}
	if len(device.Hostnames) != 1 || device.Hostnames[0] != "laptop.local" {
		t.Fatalf("hostnames = %v", device.Hostnames)
	}
	if !device.Persistent {
		t.Fatal("named device must be persistent")
	}
}

func TestDeviceAssignmentRemovePersists(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", ManualName: "tablet", Persistent: true}); err != nil {
		t.Fatal(err)
	}
	if err := ds.RemoveDeviceE("dev-1"); err != nil {
		t.Fatal(err)
	}
	restored := NewDeviceStore("local")
	if err := restored.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if got := restored.GetDevice("dev-1"); got != nil {
		t.Fatalf("removed device restored: %+v", got)
	}
}

func TestDeviceAssignmentObservedChurnDoesNotWrite(t *testing.T) {
	store, path := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", ManualName: "printer", Persistent: true}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", IPv4: "192.168.1.50", Source: SourcePassive, Persistent: true}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("observed-only churn must not rewrite the assignment store")
	}
}

func TestDeviceAssignmentMergeWithRediscovery(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", ManualName: "laptop", Owner: "Dad", Persistent: true}); err != nil {
		t.Fatal(err)
	}

	restarted := NewDeviceStore("local")
	if err := restarted.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.UpsertDeviceE(&Device{ID: "dev-1", Hostnames: []string{"laptop"}, IPv4: "10.1.1.4"}); err != nil {
		t.Fatal(err)
	}
	device := restarted.GetDevice("dev-1")
	if device == nil {
		t.Fatal("rediscovered device missing")
	}
	if device.ManualName != "laptop" || device.Owner != "Dad" {
		t.Fatalf("user assignment lost on rediscovery: %+v", device)
	}
	if device.IPv4 != "10.1.1.4" {
		t.Fatalf("rediscovered IP missing: %q", device.IPv4)
	}
}

func TestDeviceAssignmentUnsupportedVersionFailsClosed(t *testing.T) {
	store, path := assignmentsStore(t)
	if err := store.Update(DeviceAssignmentsKey, `{"version":99,"assignments":{"dev-1":{"id":"dev-1"}}}`); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ds := NewDeviceStore("local")
	err = ds.AttachPersistence(store)
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("error = %v", err)
	}
	if got := ds.GetDevice("dev-1"); got != nil {
		t.Fatalf("unsupported assignment must not load: %+v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed assignment load must leave the device store byte-identical")
	}
}

func TestDeviceAssignmentV2DuplicateNormalizedTailscaleNodeFailsClosed(t *testing.T) {
	original := `{"version":2,"assignments":{"phone":{"id":"phone","manual_name":"Phone","tailscale_nodes":[{"node_id":" node-1 "}]},"tablet":{"id":"tablet","manual_name":"Tablet","tailscale_nodes":[{"node_id":"node-1"}]}}}`
	store := &assignmentFailStore{value: original}
	ds := NewDeviceStore("local")
	if _, err := ds.UpsertDeviceE(&Device{ID: "resident", ManualName: "Resident", Persistent: true}); err != nil {
		t.Fatal(err)
	}

	err := ds.AttachPersistence(store)
	if err == nil || !strings.Contains(err.Error(), "assigned to multiple devices") {
		t.Fatalf("error = %v, want duplicate Tailscale node error", err)
	}
	if store.rawValue() != original {
		t.Fatal("duplicate assignment load changed original assignment bytes")
	}
	if store.updateCount() != 0 {
		t.Fatal("duplicate assignment load attempted a migration write")
	}
	if ds.DeviceCount() != 1 || ds.GetDevice("resident") == nil {
		t.Fatalf("duplicate assignment load changed RAM: %+v", ds.GetAllDevices())
	}
	if ds.GetDevice("phone") != nil || ds.GetDevice("tablet") != nil {
		t.Fatal("duplicate assignments were partially loaded")
	}
}

func TestDeviceAssignmentMalformedFailsClosed(t *testing.T) {
	store, _ := assignmentsStore(t)
	if err := store.Update(DeviceAssignmentsKey, `{"version":1,"assignments":[]}`); err != nil {
		t.Fatal(err)
	}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err == nil {
		t.Fatal("expected malformed assignment error")
	}
}

func TestDeviceAssignmentInvalidPayloadLeavesBytesAndRAMUnchanged(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "malformed json", raw: `{"version":1,"assignments":`},
		{name: "malformed assignments", raw: `{"version":1,"assignments":[]}`},
		{name: "future version", raw: `{"version":4,"assignments":{"new":{"id":"new","persistent":true}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &assignmentFailStore{value: tt.raw}
			ds := NewDeviceStore("local")
			before, err := ds.UpsertDeviceE(&Device{
				ID: "resident", ManualName: "Resident", IPv4: "192.0.2.8", Persistent: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := ds.AttachPersistence(store); err == nil {
				t.Fatal("expected invalid assignment payload to fail")
			}
			if store.rawValue() != tt.raw {
				t.Fatal("failed load changed original assignment bytes")
			}
			got := ds.GetDevice(before)
			if got == nil || got.ManualName != "Resident" || got.IPv4 != "192.0.2.8" || !got.Persistent {
				t.Fatalf("failed load changed RAM: %+v", got)
			}
			if ds.DeviceCount() != 1 {
				t.Fatalf("failed load changed inventory size to %d", ds.DeviceCount())
			}
		})
	}
}

func TestDeviceAssignmentCorruptedFileFailsClosed(t *testing.T) {
	store, path := assignmentsStore(t)
	if err := store.Update(DeviceAssignmentsKey, `{"version":1,"assignments":{}}`); err != nil {
		t.Fatal(err)
	}
	// Replace the encrypted envelope with raw garbage. Opening the store must
	// fail closed without rewriting the file, mirroring the storage-layer
	// fixtures for the other encrypted stores.
	corrupted := []byte("not-a-valid-gatesentry-store")
	if err := os.WriteFile(path, corrupted, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := gatesentry2storage.OpenMapStore("GSDevices", true); err == nil {
		t.Fatal("expected corrupted device store to fail opening")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(corrupted) {
		t.Fatal("corrupted device store must remain byte-identical after failed open")
	}
}

func TestUpdateManualFieldsClearsOwnerCategory(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{
		ID:         "dev-1",
		ManualName: "laptop",
		Owner:      "Dana",
		Category:   "kids",
		IPv4:       "192.0.2.10",
		Source:     SourcePassive,
		Sources:    []DiscoverySource{SourcePassive},
	}); err != nil {
		t.Fatal(err)
	}

	updated, ok, err := ds.UpdateManualFieldsE("dev-1", ManualFieldsUpdate{
		Owner:       "",
		Category:    "",
		SetOwner:    true,
		SetCategory: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected device to be found")
	}
	if updated.Owner != "" || updated.Category != "" {
		t.Fatalf("owner/category = %q/%q, want cleared", updated.Owner, updated.Category)
	}
	if updated.ManualName != "laptop" {
		t.Fatalf("manual name = %q, want preserved", updated.ManualName)
	}
	if updated.IPv4 != "192.0.2.10" || len(updated.Sources) != 1 {
		t.Fatalf("observed identity changed: %+v", updated)
	}

	// Cleared labels must survive a restart, not just the in-memory copy.
	restored := NewDeviceStore("local")
	if err := restored.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	device := restored.GetDevice("dev-1")
	if device == nil {
		t.Fatal("device missing after restart")
	}
	if device.Owner != "" || device.Category != "" {
		t.Fatalf("cleared labels restored: owner=%q category=%q", device.Owner, device.Category)
	}
	if device.ManualName != "laptop" {
		t.Fatalf("manual name lost after restart: %q", device.ManualName)
	}
}

func TestUpdateManualFieldsOmittedPreservesValues(t *testing.T) {
	ds := NewDeviceStore("local")
	if _, err := ds.UpsertDeviceE(&Device{
		ID:       "dev-1",
		Owner:    "Dana",
		Category: "kids",
		IPv4:     "192.0.2.10",
		Source:   SourcePassive,
		Sources:  []DiscoverySource{SourcePassive},
	}); err != nil {
		t.Fatal(err)
	}
	before := ds.GetDevice("dev-1")

	updated, ok, err := ds.UpdateManualFieldsE("dev-1", ManualFieldsUpdate{
		ManualName:    "tablet",
		SetManualName: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected device to be found")
	}
	if updated.Owner != "Dana" || updated.Category != "kids" {
		t.Fatalf("omitted fields changed: owner=%q category=%q", updated.Owner, updated.Category)
	}
	if updated.ManualName != "tablet" {
		t.Fatalf("manual name = %q, want tablet", updated.ManualName)
	}
	if updated.IPv4 != "192.0.2.10" || len(updated.Sources) != 1 {
		t.Fatalf("observed identity changed: %+v", updated)
	}
	if !updated.LastSeen.Equal(before.LastSeen) {
		t.Fatal("manual field update must not refresh LastSeen")
	}

	// An unknown device reports not-found without an error.
	_, ok, err = ds.UpdateManualFieldsE("missing", ManualFieldsUpdate{ManualName: "x", SetManualName: true})
	if err != nil || ok {
		t.Fatalf("unknown device: ok=%v err=%v, want false/nil", ok, err)
	}
}

type assignmentFailStore struct {
	mu           sync.Mutex
	value        string
	failUpdates  bool
	beforeUpdate func()
	updates      int
}

func (s *assignmentFailStore) GetE(string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value, nil
}
func (s *assignmentFailStore) UpdateValue(_ string, update func(string) (string, error)) error {
	s.mu.Lock()
	s.updates++
	hook := s.beforeUpdate
	s.beforeUpdate = nil
	s.mu.Unlock()
	if hook != nil {
		hook()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failUpdates {
		return errors.New("write failed")
	}
	next, err := update(s.value)
	if err == nil {
		s.value = next
	}
	return err
}

func (s *assignmentFailStore) setFailure(fail bool) {
	s.mu.Lock()
	s.failUpdates = fail
	s.mu.Unlock()
}

func (s *assignmentFailStore) rawValue() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

func (s *assignmentFailStore) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updates
}

func TestFailedMigrationDoesNotPopulateInventory(t *testing.T) {
	original := `{"version":1,"assignments":{"named":{"id":"named","dns_name":"phone"}}}`
	store := &assignmentFailStore{value: original, failUpdates: true}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err == nil {
		t.Fatal("expected migration write failure")
	}
	if store.rawValue() != original {
		t.Fatal("failed migration changed original assignment bytes")
	}
	if ds.DeviceCount() != 0 {
		t.Fatalf("failed migration populated %d devices", ds.DeviceCount())
	}
}

func TestDurableMutationFailuresLeaveInventoryAndDiskUnchanged(t *testing.T) {
	store := &assignmentFailStore{}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", ManualName: "Phone", Persistent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.LinkTailscaleIdentityE("dev-1", TailscaleIdentity{NodeID: "node-1", Name: "Pixel"}); err != nil {
		t.Fatal(err)
	}
	beforeDevice := ds.GetDevice("dev-1")
	beforeDisk := store.rawValue()
	store.setFailure(true)

	if _, _, err := ds.UpdateManualFieldsE("dev-1", ManualFieldsUpdate{ManualName: "Changed", SetManualName: true}); err == nil {
		t.Fatal("expected manual-field write failure")
	}
	if got := ds.GetDevice("dev-1"); got.ManualName != beforeDevice.ManualName {
		t.Fatalf("failed manual update changed RAM: %+v", got)
	}
	if _, err := ds.UnlinkTailscaleIdentityE("dev-1", "node-1"); err == nil {
		t.Fatal("expected unlink write failure")
	}
	if got := ds.FindDeviceByTailscaleNode("node-1"); got == nil || got.ID != "dev-1" {
		t.Fatalf("failed unlink changed RAM: %+v", got)
	}
	if err := ds.RemoveDeviceE("dev-1"); err == nil {
		t.Fatal("expected removal write failure")
	}
	if ds.GetDevice("dev-1") == nil {
		t.Fatal("failed removal deleted RAM device")
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", ManualName: "Upserted", Persistent: true}); err == nil {
		t.Fatal("expected upsert write failure")
	}
	if got := ds.GetDevice("dev-1"); got.ManualName != beforeDevice.ManualName {
		t.Fatalf("failed upsert changed RAM: %+v", got)
	}
	if store.rawValue() != beforeDisk {
		t.Fatal("failed durable mutations changed disk")
	}
}

func TestMarkDevicePersistentFailureLeavesInventoryAndDiskUnchanged(t *testing.T) {
	store := &assignmentFailStore{}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "passive", IPv4: "192.0.2.10", Source: SourcePassive}); err != nil {
		t.Fatal(err)
	}
	beforeDisk := store.rawValue()
	store.setFailure(true)

	if _, ok, err := ds.MarkDevicePersistentE("passive"); err == nil || !ok {
		t.Fatalf("mark persistent: ok=%v err=%v, want true/error", ok, err)
	}
	if got := ds.GetDevice("passive"); got == nil || got.Persistent {
		t.Fatalf("failed retention changed RAM: %+v", got)
	}
	if store.rawValue() != beforeDisk {
		t.Fatal("failed retention changed disk")
	}
	if _, ok, err := ds.MarkDevicePersistentE("missing"); err != nil || ok {
		t.Fatalf("missing device: ok=%v err=%v, want false/nil", ok, err)
	}
}

func TestApplyTailscaleSnapshotFailureRollsBackAllDevices(t *testing.T) {
	store := &assignmentFailStore{}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"dev-1", "dev-2"} {
		if _, err := ds.UpsertDeviceE(&Device{ID: id, ManualName: id, Persistent: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := ds.LinkTailscaleIdentityE(id, TailscaleIdentity{NodeID: "node-" + id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	beforeDisk := store.rawValue()
	store.setFailure(true)
	peers := []TailscaleIdentity{
		{NodeID: "node-dev-1", Name: "renamed-1", Online: true, Addresses: []string{"100.64.0.1"}},
		{NodeID: "node-dev-2", Name: "renamed-2", Online: true, Addresses: []string{"100.64.0.2"}},
	}
	if err := ds.ApplyTailscaleSnapshot(peers); err == nil {
		t.Fatal("expected snapshot write failure")
	}
	for _, id := range []string{"dev-1", "dev-2"} {
		got := ds.GetDevice(id)
		if got == nil || got.TailscaleNodes[0].Online || len(got.TailscaleNodes[0].Addresses) != 0 || got.TailscaleNodes[0].Name != id {
			t.Fatalf("failed snapshot changed %s in RAM: %+v", id, got)
		}
	}
	if store.rawValue() != beforeDisk {
		t.Fatal("failed snapshot changed disk")
	}
}

func TestApplyTailscaleSnapshotDoesNotOverwriteConcurrentUnlink(t *testing.T) {
	store := &assignmentFailStore{}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", ManualName: "Phone", Persistent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.LinkTailscaleIdentityE("dev-1", TailscaleIdentity{NodeID: "node-1", Name: "Pixel"}); err != nil {
		t.Fatal(err)
	}

	snapshotAtWrite := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	store.mu.Lock()
	store.beforeUpdate = func() {
		close(snapshotAtWrite)
		<-releaseSnapshot
	}
	store.mu.Unlock()

	snapshotDone := make(chan error, 1)
	go func() {
		snapshotDone <- ds.ApplyTailscaleSnapshot([]TailscaleIdentity{{NodeID: "node-1", Name: "Renamed", Online: true, Addresses: []string{"100.64.0.1"}}})
	}()
	<-snapshotAtWrite
	unlinkDone := make(chan error, 1)
	go func() {
		_, err := ds.UnlinkTailscaleIdentityE("dev-1", "node-1")
		unlinkDone <- err
	}()
	close(releaseSnapshot)
	if err := <-snapshotDone; err != nil {
		t.Fatal(err)
	}
	if err := <-unlinkDone; err != nil {
		t.Fatal(err)
	}
	if got := ds.FindDeviceByTailscaleNode("node-1"); got != nil {
		t.Fatalf("concurrent unlink was overwritten in RAM: %+v", got)
	}
	restored := NewDeviceStore("local")
	if err := restored.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if got := restored.FindDeviceByTailscaleNode("node-1"); got != nil {
		t.Fatalf("concurrent unlink was overwritten on disk: %+v", got)
	}
}

func TestDeviceAssignmentV1MigrationKeepsOnlyDurableIntent(t *testing.T) {
	store, _ := assignmentsStore(t)
	legacy := `{"version":1,"assignments":{` +
		`"empty":{"id":"empty","dns_name":""},` +
		`"passive-named":{"id":"passive-named","dns_name":"phone","hostnames":["phone"],"mdns_names":["Phone.local"],"macs":["aa:bb:cc:dd:ee:ff"],"first_seen":"2025-01-02T03:04:05Z"},` +
		`"persistent":{"id":"persistent","dns_name":"printer","persistent":true},` +
		`"manual-name":{"id":"manual-name","dns_name":"tablet","manual_name":"Family tablet"},` +
		`"owner":{"id":"owner","dns_name":"laptop","owner":"Dana"},` +
		`"category":{"id":"category","dns_name":"console","category":"kids"}}}`
	if err := store.Update(DeviceAssignmentsKey, legacy); err != nil {
		t.Fatal(err)
	}

	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"empty", "passive-named"} {
		if got := ds.GetDevice(id); got != nil {
			t.Fatalf("observed-only v1 record %q survived migration: %+v", id, got)
		}
	}
	for _, id := range []string{"persistent", "manual-name", "owner", "category"} {
		got := ds.GetDevice(id)
		if got == nil {
			t.Fatalf("durable v1 record %q was pruned", id)
		}
		if got.Online || !got.LastSeen.IsZero() {
			t.Fatalf("restored durable record %q = %+v, want offline with zero last seen", id, got)
		}
	}

	raw, err := store.GetE(DeviceAssignmentsKey)
	if err != nil {
		t.Fatal(err)
	}
	decoded, sourceVersion, err := decodeDeviceAssignmentsVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	if sourceVersion != deviceAssignmentsVersion || decoded.Version != deviceAssignmentsVersion {
		t.Fatalf("versions = %d/%d, want %d", sourceVersion, decoded.Version, deviceAssignmentsVersion)
	}
	if len(decoded.Assignments) != 4 {
		t.Fatalf("migrated assignments = %v, want four durable records", decoded.Assignments)
	}
}

func TestDeviceAssignmentMigrationRetainsProtectedIDOnlyRecord(t *testing.T) {
	store, _ := assignmentsStore(t)
	if err := store.Update(DeviceAssignmentsKey, `{"version":1,"assignments":{"policy-device":{"id":"policy-device","dns_name":"generated-name","hostnames":["generated-name"],"macs":["aa:bb:cc:dd:ee:ff"]}}}`); err != nil {
		t.Fatal(err)
	}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistenceWithProtectedIDs(store, map[string]bool{"policy-device": true}); err != nil {
		t.Fatal(err)
	}
	if ds.GetDevice("policy-device") == nil {
		t.Fatal("policy-referenced legacy record was pruned")
	}
	raw, err := store.GetE(DeviceAssignmentsKey)
	if err != nil {
		t.Fatal(err)
	}
	decoded, sourceVersion, err := decodeDeviceAssignmentsVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	if sourceVersion != deviceAssignmentsVersion {
		t.Fatalf("source version = %d, want %d", sourceVersion, deviceAssignmentsVersion)
	}
	if _, ok := decoded.Assignments["policy-device"]; !ok {
		t.Fatal("protected record was removed from migrated bytes")
	}
}

func TestLinkedTailscaleIdentityPersistsWithoutRuntimeMetadata(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "dev-1", ManualName: "Phone", Persistent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.LinkTailscaleIdentityE("dev-1", TailscaleIdentity{
		NodeID: "node-1", Name: "pixel", DNSName: "pixel.tailnet", Addresses: []string{"100.64.0.7"}, Online: true, LastSeen: time.Now(), WoLMACs: []string{"aa:bb:cc:dd:ee:ff"},
	}); err != nil {
		t.Fatal(err)
	}

	restarted := NewDeviceStore("local")
	if err := restarted.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	device := restarted.GetDevice("dev-1")
	if device == nil || len(device.TailscaleNodes) != 1 {
		t.Fatalf("restored device = %+v", device)
	}
	identity := device.TailscaleNodes[0]
	if identity.NodeID != "node-1" || identity.Name != "pixel" || identity.DNSName != "pixel.tailnet" {
		t.Fatalf("durable identity = %+v", identity)
	}
	if identity.Online || len(identity.Addresses) != 1 || identity.Addresses[0] != "100.64.0.7" || len(identity.WoLMACs) != 0 || !identity.LastSeen.IsZero() {
		t.Fatalf("restored identity metadata: %+v", identity)
	}
	if got := restarted.FindDeviceByIP("100.64.0.7"); got == nil || got.ID != "dev-1" {
		t.Fatalf("restored Tailscale alias = %+v", got)
	}
	if device.Online || !device.LastSeen.IsZero() {
		t.Fatalf("restored reachability = online:%v last_seen:%v", device.Online, device.LastSeen)
	}
}

func TestPersistedTailscaleAliasPreventsDuplicateAfterRestart(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "phone", ManualName: "Pixel", Persistent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.LinkTailscaleIdentityE("phone", TailscaleIdentity{NodeID: "node-1", Addresses: []string{"100.64.0.8"}, Online: true}); err != nil {
		t.Fatal(err)
	}

	restarted := NewDeviceStore("local")
	if err := restarted.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	id, created, err := restarted.ObserveDevice(Device{IPv4: "100.64.0.8", Source: SourcePassive})
	if err != nil {
		t.Fatal(err)
	}
	if created || id != "phone" || restarted.DeviceCount() != 1 {
		t.Fatalf("observation = id %q created %v count %d", id, created, restarted.DeviceCount())
	}
	device := restarted.GetDevice("phone")
	if device.IPv4 != "" {
		t.Fatalf("overlay alias was copied into LAN IPv4: %+v", device)
	}
}

func TestDeviceAssignmentV2MigratesWithoutTailscaleAddresses(t *testing.T) {
	store, _ := assignmentsStore(t)
	if err := store.Update(DeviceAssignmentsKey, `{"version":2,"assignments":{"phone":{"id":"phone","manual_name":"Pixel","tailscale_nodes":[{"node_id":"node-1","name":"pixel"}]}}}`); err != nil {
		t.Fatal(err)
	}
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	device := ds.GetDevice("phone")
	if device == nil || len(device.TailscaleNodes) != 1 || len(device.TailscaleNodes[0].Addresses) != 0 {
		t.Fatalf("migrated v2 device = %+v", device)
	}
	raw, err := store.GetE(DeviceAssignmentsKey)
	if err != nil {
		t.Fatal(err)
	}
	decoded, version, err := decodeDeviceAssignmentsVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	if version != deviceAssignmentsVersion || decoded.Version != deviceAssignmentsVersion {
		t.Fatalf("migrated versions = %d/%d", version, decoded.Version)
	}
}

func TestTransientPassiveAndMDNSDevicesAreNotPersisted(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "passive", IPv4: "192.0.2.10", Source: SourcePassive}); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{ID: "mdns", IPv4: "192.0.2.11", Hostnames: []string{"printer"}, Source: SourceMDNS}); err != nil {
		t.Fatal(err)
	}

	restarted := NewDeviceStore("local")
	if err := restarted.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if restarted.DeviceCount() != 0 {
		t.Fatalf("restored %d transient devices", restarted.DeviceCount())
	}
}

func TestMarkDevicePersistentRestoresIdentityWithoutLANClaim(t *testing.T) {
	store, _ := assignmentsStore(t)
	ds := NewDeviceStore("local")
	if err := ds.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&Device{
		ID: "phone", IPv4: "192.0.2.10", MACs: []string{"aa:bb:cc:dd:ee:ff"},
		Source: SourcePassive, Sources: []DiscoverySource{SourcePassive},
	}); err != nil {
		t.Fatal(err)
	}
	marked, ok, err := ds.MarkDevicePersistentE("phone")
	if err != nil || !ok {
		t.Fatalf("mark persistent: ok=%v err=%v", ok, err)
	}
	if !marked.Persistent || marked.IPv4 != "192.0.2.10" {
		t.Fatalf("marked device = %+v", marked)
	}

	restarted := NewDeviceStore("local")
	if err := restarted.AttachPersistence(store); err != nil {
		t.Fatal(err)
	}
	restored := restarted.GetDevice("phone")
	if restored == nil || !restored.Persistent {
		t.Fatalf("restored device = %+v", restored)
	}
	if restored.IPv4 != "" || restored.Online || !restored.LastSeen.IsZero() {
		t.Fatalf("restored runtime observation = %+v", restored)
	}
	id, created, err := restarted.ObserveDevice(Device{
		IPv4: "192.0.2.44", MACs: []string{"aa:bb:cc:dd:ee:ff"},
		Source: SourcePassive, Sources: []DiscoverySource{SourcePassive},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created || id != "phone" {
		t.Fatalf("rediscovery = id %q created=%v, want phone/false", id, created)
	}
	if got := restarted.GetDevice("phone"); got.IPv4 != "192.0.2.44" {
		t.Fatalf("rediscovered device = %+v", got)
	}
}
