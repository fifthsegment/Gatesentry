package gatesentry2storage

import (
	"crypto/aes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func useStorageDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { SetBaseDir(old) })
	return dir
}

func writeLegacyStore(t *testing.T, path string, plaintext []byte) {
	t.Helper()
	ciphertext, err := Encrypt(plaintext, []byte(legacyEncryptionKey))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"data": string(ciphertext), "encrypted": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readEnvelopeForTest(t *testing.T, path string) envelope {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestLegacyStoreMigratesToAuthenticatedEncryption(t *testing.T) {
	dir := useStorageDir(t)
	path := filepath.Join(dir, "settings")
	writeLegacyStore(t, path, []byte(`{"key":"value"}`))

	store, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, store, "key"); got != "value" {
		t.Fatalf("key = %q", got)
	}
	if got := readEnvelopeForTest(t, path).Version; got != gcmEnvelopeVersion {
		t.Fatalf("version = %q, want %q", got, gcmEnvelopeVersion)
	}
	keyInfo, err := os.Stat(filepath.Join(dir, installationKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if got := keyInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("installation key mode = %04o, want 0600", got)
	}
}

func TestLegacyMigrationFailureLeavesStoreUntouched(t *testing.T) {
	dir := useStorageDir(t)
	path := filepath.Join(dir, "settings")
	writeLegacyStore(t, path, []byte(`{"key":"value"}`))
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, installationKeyFilename), make([]byte, keySize), 0600); err != nil {
		t.Fatal(err)
	}
	oldRename := renameFile
	renameFile = func(_, _ string) error { return errors.New("simulated migration failure") }
	_, openErr := OpenMapStore("settings", true)
	renameFile = oldRename
	if openErr == nil || !strings.Contains(openErr.Error(), "simulated migration failure") {
		t.Fatalf("error = %v, want migration failure", openErr)
	}
	assertFileBytes(t, path, original)
}

func TestAuthenticatedWritesUseFreshNonces(t *testing.T) {
	store, path := testMapStore(t, true)
	if err := store.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	first := readEnvelopeForTest(t, path).Data
	if err := store.baseStore.Persist(); err != nil {
		t.Fatal(err)
	}
	second := readEnvelopeForTest(t, path).Data
	if first == second {
		t.Fatal("two writes reused the authenticated ciphertext/nonce")
	}
}

func TestMissingInstallationKeyLeavesAuthenticatedStoreUntouched(t *testing.T) {
	dir := useStorageDir(t)
	store, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, installationKeyFilename)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMapStore("settings", true); err == nil || !strings.Contains(err.Error(), "restore it from backup") {
		t.Fatalf("error = %v, want missing-key recovery guidance", err)
	}
	assertFileBytes(t, path, original)
	if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing key was unexpectedly replaced: %v", err)
	}
}

func TestMissingInstallationKeyCannotBeReplacedThroughAnotherStore(t *testing.T) {
	dir := useStorageDir(t)
	existing, err := OpenMapStore("GSWebSettings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := existing.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(dir, "GSWebSettings")
	original, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, installationKeyFilename)); err != nil {
		t.Fatal(err)
	}

	missing, err := OpenMapStore("GSSettings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := missing.Update("other", "value"); err == nil || !strings.Contains(err.Error(), "GSWebSettings") {
		t.Fatalf("error = %v, want cross-store missing-key recovery error", err)
	}
	assertFileBytes(t, existingPath, original)
	if _, err := os.Stat(filepath.Join(dir, "GSSettings")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new store was unexpectedly written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing key was unexpectedly replaced: %v", err)
	}
}

func TestMissingInstallationKeyCannotOrphanNestedStore(t *testing.T) {
	dir := useStorageDir(t)
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	existing, err := OpenMapStore(filepath.Join("nested", "settings"), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := existing.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(dir, "nested", "settings")
	original, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, installationKeyFilename)); err != nil {
		t.Fatal(err)
	}

	missing, err := OpenMapStore("other", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := missing.Update("other", "value"); err == nil || !strings.Contains(err.Error(), filepath.Join("nested", "settings")) {
		t.Fatalf("error = %v, want nested-store missing-key recovery error", err)
	}
	assertFileBytes(t, existingPath, original)
	if _, err := os.Stat(filepath.Join(dir, "other")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new store was unexpectedly written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing key was unexpectedly replaced: %v", err)
	}
}

func TestMissingInstallationKeyCannotOrphanDotPrefixedStore(t *testing.T) {
	dir := useStorageDir(t)
	existing, err := OpenMapStore(".settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := existing.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(dir, ".settings")
	original, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, installationKeyFilename)); err != nil {
		t.Fatal(err)
	}

	missing, err := OpenMapStore("other", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := missing.Update("other", "value"); err == nil || !strings.Contains(err.Error(), ".settings") {
		t.Fatalf("error = %v, want dot-prefixed missing-key recovery error", err)
	}
	assertFileBytes(t, existingPath, original)
	if _, err := os.Stat(filepath.Join(dir, "other")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new store was unexpectedly written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing key was unexpectedly replaced: %v", err)
	}
}

func TestAtomicTemporaryStoreNamesAreReserved(t *testing.T) {
	useStorageDir(t)
	for _, name := range []string{".settings.tmp-123", ".installation.key.create-456"} {
		if _, err := OpenMapStore(name, true); err == nil || !strings.Contains(err.Error(), "reserved") {
			t.Fatalf("OpenMapStore(%q) error = %v, want reserved-name error", name, err)
		}
	}
}

func TestMissingInstallationKeyCannotBeReplacedForCorruptedGCMEnvelope(t *testing.T) {
	for _, version := range []string{"", "aes-256-gcm-v999"} {
		version := version
		t.Run(fmt.Sprintf("version-%q", version), func(t *testing.T) {
			dir := useStorageDir(t)
			existing, err := OpenMapStore("GSWebSettings", true)
			if err != nil {
				t.Fatal(err)
			}
			if err := existing.Update("key", "value"); err != nil {
				t.Fatal(err)
			}
			existingPath := filepath.Join(dir, "GSWebSettings")
			parsed := readEnvelopeForTest(t, existingPath)
			parsed.Version = version
			corrupted, err := json.Marshal(parsed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(existingPath, corrupted, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(dir, installationKeyFilename)); err != nil {
				t.Fatal(err)
			}

			missing, err := OpenMapStore("GSSettings", true)
			if err != nil {
				t.Fatal(err)
			}
			if err := missing.Update("other", "value"); err == nil || !strings.Contains(err.Error(), "GSWebSettings") {
				t.Fatalf("error = %v, want cross-store missing-key recovery error", err)
			}
			assertFileBytes(t, existingPath, corrupted)
			if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing key was unexpectedly replaced: %v", err)
			}
		})
	}
}

func TestMissingKeyScanBoundsNonEnvelopeInspection(t *testing.T) {
	dir := useStorageDir(t)
	largePath := filepath.Join(dir, "log.db")
	file, err := os.Create(largePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(128 * 1024 * 1024); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	stores, err := authenticatedStoresWithoutKey(filepath.Join(dir, installationKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 0 {
		t.Fatalf("protected stores = %v, want none", stores)
	}
}

func TestCorruptedLegacyMapIsNotMigratedBeforeValidation(t *testing.T) {
	dir := useStorageDir(t)
	path := filepath.Join(dir, "settings")
	plaintext := []byte(`{"key":"value"}`) // 15 bytes: one byte of valid padding.
	writeLegacyStore(t, path, plaintext)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseEnvelope(data)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.URLEncoding.DecodeString(addBase64Padding(parsed.Data))
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte in the only plaintext block, away from its final padding
	// byte. CFB still decrypts with valid padding, but the JSON map is corrupt.
	ciphertext[aes.BlockSize+1] ^= 0x01
	parsed.Data = removeBase64Padding(base64.URLEncoding.EncodeToString(ciphertext))
	corrupted, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, corrupted, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenMapStore("settings", true); err == nil || !strings.Contains(err.Error(), "parse map storage") {
		t.Fatalf("error = %v, want map validation failure", err)
	}
	assertFileBytes(t, path, corrupted)
	if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid legacy store unexpectedly caused key creation: %v", err)
	}
}

func TestWrongInstallationKeyLeavesStoreUntouched(t *testing.T) {
	dir := useStorageDir(t)
	store, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := make([]byte, keySize)
	for i := range wrong {
		wrong[i] = byte(i + 1)
	}
	if err := os.WriteFile(filepath.Join(dir, installationKeyFilename), wrong, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMapStore("settings", true); err == nil || !strings.Contains(err.Error(), "no installation key could verify") {
		t.Fatalf("error = %v, want authentication failure", err)
	}
	assertFileBytes(t, path, original)
}

func TestMalformedAuthenticatedCiphertextLeavesStoreUntouched(t *testing.T) {
	dir := useStorageDir(t)
	store, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings")
	parsed := readEnvelopeForTest(t, path)
	encoded := strings.TrimPrefix(parsed.Data, gcmPayloadPrefix)
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	parsed.Data = gcmPayloadPrefix + base64.RawURLEncoding.EncodeToString(ciphertext)
	corrupted, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, corrupted, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMapStore("settings", true); err == nil || !strings.Contains(err.Error(), "no installation key could verify") {
		t.Fatalf("error = %v, want tag verification failure", err)
	}
	assertFileBytes(t, path, corrupted)
}

func TestAuthenticatedCiphertextCannotBeSubstitutedAcrossStores(t *testing.T) {
	dir := useStorageDir(t)
	for name, value := range map[string]string{
		"GSSettings":    "runtime",
		"GSWebSettings": "web",
	} {
		store, err := OpenMapStore(name, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update("owner", value); err != nil {
			t.Fatal(err)
		}
	}

	runtimeEnvelope := readEnvelopeForTest(t, filepath.Join(dir, "GSSettings"))
	webPath := filepath.Join(dir, "GSWebSettings")
	webEnvelope := readEnvelopeForTest(t, webPath)
	webEnvelope.Data = runtimeEnvelope.Data
	substituted, err := json.Marshal(webEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(webPath, substituted, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := OpenMapStore("GSWebSettings", true); err == nil || !strings.Contains(err.Error(), "no installation key could verify") {
		t.Fatalf("error = %v, want cross-store authentication failure", err)
	}
	assertFileBytes(t, webPath, substituted)
}

func TestEncryptedStoreRejectsPlaintextDowngradeWithoutModification(t *testing.T) {
	dir := useStorageDir(t)
	path := filepath.Join(dir, "settings")
	downgraded := []byte(`{"data":"{\"admin\":\"attacker\"}","encrypted":"false"}`)
	if err := os.WriteFile(path, downgraded, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenMapStore("settings", true); err == nil || !strings.Contains(err.Error(), "plaintext envelope") {
		t.Fatalf("error = %v, want plaintext downgrade rejection", err)
	}
	assertFileBytes(t, path, downgraded)
	if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("downgrade load unexpectedly created a key: %v", err)
	}
}

func TestConcurrentProcessesCreateOneInstallationKey(t *testing.T) {
	dir := useStorageDir(t)
	gate := filepath.Join(dir, "start")
	const processCount = 12
	commands := make([]*exec.Cmd, 0, processCount)
	for i := 0; i < processCount; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestInstallationKeyConcurrentCreatorHelper$")
		cmd.Env = append(os.Environ(),
			"GATESENTRY_KEY_HELPER=1",
			"GATESENTRY_KEY_HELPER_DIR="+dir,
			fmt.Sprintf("GATESENTRY_KEY_HELPER_STORE=store-%d", i),
			"GATESENTRY_KEY_HELPER_GATE="+gate,
		)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	if err := os.WriteFile(gate, []byte("start"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("creator process %d: %v", i, err)
		}
	}
	for i := 0; i < processCount; i++ {
		store, err := OpenMapStore(fmt.Sprintf("store-%d", i), true)
		if err != nil {
			t.Fatalf("open store %d with installed key: %v", i, err)
		}
		if got := mustGet(t, store, "creator"); got != fmt.Sprint(i) {
			t.Fatalf("store %d creator = %q", i, got)
		}
	}
}

func TestInstallationKeyConcurrentCreatorHelper(t *testing.T) {
	if os.Getenv("GATESENTRY_KEY_HELPER") != "1" {
		return
	}
	dir := os.Getenv("GATESENTRY_KEY_HELPER_DIR")
	SetBaseDir(dir + string(os.PathSeparator))
	gate := os.Getenv("GATESENTRY_KEY_HELPER_GATE")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(gate); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for creator gate")
		}
		time.Sleep(time.Millisecond)
	}
	store, err := OpenMapStore(os.Getenv("GATESENTRY_KEY_HELPER_STORE"), true)
	if err != nil {
		t.Fatal(err)
	}
	creator := strings.TrimPrefix(os.Getenv("GATESENTRY_KEY_HELPER_STORE"), "store-")
	if err := store.Update("creator", creator); err != nil {
		t.Fatal(err)
	}
}

func TestManagedInstallationKeyFile(t *testing.T) {
	dir := useStorageDir(t)
	managedDir := t.TempDir()
	managedPath := filepath.Join(managedDir, "managed.key")
	key := make([]byte, keySize)
	for i := range key {
		key[i] = byte(255 - i)
	}
	if err := os.WriteFile(managedPath, key, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(managedKeyFileEnv, managedPath)
	store, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("key", "managed"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, installationKeyFilename)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("default key file unexpectedly created: %v", err)
	}
	reopened, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, reopened, "key"); got != "managed" {
		t.Fatalf("key = %q", got)
	}
}

func TestManagedInstallationKeyRejectsOverPermissiveFile(t *testing.T) {
	useStorageDir(t)
	path := filepath.Join(t.TempDir(), "managed.key")
	if err := os.WriteFile(path, make([]byte, keySize), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(managedKeyFileEnv, path)
	store, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("key", "value"); err == nil || !strings.Contains(err.Error(), "group and other access") {
		t.Fatalf("error = %v, want permission rejection", err)
	}
}

func TestRotationSurvivesInterruptionBetweenStores(t *testing.T) {
	dir := useStorageDir(t)
	for _, name := range []string{"GSSettings", "GSWebSettings"} {
		store, err := OpenMapStore(name, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Update("name", name); err != nil {
			t.Fatal(err)
		}
	}

	oldRename := renameFile
	renameFile = func(source, destination string) error {
		if destination == filepath.Join(dir, "GSWebSettings") {
			return errors.New("simulated crash before second store commit")
		}
		return oldRename(source, destination)
	}
	err := RotateInstallationKey("GSSettings", "GSWebSettings")
	renameFile = oldRename
	if err == nil || !strings.Contains(err.Error(), "simulated crash") {
		t.Fatalf("rotation error = %v", err)
	}
	for _, name := range []string{"GSSettings", "GSWebSettings"} {
		store, openErr := OpenMapStore(name, true)
		if openErr != nil {
			t.Fatalf("open %s after interruption: %v", name, openErr)
		}
		if got := mustGet(t, store, "name"); got != name {
			t.Fatalf("%s value = %q", name, got)
		}
	}
	if err := RotateInstallationKey("GSSettings", "GSWebSettings"); err != nil {
		t.Fatal(err)
	}
	if err := PrunePreviousInstallationKeys("GSSettings", "GSWebSettings"); err != nil {
		t.Fatal(err)
	}
	keys, err := readInstallationKeys(filepath.Join(dir, installationKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys.previous) != 0 {
		t.Fatalf("previous keys retained after prune: %d", len(keys.previous))
	}
}

func TestRotationSupportsNestedStore(t *testing.T) {
	dir := useStorageDir(t)
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join("nested", "settings")
	store, err := OpenMapStore(name, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("key", "value"); err != nil {
		t.Fatal(err)
	}
	if err := RotateInstallationKey(name); err != nil {
		t.Fatal(err)
	}
	if err := PrunePreviousInstallationKeys(name); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenMapStore(name, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, reopened, "key"); got != "value" {
		t.Fatalf("key = %q, want value", got)
	}
}

func TestConcurrentUpdatesAfterLegacyMigration(t *testing.T) {
	dir := useStorageDir(t)
	path := filepath.Join(dir, "settings")
	writeLegacyStore(t, path, []byte(`{}`))
	store, err := OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	const count = 24
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.Update(fmt.Sprintf("key-%d", i), fmt.Sprint(i)); err != nil {
				t.Errorf("update: %v", err)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < count; i++ {
		if got := mustGet(t, store, fmt.Sprintf("key-%d", i)); got != fmt.Sprint(i) {
			t.Fatalf("key-%d = %q", i, got)
		}
	}
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("storage file changed after failed load")
	}
}
