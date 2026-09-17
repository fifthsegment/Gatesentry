package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
