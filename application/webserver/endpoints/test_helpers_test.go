package gatesentryWebserverEndpoints

import (
	"path/filepath"
	"testing"
	"time"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
	"github.com/tidwall/buntdb"
)

// setupTestLogger creates a fresh BuntDB-backed logger in a temp directory.
func setupTestLogger(tmpDir string) *gatesentryLogger.Log {
	logPath := filepath.Join(tmpDir, "test_log.db")
	return gatesentryLogger.NewLogger(logPath)
}

// insertProxyLogEntry directly inserts a proxy log entry into the logger's BuntDB.
// This bypasses the async goroutine in LogProxy so the entry is immediately
// available for testing.
func insertProxyLogEntry(t *testing.T, logger *gatesentryLogger.Log, url, user, action string) {
	t.Helper()
	now := time.Now()
	secs := now.Unix()
	timestring := gatesentryUtils.Int64toString(secs)

	logJson := `{"time": ` + timestring + `, "ip":"` + user + `","url":"` + url + `", "type":"proxy", "proxyResponseType":"` + action + `"}`
	key := gatesentryUtils.RandomString(25) + timestring

	err := logger.Database.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(key, logJson, &buntdb.SetOptions{Expires: true, TTL: time.Hour})
		return err
	})
	if err != nil {
		t.Fatalf("failed to insert test log entry: %v", err)
	}
}
