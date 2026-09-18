package backup

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// backupFormatVersion is the archive format version this code writes and
// accepts. An archive from a newer format is rejected before live state is
// altered so a downgrade cannot silently drop fields it does not understand.
const backupFormatVersion = 1

// maxSettingsSchemaVersion must match application.CurrentSettingsSchema in
// migrations.go. It is duplicated here because the backup package cannot
// import the root gatesentryf package without a circular dependency. If the
// schema advances, update both constants together.
const maxSettingsSchemaVersion = 1

// maxDeviceAssignmentsVersion must match discovery.deviceAssignmentsVersion in
// persistence.go. Same duplication rationale as above.
const maxDeviceAssignmentsVersion = 1

const (
	settingsSchemaKey       = "settings_schema_version"
	deviceAssignmentsKey    = "assignments"
	recoveryDirName         = "recovery"
	installationKeyFilename = "installation.key"
)

// Archive is the versioned backup format. Settings and Devices are the raw
// MapStore key/value maps (decrypted plaintext, not the on-disk envelope).
// InstallationKey is the base64-encoded installation key file content, needed
// to decrypt the on-disk stores after a total data-directory loss. Restore
// does not overwrite the live key; the decrypted maps are re-encrypted with
// the current installation key on write.
type Archive struct {
	FormatVersion          int               `json:"format_version"`
	CreatedAt              time.Time         `json:"created_at"`
	BinaryVersion          string            `json:"binary_version,omitempty"`
	Settings               map[string]string `json:"settings"`
	Devices                map[string]string `json:"devices"`
	InstallationKey        string            `json:"installation_key"`
	InstallationKeyManaged bool              `json:"installation_key_managed"`
}

// RestoreResult reports the outcome of a restore operation. RecoveryPoint is
// the directory containing the pre-restore snapshots so an operator can roll
// back manually if needed.
type RestoreResult struct {
	SettingsKeys  int       `json:"settings_keys"`
	DeviceKeys    int       `json:"device_keys"`
	RecoveryPoint string    `json:"recovery_point"`
	RestoredAt    time.Time `json:"restored_at"`
}

// CreateBackup snapshots the settings and devices stores and reads the
// installation key, returning a JSON-encoded archive. The archive contains
// decrypted configuration; treat it as sensitive material.
func CreateBackup(settings, devices *gatesentry2storage.MapStore, binaryVersion string) ([]byte, error) {
	if settings == nil || devices == nil {
		return nil, errors.New("backup: settings and devices stores are required")
	}
	settingsSnapshot, err := settings.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("backup: snapshot settings: %w", err)
	}
	devicesSnapshot, err := devices.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("backup: snapshot devices: %w", err)
	}
	keyPath, managed := gatesentry2storage.InstallationKeyPath()
	keyData, keyErr := os.ReadFile(keyPath)
	if keyErr != nil && !errors.Is(keyErr, os.ErrNotExist) {
		return nil, fmt.Errorf("backup: read installation key: %w", keyErr)
	}
	archive := Archive{
		FormatVersion:          backupFormatVersion,
		CreatedAt:              time.Now().UTC(),
		BinaryVersion:          binaryVersion,
		Settings:               settingsSnapshot,
		Devices:                devicesSnapshot,
		InstallationKeyManaged: managed,
	}
	if keyErr == nil && len(keyData) > 0 {
		archive.InstallationKey = base64.StdEncoding.EncodeToString(keyData)
	}
	data, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("backup: encode archive: %w", err)
	}
	return data, nil
}

// ValidateArchive checks that the archive is structurally valid and
// compatible with this binary before any live state is altered. It fails
// closed on newer schemas, missing required keys, and malformed data.
func ValidateArchive(data []byte) (*Archive, error) {
	if len(data) == 0 {
		return nil, errors.New("restore: archive is empty")
	}
	var archive Archive
	if err := json.Unmarshal(data, &archive); err != nil {
		return nil, fmt.Errorf("restore: parse archive: %w", err)
	}
	if archive.FormatVersion == 0 {
		return nil, errors.New("restore: archive is missing format_version")
	}
	if archive.FormatVersion > backupFormatVersion {
		return nil, fmt.Errorf("restore: archive format version %d is newer than this release supports (%d); upgrade GateSentry before restoring", archive.FormatVersion, backupFormatVersion)
	}
	if archive.Settings == nil {
		return nil, errors.New("restore: archive is missing settings data")
	}
	if archive.Devices == nil {
		return nil, errors.New("restore: archive is missing devices data")
	}
	// Validate settings schema version - fail closed like MigrateSettings.
	rawVersion, ok := archive.Settings[settingsSchemaKey]
	if !ok || rawVersion == "" {
		return nil, fmt.Errorf("restore: settings are missing %q key; cannot verify schema compatibility", settingsSchemaKey)
	}
	schemaVersion, err := strconv.Atoi(rawVersion)
	if err != nil {
		return nil, fmt.Errorf("restore: invalid settings schema version %q: %w", rawVersion, err)
	}
	if schemaVersion > maxSettingsSchemaVersion {
		return nil, fmt.Errorf("restore: settings schema %d is newer than this release supports (%d); restore the matching newer GateSentry binary or the pre-upgrade backup", schemaVersion, maxSettingsSchemaVersion)
	}
	// Validate device assignments version if present.
	if assignments, ok := archive.Devices[deviceAssignmentsKey]; ok && assignments != "" {
		if err := validateDeviceAssignmentsVersion(assignments); err != nil {
			return nil, fmt.Errorf("restore: %w", err)
		}
	}
	return &archive, nil
}

// validateDeviceAssignmentsVersion checks that the device assignments JSON
// has a version this binary supports. This mirrors
// discovery.deviceAssignmentsVersion without importing the discovery package.
func validateDeviceAssignmentsVersion(raw string) error {
	var doc struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return fmt.Errorf("parse device assignments: %w", err)
	}
	if doc.Version > maxDeviceAssignmentsVersion {
		return fmt.Errorf("device assignments version %d is newer than this release supports (%d)", doc.Version, maxDeviceAssignmentsVersion)
	}
	return nil
}

// RestoreBackup validates the archive, creates a recovery point, and writes
// the restored data atomically. If the devices write fails after the settings
// write succeeds, settings are rolled back to preserve consistency. The
// caller must reload runtime state after this returns successfully.
func RestoreBackup(data []byte, settings, devices *gatesentry2storage.MapStore, baseDir string) (*RestoreResult, error) {
	archive, err := ValidateArchive(data)
	if err != nil {
		return nil, err
	}
	if settings == nil || devices == nil {
		return nil, errors.New("restore: settings and devices stores are required")
	}
	// Snapshot current state for rollback and the durable recovery point.
	currentSettings, err := settings.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("restore: snapshot current settings: %w", err)
	}
	currentDevices, err := devices.Snapshot()
	if err != nil {
		return nil, fmt.Errorf("restore: snapshot current devices: %w", err)
	}
	// Create a durable recovery point with the pre-restore snapshots.
	recoveryDir, err := createRecoveryPoint(baseDir, currentSettings, currentDevices)
	if err != nil {
		return nil, fmt.Errorf("restore: create recovery point: %w", err)
	}
	// Write restored data atomically. Roll back on failure.
	if err := settings.ReplaceAll(archive.Settings); err != nil {
		return nil, fmt.Errorf("restore: write settings: %w", err)
	}
	if err := devices.ReplaceAll(archive.Devices); err != nil {
		// Roll back settings to preserve consistency.
		if rbErr := settings.ReplaceAll(currentSettings); rbErr != nil {
			return nil, fmt.Errorf("restore: write devices failed: %v; rollback settings also failed: %v (recovery point at %s)", err, rbErr, recoveryDir)
		}
		return nil, fmt.Errorf("restore: write devices failed: %w (settings rolled back; recovery point at %s)", err, recoveryDir)
	}
	return &RestoreResult{
		SettingsKeys:  len(archive.Settings),
		DeviceKeys:    len(archive.Devices),
		RecoveryPoint: recoveryDir,
		RestoredAt:    time.Now().UTC(),
	}, nil
}

// createRecoveryPoint saves the pre-restore snapshots and the installation
// key to a timestamped directory. This preserves the pre-restore state so
// an interrupted or mistaken restore can be rolled back manually.
func createRecoveryPoint(baseDir string, settings, devices map[string]string) (string, error) {
	timestamp := time.Now().UTC().Format("20060102-150405")
	recoveryDir := filepath.Join(filepath.Clean(baseDir), recoveryDirName, timestamp)
	if err := os.MkdirAll(recoveryDir, 0700); err != nil {
		return "", fmt.Errorf("create recovery directory: %w", err)
	}
	if err := writeJSONFile(filepath.Join(recoveryDir, "settings.json"), settings); err != nil {
		return recoveryDir, fmt.Errorf("save settings snapshot: %w", err)
	}
	if err := writeJSONFile(filepath.Join(recoveryDir, "devices.json"), devices); err != nil {
		return recoveryDir, fmt.Errorf("save devices snapshot: %w", err)
	}
	// Copy the installation key if it exists.
	keyPath, _ := gatesentry2storage.InstallationKeyPath()
	if err := copyFile(keyPath, filepath.Join(recoveryDir, installationKeyFilename)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return recoveryDir, fmt.Errorf("copy installation key: %w", err)
	}
	return recoveryDir, nil
}

func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	dest, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dest.Close()
	if _, err := io.Copy(dest, source); err != nil {
		return err
	}
	return dest.Sync()
}
