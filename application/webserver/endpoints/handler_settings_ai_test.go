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

func TestAISettingsGetAndPost(t *testing.T) {
	store, cleanup := setupSettingsTestStore(t)
	defer cleanup()

	if v := settingValue(t, GSApiSettingsGET("ai_image_filtering_mode", store)); v != "disabled" {
		t.Fatalf("empty mode GET = %q, want disabled", v)
	}

	store.Update("ai_image_filtering_mode", "disabled")
	store.Update("ai_grok_api_key", "xai-test")
	store.Update("ai_openai_api_key", "sk-test")

	for _, key := range []string{"ai_image_filtering_mode", "ai_grok_api_key", "ai_openai_api_key", "ai_local_llm_url"} {
		got := GSApiSettingsGET(key, store)
		if got == nil {
			t.Fatalf("GET %s returned nil (not whitelisted)", key)
		}
	}

	if v := settingValue(t, GSApiSettingsGET("ai_image_filtering_mode", store)); v != "disabled" {
		t.Fatalf("mode = %q", v)
	}

	posted := GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "Grok",
	})
	if v := settingValue(t, posted); v != "grok" {
		t.Fatalf("POST mode returned %q", v)
	}
	if store.Get("ai_image_filtering_mode") != "grok" {
		t.Fatalf("stored mode = %q", store.Get("ai_image_filtering_mode"))
	}
	if store.Get("enable_ai_image_filtering") != "true" {
		t.Fatal("expected enable_ai_image_filtering synced to true")
	}

	posted = GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "disabled",
	})
	if settingValue(t, posted) != "disabled" {
		t.Fatal("disable POST failed")
	}
	if store.Get("enable_ai_image_filtering") != "false" {
		t.Fatal("expected enable_ai_image_filtering synced to false")
	}

	bad := GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "claude",
	})
	if v := settingValue(t, bad); !strings.HasPrefix(v, "ERROR:") {
		t.Fatalf("expected ERROR for invalid mode, got %q", v)
	}
	if store.Get("ai_image_filtering_mode") != "disabled" {
		t.Fatal("invalid mode must not persist")
	}

	GSApiSettingsPOST("ai_grok_api_key", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_grok_api_key", Value: "xai-new",
	})
	if store.Get("ai_grok_api_key") != "xai-new" {
		t.Fatal("grok key not stored")
	}

	posted = GSApiSettingsPOST("ai_image_filtering_mode", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_image_filtering_mode", Value: "local",
	})
	if settingValue(t, posted) != "local" {
		t.Fatalf("POST local mode returned %q", settingValue(t, posted))
	}
	if store.Get("enable_ai_image_filtering") != "true" {
		t.Fatal("expected enable_ai_image_filtering true for local LLM")
	}

	GSApiSettingsPOST("ai_local_llm_url", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_local_llm_url", Value: "http://127.0.0.1:11434",
	})
	if store.Get("ai_local_llm_url") != "http://127.0.0.1:11434" {
		t.Fatal("local LLM URL not stored")
	}

	badURL := GSApiSettingsPOST("ai_local_llm_url", store, gatesentryWebserverTypes.Datareceiver{
		Key: "ai_local_llm_url", Value: "not-a-url",
	})
	if v := settingValue(t, badURL); !strings.HasPrefix(v, "ERROR:") {
		t.Fatalf("expected ERROR for invalid local LLM URL, got %q", v)
	}
	if store.Get("ai_local_llm_url") != "http://127.0.0.1:11434" {
		t.Fatal("invalid URL must not replace the stored value")
	}
}
