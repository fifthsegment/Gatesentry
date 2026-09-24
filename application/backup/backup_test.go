package backup

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// setupTestDir creates a temp data directory with a dummy installation key,
// sets it as the storage base dir, and restores the previous base dir on
// cleanup. Returns the directory path.
func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	keyPath := filepath.Join(dir, "installation.key")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func openStores(t *testing.T) (settings, devices *gatesentry2storage.MapStore) {
	t.Helper()
	var err error
	settings, err = gatesentry2storage.OpenMapStore("GSSettings", false)
	if err != nil {
		t.Fatal(err)
	}
	devices, err = gatesentry2storage.OpenMapStore("GSDevices", false)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func seedStores(t *testing.T, settings, devices *gatesentry2storage.MapStore) {
	t.Helper()
	if err := settings.Update("timezone", "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Update("settings_schema_version", "1"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Update("strictness", "2000"); err != nil {
		t.Fatal(err)
	}
	if err := devices.Update("assignments", `{"version":2,"assignments":{"dev-1":{"id":"dev-1","dns_name":"phone","tailscale_nodes":[{"node_id":"node-1","name":"phone-tailnet"}]}}}`); err != nil {
		t.Fatal(err)
	}
}

func TestCreateBackupSeedsEmptyDeviceAssignments(t *testing.T) {
	setupTestDir(t)
	settings, devices := openStores(t)
	if err := settings.Update("settings_schema_version", "1"); err != nil {
		t.Fatal(err)
	}

	data, err := CreateBackup(settings, devices, "test")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := ValidateArchive(data)
	if err != nil {
		t.Fatalf("clean-install backup is not restorable: %v", err)
	}
	if got := archive.Devices[deviceAssignmentsKey]; got != `{"version":3,"assignments":{}}` {
		t.Fatalf("assignments = %q", got)
	}
}

// TestBackupRoundTrip verifies that a backup created from a configured
// installation can be restored to a clean install and all data matches.
func TestBackupRoundTrip(t *testing.T) {
	dir := setupTestDir(t)
	settings, devices := openStores(t)
	seedStores(t, settings, devices)

	// Create backup.
	data, err := CreateBackup(settings, devices, "test-v1.0")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Verify archive structure.
	var archive Archive
	if err := json.Unmarshal(data, &archive); err != nil {
		t.Fatalf("unmarshal archive: %v", err)
	}
	if archive.FormatVersion != backupFormatVersion {
		t.Fatalf("format version = %d, want %d", archive.FormatVersion, backupFormatVersion)
	}
	if archive.BinaryVersion != "test-v1.0" {
		t.Fatalf("binary version = %q, want test-v1.0", archive.BinaryVersion)
	}
	if archive.Settings["timezone"] != "UTC" {
		t.Fatalf("timezone = %q, want UTC", archive.Settings["timezone"])
	}
	if archive.InstallationKey == "" {
		t.Fatal("installation key is empty")
	}

	// Restore to a clean directory (simulate clean install).
	dir2 := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir2 + string(os.PathSeparator))
	defer gatesentry2storage.SetBaseDir(old)
	// Copy the key so the installation key is available.
	keyData, _ := os.ReadFile(filepath.Join(dir, "installation.key"))
	if err := os.WriteFile(filepath.Join(dir2, "installation.key"), keyData, 0600); err != nil {
		t.Fatal(err)
	}

	settings2, devices2 := openStores(t) // empty stores

	result, err := RestoreBackup(data, settings2, devices2, dir2)
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if result.SettingsKeys != 3 {
		t.Fatalf("settings keys = %d, want 3", result.SettingsKeys)
	}
	if result.DeviceKeys != 1 {
		t.Fatalf("device keys = %d, want 1", result.DeviceKeys)
	}

	// Verify restored settings data.
	tz, err := settings2.GetE("timezone")
	if err != nil {
		t.Fatalf("read timezone: %v", err)
	}
	if tz != "UTC" {
		t.Fatalf("timezone = %q, want UTC", tz)
	}
	schema, err := settings2.GetE("settings_schema_version")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if schema != "1" {
		t.Fatalf("schema = %q, want 1", schema)
	}

	// Verify restored devices data.
	assign, err := devices2.GetE("assignments")
	if err != nil {
		t.Fatalf("read assignments: %v", err)
	}
	if !strings.Contains(assign, "dev-1") {
		t.Fatalf("assignments = %q, expected dev-1", assign)
	}

	// Verify recovery point exists and contains pre-restore (empty) snapshots.
	settingsSnapPath := filepath.Join(result.RecoveryPoint, "settings.json")
	snapData, err := os.ReadFile(settingsSnapPath)
	if err != nil {
		t.Fatalf("read recovery settings: %v", err)
	}
	var snap map[string]string
	if err := json.Unmarshal(snapData, &snap); err != nil {
		t.Fatalf("unmarshal recovery settings: %v", err)
	}
	if len(snap) != 0 {
		t.Fatalf("recovery settings has %d keys, want 0 (clean install)", len(snap))
	}
}

// TestValidateArchiveInvalid verifies that malformed, incomplete, or
// incompatible archives are rejected before any live state is altered.
func TestValidateArchiveInvalid(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"not_json", []byte("not json at all")},
		{"missing_format_version", []byte(`{"settings":{},"devices":{}}`)},
		{"newer_format_version", []byte(`{"format_version":99,"settings":{},"devices":{}}`)},
		{"missing_settings", []byte(`{"format_version":1,"devices":{}}`)},
		{"missing_devices", []byte(`{"format_version":1,"settings":{}}`)},
		{"missing_schema_version", []byte(`{"format_version":1,"settings":{},"devices":{}}`)},
		{"newer_schema_version", []byte(`{"format_version":1,"settings":{"settings_schema_version":"99"},"devices":{}}`)},
		{"non_numeric_schema", []byte(`{"format_version":1,"settings":{"settings_schema_version":"abc"},"devices":{}}`)},
		{
			"newer_device_version",
			[]byte(`{"format_version":1,"settings":{"settings_schema_version":"1"},"devices":{"assignments":"{\"version\":3}"}}`),
		},
		{
			"malformed_device_version",
			[]byte(`{"format_version":1,"settings":{"settings_schema_version":"1"},"devices":{"assignments":"not json"}}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ValidateArchive(tt.data); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestValidateDeviceAssignmentsVersionCompatibility(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		t.Run(fmt.Sprintf("accepts_v%d", version), func(t *testing.T) {
			raw := fmt.Sprintf(`{"version":%d,"assignments":{}}`, version)
			if err := validateDeviceAssignmentsVersion(raw); err != nil {
				t.Fatalf("validateDeviceAssignmentsVersion(v%d): %v", version, err)
			}
		})
	}

	tests := []struct {
		name string
		raw  string
	}{
		{name: "missing version", raw: `{"assignments":{}}`},
		{name: "zero version", raw: `{"version":0,"assignments":{}}`},
		{name: "negative version", raw: `{"version":-1,"assignments":{}}`},
		{name: "malformed version", raw: `{"version":"2","assignments":{}}`},
		{name: "unsupported version", raw: `{"version":4,"assignments":{}}`},
		{name: "malformed document", raw: `{"version":2`},
		{name: "missing assignments", raw: `{"version":2}`},
		{name: "null assignments", raw: `{"version":2,"assignments":null}`},
		{name: "malformed assignments", raw: `{"version":2,"assignments":[]}`},
	}
	for _, tt := range tests {
		t.Run("rejects_"+tt.name, func(t *testing.T) {
			if err := validateDeviceAssignmentsVersion(tt.raw); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

func TestValidateArchiveRequiresDeviceAssignments(t *testing.T) {
	for _, devices := range []string{`{}`, `{"assignments":""}`} {
		data := []byte(fmt.Sprintf(`{"format_version":1,"settings":{"settings_schema_version":"1"},"devices":%s}`, devices))
		if _, err := ValidateArchive(data); err == nil {
			t.Fatalf("ValidateArchive with devices %s: expected validation error", devices)
		}
	}
}

// TestValidateArchiveAcceptsValid verifies that a well-formed archive passes.
func TestValidateArchiveAcceptsValid(t *testing.T) {
	setupTestDir(t)
	settings, devices := openStores(t)
	seedStores(t, settings, devices)

	data, err := CreateBackup(settings, devices, "test")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	archive, err := ValidateArchive(data)
	if err != nil {
		t.Fatalf("ValidateArchive: %v", err)
	}
	if archive.Settings["timezone"] != "UTC" {
		t.Fatalf("timezone = %q, want UTC", archive.Settings["timezone"])
	}
}

// TestRestoreRollbackOnDeviceWriteFailure verifies that if the devices
// write fails after the settings write succeeds, settings are rolled back.
func TestRestoreRollbackOnDeviceWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test permission-based write failure as root")
	}
	dir := setupTestDir(t)

	// Open settings store in the base directory (writable).
	settings, err := gatesentry2storage.OpenMapStore("GSSettings", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Update("timezone", "Original-TZ"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Update("settings_schema_version", "1"); err != nil {
		t.Fatal(err)
	}

	// Open devices store in a subdirectory so we can make only that
	// subdirectory read-only while settings stays writable.
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	devices, err := gatesentry2storage.OpenMapStore("sub/GSDevices", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := devices.Update("assignments", `{"version":1,"assignments":{}}`); err != nil {
		t.Fatal(err)
	}

	// Create a valid backup archive from the current state.
	archiveData, err := CreateBackup(settings, devices, "test")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Mutate settings so the archive has different settings than live state.
	if err := settings.Update("timezone", "Restored-TZ"); err != nil {
		t.Fatal(err)
	}

	// Make the devices subdirectory read-only so devices.ReplaceAll fails.
	if err := os.Chmod(subDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(subDir, 0755)

	// Restore should fail on devices write and roll back settings.
	_, err = RestoreBackup(archiveData, settings, devices, dir)
	if err == nil {
		t.Fatal("expected restore to fail on devices write")
	}
	if !strings.Contains(err.Error(), "rolled back") && !strings.Contains(err.Error(), "rollback") {
		t.Fatalf("error should mention rollback: %v", err)
	}
}

// TestRecoveryPointPreservesOriginalState verifies that the recovery point
// contains the pre-restore snapshots so an operator can manually roll back.
func TestRecoveryPointPreservesOriginalState(t *testing.T) {
	dir := setupTestDir(t)
	settings, devices := openStores(t)
	seedStores(t, settings, devices)

	// Create a backup, then modify live state so the archive differs.
	archiveData, err := CreateBackup(settings, devices, "test")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Mutate live state so restore actually changes something.
	if err := settings.Update("timezone", "Changed-TZ"); err != nil {
		t.Fatal(err)
	}

	// Restore from the backup (which has the original timezone).
	result, err := RestoreBackup(archiveData, settings, devices, dir)
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	// Verify the recovery point contains the pre-restore state (Changed-TZ).
	settingsSnapPath := filepath.Join(result.RecoveryPoint, "settings.json")
	snapData, err := os.ReadFile(settingsSnapPath)
	if err != nil {
		t.Fatalf("read recovery settings: %v", err)
	}
	var snap map[string]string
	if err := json.Unmarshal(snapData, &snap); err != nil {
		t.Fatalf("unmarshal recovery settings: %v", err)
	}
	if snap["timezone"] != "Changed-TZ" {
		t.Fatalf("recovery settings timezone = %q, want Changed-TZ (pre-restore state)", snap["timezone"])
	}

	// Verify live state was restored to the original.
	tz, err := settings.GetE("timezone")
	if err != nil {
		t.Fatalf("read timezone: %v", err)
	}
	if tz != "UTC" {
		t.Fatalf("timezone = %q, want UTC (restored)", tz)
	}

	// Verify the installation key was copied to the recovery point.
	keyPath := filepath.Join(result.RecoveryPoint, "installation.key")
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("recovery point installation key not found: %v", err)
	}
}

// TestCreateBackupRejectsNilStores verifies that nil stores are rejected.
func TestCreateBackupRejectsNilStores(t *testing.T) {
	_, err := CreateBackup(nil, nil, "test")
	if err == nil {
		t.Fatal("expected error for nil stores")
	}
}

// TestRestoreBackupRejectsNilStores verifies that nil stores are rejected.
func TestRestoreBackupRejectsNilStores(t *testing.T) {
	data := []byte(`{"format_version":1,"settings":{"settings_schema_version":"1"},"devices":{},"installation_key":"dGVzdA=="}`)
	_, err := RestoreBackup(data, nil, nil, "/tmp")
	if err == nil {
		t.Fatal("expected error for nil stores")
	}
}
