package gatesentryf

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func TestInitReturnsMalformedSettingsError(t *testing.T) {
	dir := t.TempDir()
	oldBaseDir := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { SetBaseDir(oldBaseDir) })
	if err := os.WriteFile(filepath.Join(dir, "GSSettings"), []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &GSRuntime{DNSServerChannel: make(chan int, 1)}
	err := runtime.Init()
	if err == nil || !strings.Contains(err.Error(), "open settings storage") {
		t.Fatalf("Init error = %v", err)
	}
}

func TestConcurrentGSWasUpdatedPreservesEveryVersionRecord(t *testing.T) {
	dir := t.TempDir()
	oldStorageBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(oldStorageBaseDir) })

	settings, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	updateLog, err := gatesentry2storage.OpenMapStore("updates", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Update("version", "old-version"); err != nil {
		t.Fatal(err)
	}

	oldVersion := GSVerString
	SetGSVer("new-version")
	t.Cleanup(func() { SetGSVer(oldVersion) })
	runtime := &GSRuntime{GSSettings: settings, GSUpdateLog: updateLog}

	const updates = 40
	var wg sync.WaitGroup
	for i := 0; i < updates; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := runtime.GSWasUpdated(); err != nil {
				t.Errorf("GSWasUpdated: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := strings.Count(updateLog.GetOrDefault("versions", ""), " - new-version on = "); got != updates {
		t.Fatalf("version records = %d, want %d", got, updates)
	}
}
