package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	gatesentryBackup "bitbucket.org/abdullah_irfan/gatesentryf/backup"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// BackupDeps carries the stores and callbacks the backup handlers need.
type BackupDeps struct {
	Settings *gatesentry2storage.MapStore
	Devices  *gatesentry2storage.MapStore
	BaseDir  string
	Reload   func() error
	Version  func() string
}

// GSApiBackupGET creates a backup archive and returns it as a JSON download.
// GET /api/backup
func GSApiBackupGET(w http.ResponseWriter, r *http.Request, deps BackupDeps) {
	if deps.Settings == nil || deps.Devices == nil {
		http.Error(w, `{"error":"Backup stores not initialized"}`, http.StatusServiceUnavailable)
		return
	}
	version := ""
	if deps.Version != nil {
		version = deps.Version()
	}
	data, err := gatesentryBackup.CreateBackup(deps.Settings, deps.Devices, version)
	if err != nil {
		http.Error(w, `{"error":"Unable to create backup"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=gatesentry-backup.json")
	w.Write(data)
}

// GSApiRestorePOST accepts a backup archive, validates it, creates a recovery
// point, and restores the data.
// POST /api/restore
func GSApiRestorePOST(w http.ResponseWriter, r *http.Request, deps BackupDeps) {
	if deps.Settings == nil || deps.Devices == nil {
		http.Error(w, `{"error":"Backup stores not initialized"}`, http.StatusServiceUnavailable)
		return
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 16<<20))
	if err != nil {
		http.Error(w, `{"error":"Unable to read backup archive"}`, http.StatusBadRequest)
		return
	}
	result, err := gatesentryBackup.RestoreBackup(data, deps.Settings, deps.Devices, deps.BaseDir)
	if err != nil {
		if isRestoreValidationError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, `{"error":"Unable to restore backup"}`, http.StatusInternalServerError)
		}
		return
	}
	if deps.Reload != nil {
		if err := deps.Reload(); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPartialContent)
		json.NewEncoder(w).Encode(map[string]string{
			"warning":         "Restore succeeded but reload failed; restart GateSentry to apply changes",
			"recovery_point":  result.RecoveryPoint,
		})
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"settings_keys":   result.SettingsKeys,
		"device_keys":     result.DeviceKeys,
		"recovery_point":  result.RecoveryPoint,
		"restored_at":     result.RestoredAt.Format(time.RFC3339),
	})
}

func isRestoreValidationError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "restore:") || strings.Contains(msg, "schema") || strings.Contains(msg, "format version") || strings.Contains(msg, "missing")
}
