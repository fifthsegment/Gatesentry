package gatesentry2storage

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func testMapStore(t *testing.T, encrypted bool) (*MapStore, string) {
	t.Helper()
	dir := t.TempDir()
	old := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { SetBaseDir(old) })
	store, err := OpenMapStore("settings", encrypted)
	if err != nil {
		t.Fatal(err)
	}
	return store, filepath.Join(dir, "settings")
}

func mustGet(t *testing.T, store *MapStore, key string) string {
	t.Helper()
	value, err := store.GetE(key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestStoreGetReturnsDefensiveCopy(t *testing.T) {
	store, _ := testMapStore(t, false)
	if err := store.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	first := store.baseStore.Get()
	first[0] ^= 0xff
	if got := store.GetOrDefault("key", ""); got != "value" {
		t.Fatalf("mutating returned bytes changed store: %q", got)
	}
}

func TestConcurrentMapUpdatesDoNotLoseValues(t *testing.T) {
	store, _ := testMapStore(t, true)
	const count = 40
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.Update(fmt.Sprintf("key-%d", i), strconv.Itoa(i)); err != nil {
				t.Errorf("update: %v", err)
			}
			_, _ = store.GetE(fmt.Sprintf("key-%d", i))
		}(i)
	}
	wg.Wait()
	for i := 0; i < count; i++ {
		if got := mustGet(t, store, fmt.Sprintf("key-%d", i)); got != strconv.Itoa(i) {
			t.Errorf("key-%d = %q", i, got)
		}
	}
}

func TestAtomicReadModifyWrite(t *testing.T) {
	store, _ := testMapStore(t, false)
	const count = 50
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := store.UpdateValue("counter", func(current string) (string, error) {
				n, _ := strconv.Atoi(current)
				return strconv.Itoa(n + 1), nil
			})
			if err != nil {
				t.Errorf("update value: %v", err)
			}
		}()
	}
	wg.Wait()
	got, err := store.GetIntE("counter")
	if err != nil {
		t.Fatal(err)
	}
	if got != count {
		t.Fatalf("counter = %d, want %d", got, count)
	}
}

func TestGetIntEReturnsReadAndParseErrors(t *testing.T) {
	store, path := testMapStore(t, false)
	if err := store.Update("counter", "not-an-integer"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetIntE("counter"); err == nil {
		t.Fatal("expected integer parse error")
	}
	if err := os.WriteFile(path, []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetIntE("counter"); err == nil {
		t.Fatal("expected storage read error")
	}
}

func TestUnpadRejectsInconsistentPaddingSuffix(t *testing.T) {
	padded := Pad([]byte("payload"))
	padded[len(padded)-2] ^= 1
	if _, err := Unpad(padded); err == nil {
		t.Fatal("expected invalid padding suffix error")
	}
}

func TestOpenMapStoreRejectsCorruptedNonFinalPaddingByte(t *testing.T) {
	dir := t.TempDir()
	old := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { SetBaseDir(old) })
	ciphertext, err := Encrypt([]byte("{}"), []byte(legacyEncryptionKey))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.URLEncoding.DecodeString(addBase64Padding(string(ciphertext)))
	if err != nil {
		t.Fatal(err)
	}
	decoded[len(decoded)-2] ^= 1
	corrupted := removeBase64Padding(base64.URLEncoding.EncodeToString(decoded))
	envelope, err := json.Marshal(map[string]string{"data": corrupted, "encrypted": "true"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings")
	if err := os.WriteFile(path, envelope, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMapStore("settings", true); err == nil || !strings.Contains(err.Error(), "invalid padding bytes") {
		t.Fatalf("error = %v, want invalid padding bytes", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(envelope) {
		t.Fatal("corrupted legacy file was modified")
	}
}

func TestConcurrentMapStoreInstancesDoNotLoseUpdates(t *testing.T) {
	dir := t.TempDir()
	old := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { SetBaseDir(old) })
	first, err := OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}

	const count = 40
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := first
			if i%2 == 1 {
				store = second
			}
			if err := store.Update(fmt.Sprintf("instance-key-%d", i), strconv.Itoa(i)); err != nil {
				t.Errorf("update: %v", err)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < count; i++ {
		if got := mustGet(t, first, fmt.Sprintf("instance-key-%d", i)); got != strconv.Itoa(i) {
			t.Errorf("instance-key-%d = %q", i, got)
		}
	}
}

func TestUpdateValuesFailurePreservesCompleteSnapshot(t *testing.T) {
	store, path := testMapStore(t, false)
	if err := store.UpdateValues(map[string]string{"mode": "disabled", "enabled": "false"}); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	oldRename := renameFile
	renameFile = func(_, _ string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { renameFile = oldRename })
	if err := store.UpdateValues(map[string]string{"mode": "local", "enabled": "true"}); err == nil {
		t.Fatal("expected transaction failure")
	}
	if got := store.GetOrDefault("mode", ""); got != "disabled" {
		t.Fatalf("mode changed to %q", got)
	}
	if got := store.GetOrDefault("enabled", ""); got != "false" {
		t.Fatalf("enabled changed to %q", got)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(original) {
		t.Fatal("transaction changed last-good file")
	}
}

func TestMalformedFileIsNotOverwrittenByDefaults(t *testing.T) {
	dir := t.TempDir()
	old := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { SetBaseDir(old) })
	path := filepath.Join(dir, "settings")
	original := []byte("not-json")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenMapStore("settings", false)
	if err == nil {
		t.Fatal("expected malformed storage error")
	}
	if err := store.SetDefault("key", "value"); err == nil {
		t.Fatal("default unexpectedly succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("malformed file changed to %q", got)
	}
}

func TestOpenMapStoreRejectsMalformedDecodedMap(t *testing.T) {
	tests := []struct {
		name     string
		envelope string
		want     string
	}{
		{name: "malformed inner map", envelope: `{"data":"not-json","encrypted":"false"}`, want: "parse map storage"},
		{name: "null inner map", envelope: `{"data":"null","encrypted":"false"}`, want: "payload must be a JSON object"},
		{name: "missing encrypted metadata", envelope: `{"data":"{}"}`, want: "missing encrypted field"},
		{name: "invalid encrypted metadata", envelope: `{"data":"{}","encrypted":"tru"}`, want: "invalid encrypted value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			old := GSBASEDIR
			SetBaseDir(dir + string(os.PathSeparator))
			t.Cleanup(func() { SetBaseDir(old) })
			path := filepath.Join(dir, "settings")
			original := []byte(tt.envelope)
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			store, err := OpenMapStore("settings", false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if store.Err() == nil {
				t.Fatal("store did not retain constructor error")
			}
			if err := store.SetDefault("key", "value"); err == nil {
				t.Fatal("default unexpectedly succeeded")
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != string(original) {
				t.Fatalf("malformed file changed to %q", got)
			}
		})
	}
}

func TestPersistenceFailurePreservesMemoryAndLastGoodFile(t *testing.T) {
	store, path := testMapStore(t, false)
	if err := store.Update("key", "old"); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	oldRename := renameFile
	renameFile = func(_, _ string) error { return errors.New("injected rename failure") }
	t.Cleanup(func() { renameFile = oldRename })
	if err := store.Update("key", "new"); err == nil || !strings.Contains(err.Error(), "rename") {
		t.Fatalf("update error = %v", err)
	}
	if got := store.GetOrDefault("key", ""); got != "old" {
		t.Fatalf("memory changed to %q", got)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("last-good file changed")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".settings.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func TestNewFileHasRestrictivePermissions(t *testing.T) {
	store, path := testMapStore(t, false)
	if err := store.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}

func TestPlaintextAndEncryptedFilesRemainCompatible(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		t.Run(fmt.Sprintf("encrypted-%t", encrypted), func(t *testing.T) {
			store, path := testMapStore(t, encrypted)
			if err := store.Update("key", "value"); err != nil {
				t.Fatal(err)
			}
			onDisk, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(onDisk), "\"data\"") || !strings.Contains(string(onDisk), "\"encrypted\"") {
				t.Fatalf("outer envelope changed: %s", onDisk)
			}
			reopened, err := OpenMapStore("settings", encrypted)
			if err != nil {
				t.Fatal(err)
			}
			if got := reopened.GetOrDefault("key", ""); got != "value" {
				t.Fatalf("reopened value = %q", got)
			}
		})
	}
}

func TestEncryptionFailureDoesNotCreateFile(t *testing.T) {
	store, path := testMapStore(t, true)
	managedKey := filepath.Join(t.TempDir(), "invalid-managed.key")
	if err := os.WriteFile(managedKey, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(managedKeyFileEnv, managedKey)
	if err := store.Update("key", "value"); err == nil {
		t.Fatal("expected encryption error")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file exists or unexpected error: %v", err)
	}
}

func TestWriteFailureIsReturned(t *testing.T) {
	dir := t.TempDir()
	old := GSBASEDIR
	SetBaseDir(filepath.Join(dir, "missing") + string(os.PathSeparator))
	t.Cleanup(func() { SetBaseDir(old) })
	store, err := OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("key", "value"); err == nil {
		t.Fatal("expected create failure")
	}
}

func TestAtomicWriteOperationFailuresAreReturned(t *testing.T) {
	tests := []struct {
		name    string
		install func() func()
		want    string
	}{
		{
			name: "write",
			install: func() func() {
				old := writeTemporaryFile
				writeTemporaryFile = func(*os.File, []byte) error { return errors.New("injected write failure") }
				return func() { writeTemporaryFile = old }
			},
			want: "write temporary storage file",
		},
		{
			name: "file sync",
			install: func() func() {
				old := syncFile
				syncFile = func(file *os.File) error {
					info, err := file.Stat()
					if err == nil && !info.IsDir() {
						return errors.New("injected sync failure")
					}
					return old(file)
				}
				return func() { syncFile = old }
			},
			want: "sync temporary storage file",
		},
		{
			name: "file close",
			install: func() func() {
				old := closeFile
				failed := false
				closeFile = func(file *os.File) error {
					info, _ := file.Stat()
					err := old(file)
					if !failed && info != nil && !info.IsDir() {
						failed = true
						return errors.New("injected close failure")
					}
					return err
				}
				return func() { closeFile = old }
			},
			want: "close temporary storage file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, path := testMapStore(t, false)
			if err := store.Update("key", "old"); err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			restore := tt.install()
			err = store.Update("key", "new")
			restore()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			onDisk, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(onDisk) != string(original) {
				t.Fatal("pre-rename failure changed last-good file")
			}
		})
	}
}

func TestDirectorySyncFailureQuarantinesVisibleSnapshotUntilRestart(t *testing.T) {
	store, path := testMapStore(t, false)
	if err := store.Update("key", "old"); err != nil {
		t.Fatal(err)
	}
	oldSync := syncFile
	oldDirSync := syncDirectory
	oldWrite := writeTemporaryFile
	writes := 0
	writeTemporaryFile = func(file *os.File, data []byte) error {
		writes++
		if writes > 1 {
			return errors.New("injected rollback preparation failure")
		}
		return oldWrite(file, data)
	}
	syncDirectory = func(file *os.File) error {
		return errors.New("injected directory sync failure")
	}
	t.Cleanup(func() {
		syncFile = oldSync
		syncDirectory = oldDirSync
		writeTemporaryFile = oldWrite
	})
	if err := store.Update("key", "value"); err == nil || !strings.Contains(err.Error(), "sync storage directory") {
		t.Fatalf("error = %v", err)
	}
	if writes != 1 {
		t.Fatalf("temporary payload writes = %d, want 1; rollback must not run after commit", writes)
	}
	if value, err := store.GetE("key"); err == nil || value != "value" {
		t.Fatalf("quarantined read = %q, %v; want committed snapshot with error", value, err)
	}
	if got := store.GetOrDefault("key", "fallback"); got != "fallback" {
		t.Fatalf("compatibility read = %q, want fail-closed fallback", got)
	}
	reopened, err := OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.GetOrDefault("key", ""); got != "value" {
		t.Fatalf("disk = %q, want committed value", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryOpenFailureQuarantinesVisibleSnapshotUntilRestart(t *testing.T) {
	store, _ := testMapStore(t, false)
	if err := store.Update("key", "old"); err != nil {
		t.Fatal(err)
	}
	oldOpen := openDirectory
	openDirectory = func(string) (*os.File, error) { return nil, errors.New("injected directory open failure") }
	err := store.Update("key", "new")
	openDirectory = oldOpen
	if err == nil || !strings.Contains(err.Error(), "open storage directory for sync") {
		t.Fatalf("error = %v", err)
	}
	if value, readErr := store.GetE("key"); readErr == nil || value != "new" {
		t.Fatalf("quarantined read = %q, %v; want committed snapshot with error", value, readErr)
	}
	reopened, err := OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.GetE("key"); err != nil || got != "new" {
		t.Fatalf("reopened read = %q, %v", got, err)
	}
}

func TestGetEReturnsKnownGoodSnapshotWithRefreshError(t *testing.T) {
	store, path := testMapStore(t, false)
	if err := store.Update("enable_https_filtering", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err := store.GetE("enable_https_filtering")
	if err == nil {
		t.Fatal("GetE succeeded after durable data was corrupted")
	}
	if value != "true" {
		t.Fatalf("known-good value = %q, want true", value)
	}
}
