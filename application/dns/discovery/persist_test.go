package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPersistRoundTrip verifies that devices survive a save→load cycle.
func TestPersistRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "devices.json")

	// Create store, add devices, save
	ds := NewDeviceStoreMultiZone("local")
	ds.persist = &persistState{
		filePath: fp,
		interval: time.Hour, // disable auto-fire for deterministic tests
	}

	ds.UpsertDevice(&Device{
		Hostnames: []string{"macmini"},
		IPv4:      "192.168.1.10",
		Source:    SourceDDNS,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	})
	ds.UpsertDevice(&Device{
		Hostnames: []string{"printer"},
		IPv4:      "192.168.1.20",
		Source:    SourceMDNS,
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	})

	if err := ds.SaveNow(); err != nil {
		t.Fatalf("SaveNow() failed: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("cannot read persisted file: %v", err)
	}
	var store persistedStore
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatalf("invalid JSON in persisted file: %v", err)
	}
	if store.Version != 1 {
		t.Errorf("version = %d, want 1", store.Version)
	}
	if len(store.Devices) != 2 {
		t.Fatalf("persisted %d devices, want 2", len(store.Devices))
	}

	// Create a fresh store and load
	ds2 := NewDeviceStoreMultiZone("local")
	ds2.SetPersistPath(fp)

	devices := ds2.GetAllDevices()
	if len(devices) != 2 {
		t.Fatalf("loaded %d devices, want 2", len(devices))
	}

	// Verify data integrity
	found := false
	for _, d := range devices {
		if d.DNSName == "macmini" {
			found = true
			if d.IPv4 != "192.168.1.10" {
				t.Errorf("macmini IPv4 = %q, want 192.168.1.10", d.IPv4)
			}
			if d.Source != SourceDDNS {
				t.Errorf("macmini Source = %v, want %v", d.Source, SourceDDNS)
			}
		}
	}
	if !found {
		t.Error("macmini device not found after load")
	}
}

// TestPersistEphemeralFiltering ensures devices without a DNSName
// and without the Persistent flag are NOT saved to disk.
func TestPersistEphemeralFiltering(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "devices.json")

	ds := NewDeviceStoreMultiZone("local")
	ds.persist = &persistState{
		filePath: fp,
		interval: time.Hour,
	}

	// Device with DNS name — should be persisted
	ds.UpsertDevice(&Device{
		Hostnames: []string{"laptop"},
		IPv4:      "192.168.1.30",
		Source:    SourceDDNS,
	})

	// Ephemeral IP-only device — should NOT be persisted
	ds.UpsertDevice(&Device{
		IPv4:   "192.168.1.99",
		Source: SourcePassive,
	})

	// Persistent flag set but no DNS name — should be persisted
	ds.UpsertDevice(&Device{
		IPv4:       "192.168.1.100",
		Source:     SourcePassive,
		Persistent: true,
	})

	if err := ds.SaveNow(); err != nil {
		t.Fatalf("SaveNow() failed: %v", err)
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("cannot read persisted file: %v", err)
	}
	var store persistedStore
	if err := json.Unmarshal(data, &store); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should have laptop + persistent-flagged device, NOT the ephemeral one
	if len(store.Devices) != 2 {
		t.Errorf("persisted %d devices, want 2 (laptop + persistent)", len(store.Devices))
		for _, d := range store.Devices {
			t.Logf("  - %s (dns=%q, persistent=%v)", d.ID, d.DNSName, d.Persistent)
		}
	}
}

// TestPersistLoadDoesNotOverwrite ensures that already-discovered devices
// are not replaced by older persisted data.
func TestPersistLoadDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "devices.json")

	// Save a device with old IP
	ds1 := NewDeviceStoreMultiZone("local")
	ds1.persist = &persistState{
		filePath: fp,
		interval: time.Hour,
	}
	ds1.UpsertDevice(&Device{
		Hostnames: []string{"server"},
		IPv4:      "10.0.0.1",
		Source:    SourceDDNS,
	})
	if err := ds1.SaveNow(); err != nil {
		t.Fatalf("SaveNow() failed: %v", err)
	}

	// Create new store and add the same device with NEW IP before loading
	ds2 := NewDeviceStoreMultiZone("local")
	ds2.UpsertDevice(&Device{
		Hostnames: []string{"server"},
		IPv4:      "10.0.0.99",
		Source:    SourceDDNS,
	})

	// Now enable persistence — this loads from disk
	ds2.SetPersistPath(fp)

	// The in-memory device should keep its newer IP
	devices := ds2.GetAllDevices()
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if devices[0].IPv4 != "10.0.0.99" {
		t.Errorf("IPv4 = %q, want 10.0.0.99 (should not be overwritten by persisted 10.0.0.1)", devices[0].IPv4)
	}
}

// TestPersistNoFile verifies graceful handling when no persist file exists.
func TestPersistNoFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "nonexistent", "devices.json")

	ds := NewDeviceStoreMultiZone("local")
	// Should not panic, just log a message
	ds.SetPersistPath(fp)

	// Should work fine with no data loaded
	devices := ds.GetAllDevices()
	if len(devices) != 0 {
		t.Errorf("got %d devices, want 0 for empty/missing persist file", len(devices))
	}

	// Should be able to save after adding a device
	ds.UpsertDevice(&Device{
		Hostnames: []string{"test"},
		IPv4:      "1.2.3.4",
		Source:    SourceDDNS,
	})
	if err := ds.SaveNow(); err != nil {
		t.Fatalf("SaveNow() after adding device failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		t.Error("persist file was not created")
	}
}

// TestPersistDNSRecordsRebuilt ensures that DNS records are properly
// regenerated from persisted devices after loading.
func TestPersistDNSRecordsRebuilt(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "devices.json")

	// Save a device
	ds1 := NewDeviceStoreMultiZone("local")
	ds1.persist = &persistState{
		filePath: fp,
		interval: time.Hour,
	}
	ds1.UpsertDevice(&Device{
		Hostnames: []string{"webserver"},
		IPv4:      "192.168.1.50",
		Source:    SourceDDNS,
	})
	if err := ds1.SaveNow(); err != nil {
		t.Fatalf("SaveNow() failed: %v", err)
	}

	// Load into fresh store
	ds2 := NewDeviceStoreMultiZone("local")
	ds2.SetPersistPath(fp)

	// DNS lookup should work for the loaded device
	records := ds2.LookupAll("webserver.local.")
	if len(records) == 0 {
		t.Fatal("no DNS records for webserver.local. after loading from disk")
	}

	foundA := false
	for _, rr := range records {
		if rr.Type == 1 { // A record
			foundA = true
		}
	}
	if !foundA {
		t.Error("expected A record for webserver.local. not found")
	}

	// PTR reverse lookup should also work
	reverseRecords := ds2.LookupReverse("50.1.168.192.in-addr.arpa.")
	if len(reverseRecords) == 0 {
		t.Error("no PTR records for 50.1.168.192.in-addr.arpa. after loading from disk")
	}
}

// TestSaveNowNoPersist ensures SaveNow is a no-op when persistence is not configured.
func TestSaveNowNoPersist(t *testing.T) {
	ds := NewDeviceStoreMultiZone("local")
	// persist is nil — should return nil, not panic
	if err := ds.SaveNow(); err != nil {
		t.Errorf("SaveNow() with no persist should return nil, got: %v", err)
	}
}
