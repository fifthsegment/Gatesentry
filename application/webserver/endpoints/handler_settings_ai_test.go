package gatesentryWebserverEndpoints

import (
	"os"
	"strings"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

func setupSettingsTestStore(t *testing.T) (*gatesentry2storage.MapStore, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "aisettings-*")
	if err != nil {
		t.Fatal(err)
	}
	orig := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(tmpDir + "/")
	store := gatesentry2storage.NewMapStore("test_ai_settings", false)
	return store, func() {
		gatesentry2storage.SetBaseDir(orig)
		os.RemoveAll(tmpDir)
	}
}

func settingValue(t *testing.T, got interface{}) string {
	t.Helper()
	switch v := got.(type) {
	case struct {
		Key   string
		Value string
	}:
		return v.Value
	case gatesentryWebserverTypes.Datareceiver:
		return v.Value
	default:
		t.Fatalf("unexpected type %T", got)
		return ""
	}
}

func getSetting(t *testing.T, key string, store *gatesentry2storage.MapStore) interface{} {
	t.Helper()
	got, err := GSApiSettingsGET(key, store)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAISettingsGetAndPost(t *testing.T) {
	store, cleanup := setupSettingsTestStore(t)
	defer cleanup()

	if v := settingValue(t, getSetting(t, "ai_image_filtering_mode", store)); v != "disabled" {
		t.Fatalf("empty mode GET = %q, want disabled", v)
	}
	store.Update("ai_image_filtering_mode", "disabled")
	store.Update("ai_grok_api_key", "xai-test")
	store.Update("ai_openai_api_key", "sk-test")
	masked := getSetting(t, "ai_grok_api_key", store).(struct {
		Key        string
		Configured bool
	})
	if !masked.Configured || masked.Key != "ai_grok_api_key" {
		t.Fatalf("masked key response = %+v", masked)
	}

	for _, key := range []string{"ai_image_filtering_mode", "ai_grok_api_key", "ai_openai_api_key", "ai_local_llm_url"} {
		got := getSetting(t, key, store)
		if got == nil {
			t.Fatalf("GET %s returned nil (not whitelisted)", key)
		}
	}

	if v := settingValue(t, getSetting(t, "ai_image_filtering_mode", store)); v != "disabled" {
		t.Fatalf("mode = %q", v)
	}

	posted, err := GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "Grok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := settingValue(t, posted); v != "grok" {
		t.Fatalf("POST mode returned %q", v)
	}
	if store.GetOrDefault("ai_image_filtering_mode", "") != "grok" {
		t.Fatalf("stored mode = %q", store.GetOrDefault("ai_image_filtering_mode", ""))
	}
	if store.GetOrDefault("enable_ai_image_filtering", "") != "true" {
		t.Fatal("expected enable_ai_image_filtering synced to true")
	}

	posted, err = GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "disabled",
	})
	if err != nil {
		t.Fatal(err)
	}
	if settingValue(t, posted) != "disabled" {
		t.Fatal("disable POST failed")
	}
	if store.GetOrDefault("enable_ai_image_filtering", "") != "false" {
		t.Fatal("expected enable_ai_image_filtering synced to false")
	}

	bad, err := GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := settingValue(t, bad); !strings.HasPrefix(v, "ERROR:") {
		t.Fatalf("expected ERROR for invalid mode, got %q", v)
	}
	if store.GetOrDefault("ai_image_filtering_mode", "") != "disabled" {
		t.Fatal("invalid mode must not persist")
	}

	_, err = GSApiSettingsPOST("ai_grok_api_key", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_grok_api_key", Value: "xai-new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.GetOrDefault("ai_grok_api_key", "") != "xai-new" {
		t.Fatal("grok key not stored")
	}

	posted, err = GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "local",
	})
	if err != nil {
		t.Fatal(err)
	}
	if settingValue(t, posted) != "local" {
		t.Fatalf("POST local mode returned %q", settingValue(t, posted))
	}
	if store.GetOrDefault("enable_ai_image_filtering", "") != "true" {
		t.Fatal("expected enable_ai_image_filtering true for local LLM")
	}

	_, err = GSApiSettingsPOST("ai_local_llm_url", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_local_llm_url", Value: "http://127.0.0.1:11434",
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.GetOrDefault("ai_local_llm_url", "") != "http://127.0.0.1:11434" {
		t.Fatal("local LLM URL not stored")
	}

	badURL, err := GSApiSettingsPOST("ai_local_llm_url", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_local_llm_url", Value: "not-a-url",
	})
	if err != nil {
		t.Fatal(err)
	}
	if v := settingValue(t, badURL); !strings.HasPrefix(v, "ERROR:") {
		t.Fatalf("expected ERROR for invalid local LLM URL, got %q", v)
	}
	if store.GetOrDefault("ai_local_llm_url", "") != "http://127.0.0.1:11434" {
		t.Fatal("invalid URL must not replace the stored value")
	}
}

func TestSettingsPostReturnsPersistenceFailure(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + "/missing/")
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	output, err := GSApiSettingsPOST("strictness", store, gatesentryWebserverTypes.Datareceiver{Value: "1000"})
	if err == nil {
		t.Fatalf("output = %#v; expected persistence error", output)
	}
}
