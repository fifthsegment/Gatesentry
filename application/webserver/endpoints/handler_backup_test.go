package gatesentryWebserverEndpoints

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gatesentryBackup "bitbucket.org/abdullah_irfan/gatesentryf/backup"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func backupTestDir(t *testing.T) string {
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

func backupTestStores(t *testing.T) (settings, devices *gatesentry2storage.MapStore) {
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

func seedBackupStores(t *testing.T, settings, devices *gatesentry2storage.MapStore) {
	t.Helper()
	if err := settings.Update("timezone", "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := settings.Update("settings_schema_version", "1"); err != nil {
		t.Fatal(err)
	}
	if err := devices.Update("assignments", `{"version":1,"assignments":{"dev-1":{"id":"dev-1","dns_name":"phone"}}}`); err != nil {
		t.Fatal(err)
	}
}

func TestGSApiBackupGETSuccess(t *testing.T) {
	dir := backupTestDir(t)
	settings, devices := backupTestStores(t)
	seedBackupStores(t, settings, devices)

	deps := BackupDeps{
		Settings: settings,
		Devices:  devices,
		BaseDir:  dir,
		Version:  func() string { return "test-v1" },
	}

	req := httptest.NewRequest("GET", "/api/backup", nil)
	rr := httptest.NewRecorder()
	GSApiBackupGET(rr, req, deps)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content type = %q, want application/json", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "gatesentry-backup.json") {
		t.Fatalf("content disposition = %q, want filename=gatesentry-backup.json", cd)
	}

	// Verify the body is a valid archive.
	archive, err := gatesentryBackup.ValidateArchive(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("validate archive: %v", err)
	}
	if archive.BinaryVersion != "test-v1" {
		t.Fatalf("binary version = %q, want test-v1", archive.BinaryVersion)
	}
}

func TestGSApiBackupGETNilStores(t *testing.T) {
	deps := BackupDeps{}
	req := httptest.NewRequest("GET", "/api/backup", nil)
	rr := httptest.NewRecorder()
	GSApiBackupGET(rr, req, deps)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestGSApiRestorePOSTSuccess(t *testing.T) {
	dir := backupTestDir(t)
	settings, devices := backupTestStores(t)
	seedBackupStores(t, settings, devices)

	// Create a backup archive.
	archiveData, err := gatesentryBackup.CreateBackup(settings, devices, "test")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Mutate live state so restore changes something.
	if err := settings.Update("timezone", "Changed-TZ"); err != nil {
		t.Fatal(err)
	}

	reloadCalled := false
	deps := BackupDeps{
		Settings: settings,
		Devices:  devices,
		BaseDir:  dir,
		Reload: func() error {
			reloadCalled = true
			return nil
		},
	}

	req := httptest.NewRequest("POST", "/api/restore", bytes.NewReader(archiveData))
	rr := httptest.NewRecorder()
	GSApiRestorePOST(rr, req, deps)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !reloadCalled {
		t.Fatal("reload was not called after successful restore")
	}

	// Verify timezone was restored.
	tz, err := settings.GetE("timezone")
	if err != nil {
		t.Fatalf("read timezone: %v", err)
	}
	if tz != "UTC" {
		t.Fatalf("timezone = %q, want UTC (restored)", tz)
	}

	// Verify response body has the expected fields.
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if _, ok := resp["recovery_point"]; !ok {
		t.Fatal("response missing recovery_point")
	}
}

func TestGSApiRestorePOSTInvalidArchive(t *testing.T) {
	dir := backupTestDir(t)
	settings, devices := backupTestStores(t)

	deps := BackupDeps{
		Settings: settings,
		Devices:  devices,
		BaseDir:  dir,
		Reload:   func() error { return nil },
	}

	req := httptest.NewRequest("POST", "/api/restore", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	GSApiRestorePOST(rr, req, deps)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGSApiRestorePOSTIncompatibleSchema(t *testing.T) {
	dir := backupTestDir(t)
	settings, devices := backupTestStores(t)

	badArchive := `{"format_version":1,"settings":{"settings_schema_version":"99"},"devices":{},"installation_key":""}`
	deps := BackupDeps{
		Settings: settings,
		Devices:  devices,
		BaseDir:  dir,
		Reload:   func() error { return nil },
	}

	req := httptest.NewRequest("POST", "/api/restore", bytes.NewReader([]byte(badArchive)))
	rr := httptest.NewRecorder()
	GSApiRestorePOST(rr, req, deps)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestGSApiRestorePOSTNilStores(t *testing.T) {
	deps := BackupDeps{}
	req := httptest.NewRequest("POST", "/api/restore", bytes.NewReader([]byte("{}")))
	rr := httptest.NewRecorder()
	GSApiRestorePOST(rr, req, deps)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}
}

func TestGSApiRestorePOSTReloadFailure(t *testing.T) {
	dir := backupTestDir(t)
	settings, devices := backupTestStores(t)
	seedBackupStores(t, settings, devices)

	archiveData, err := gatesentryBackup.CreateBackup(settings, devices, "test")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Mutate so restore actually does something.
	if err := settings.Update("timezone", "Changed"); err != nil {
		t.Fatal(err)
	}

	deps := BackupDeps{
		Settings: settings,
		Devices:  devices,
		BaseDir:  dir,
		Reload:   func() error { return io.ErrUnexpectedEOF },
	}

	req := httptest.NewRequest("POST", "/api/restore", bytes.NewReader(archiveData))
	rr := httptest.NewRecorder()
	GSApiRestorePOST(rr, req, deps)

	// Reload failure returns 206 Partial Content.
	if rr.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d (body: %s)", rr.Code, http.StatusPartialContent, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "warning") {
		t.Fatalf("response should contain warning: %s", rr.Body.String())
	}
}
