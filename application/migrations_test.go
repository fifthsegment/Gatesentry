package gatesentryf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func settingsStore(t *testing.T) (*gatesentry2storage.MapStore, string) {
	t.Helper()
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("GSSettings", true)
	if err != nil {
		t.Fatal(err)
	}
	return store, filepath.Join(dir, "GSSettings")
}

func mustSetting(t *testing.T, store *gatesentry2storage.MapStore, key string) string {
	t.Helper()
	value, err := store.GetE(key)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestMigrateSettingsFreshSetupStampsVersion(t *testing.T) {
	store, _ := settingsStore(t)
	if err := MigrateSettings(store); err != nil {
		t.Fatal(err)
	}
	if got := mustSetting(t, store, SettingsSchemaKey); got != "1" {
		t.Fatalf("schema version = %q, want 1", got)
	}
}

func TestMigrateSettingsLegacyRulesArray(t *testing.T) {
	store, _ := settingsStore(t)
	legacy := `[{"id":"rule-1","name":"legacy","action":"block"}]`
	if err := store.Update("rules", legacy); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSettings(store); err != nil {
		t.Fatal(err)
	}
	got := mustSetting(t, store, "rules")
	if !strings.Contains(got, `"rules":[`) || !strings.Contains(got, `"id":"rule-1"`) {
		t.Fatalf("migrated rules = %s", got)
	}
}

func TestMigrateSettingsRulesListIsIdempotent(t *testing.T) {
	store, _ := settingsStore(t)
	encoded := `{"rules":[{"id":"rule-1","name":"modern","enabled":true,"priority":1,"domain":"example.com","action":"block","mitm_action":"default","block_type":"none","blocked_content_types":[],"url_regex_patterns":[],"description":"","created_at":"","updated_at":""}]}`
	if err := store.Update("rules", encoded); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSettings(store); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSettings(store); err != nil {
		t.Fatal(err)
	}
	if got := mustSetting(t, store, "rules"); got != encoded {
		t.Fatalf("rules changed on idempotent migration: %s", got)
	}
}

func TestMigrateSettingsMissingOptionalKeys(t *testing.T) {
	store, _ := settingsStore(t)
	if err := store.Update("authusers", `[{"user":"admin"}]`); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSettings(store); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"enable_https_filtering", "enable_ai_image_filtering", "timezone"} {
		if got := mustSetting(t, store, key); got != "" {
			t.Fatalf("%q should remain absent before default seeding, got %q", key, got)
		}
	}
}

func TestMigrateSettingsNewerSchemaFailsClosed(t *testing.T) {
	store, _ := settingsStore(t)
	if err := store.Update(SettingsSchemaKey, "99"); err != nil {
		t.Fatal(err)
	}
	err := MigrateSettings(store)
	if err == nil || !strings.Contains(err.Error(), "newer than this release supports") {
		t.Fatalf("error = %v", err)
	}
}

func TestMigrateSettingsInvalidVersionFailsClosed(t *testing.T) {
	store, _ := settingsStore(t)
	if err := store.Update(SettingsSchemaKey, "not-a-number"); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSettings(store); err == nil {
		t.Fatal("expected invalid schema version error")
	}
}

func TestMigrateSettingsCorruptedRulesFailClosed(t *testing.T) {
	store, path := settingsStore(t)
	if err := store.Update("rules", `[{"id":`); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateSettings(store); err == nil {
		t.Fatal("expected malformed rules migration error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed migration must leave the settings file byte-identical")
	}
}

func TestMigrateSettingsPreservesCredentialsAndDisabledFeatures(t *testing.T) {
	store, _ := settingsStore(t)
	users := `[{"user":"admin","password":"secret","Base64String":"c2VjcmV0"}]`
	if err := store.UpdateValues(map[string]string{
		"authusers":                 users,
		"rules":                     `[{"id":"rule-1","name":"legacy","action":"block"}]`,
		"enable_https_filtering":    "false",
		"enable_ai_image_filtering": "false",
		"enable_dns_server":         "true",
	}); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSettings(store); err != nil {
		t.Fatal(err)
	}
	if got := mustSetting(t, store, "authusers"); got != users {
		t.Fatalf("authusers changed: %s", got)
	}
	if got := mustSetting(t, store, "enable_https_filtering"); got != "false" {
		t.Fatalf("https filtering = %q", got)
	}
	if got := mustSetting(t, store, "enable_ai_image_filtering"); got != "false" {
		t.Fatalf("AI filtering = %q", got)
	}
}
