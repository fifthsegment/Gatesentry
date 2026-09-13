package gatesentry2filters

import "testing"

func TestEnvLooksUnset(t *testing.T) {
	if !EnvLooksUnset("") || !EnvLooksUnset("CHANGE_ME") || !EnvLooksUnset("  your-key-here") {
		t.Fatal("placeholders must look unset")
	}
	if EnvLooksUnset("xai-real") {
		t.Fatal("real value")
	}
}

func TestSeedEmptyDoesNotClobber(t *testing.T) {
	got, ok := SeedEmpty("saved-in-ui", "from-env")
	if ok || got != "saved-in-ui" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	got, ok = SeedEmpty("", "from-env")
	if !ok || got != "from-env" {
		t.Fatalf("got %q ok=%v", got, ok)
	}
	_, ok = SeedEmpty("", "CHANGE_ME")
	if ok {
		t.Fatal("placeholder must not seed")
	}
}

func TestApplyAIEnvSeeds(t *testing.T) {
	store := map[string]string{
		"ai_grok_api_key":         "",
		"ai_openai_api_key":       "already-set",
		"ai_image_filtering_mode": "disabled",
	}
	env := map[string]string{
		"GS_AI_GROK_API_KEY":         "xai-from-compose",
		"GS_AI_OPENAI_API_KEY":       "sk-from-compose",
		"GS_AI_IMAGE_FILTERING_MODE": "grok",
		"GS_AI_OLLAMA_URL":           "http://127.0.0.1:11434",
		"GS_AI_OLLAMA_MODEL":         "llava",
	}
	seeded := ApplyAIEnvSeeds(
		func(k string) string { return store[k] },
		func(k, v string) { store[k] = v },
		func(k string) string { return env[k] },
	)
	if store["ai_grok_api_key"] != "xai-from-compose" {
		t.Fatalf("grok key = %q", store["ai_grok_api_key"])
	}
	if store["ai_openai_api_key"] != "already-set" {
		t.Fatal("openai key must not be overwritten")
	}
	if store["ai_image_filtering_mode"] != "disabled" {
		t.Fatal("non-empty mode must not be overwritten")
	}
	if store["ai_local_llm_url"] != "http://127.0.0.1:11434" {
		t.Fatalf("ollama url = %q", store["ai_local_llm_url"])
	}
	if len(seeded) == 0 {
		t.Fatal("expected some seeds")
	}

	store["ai_image_filtering_mode"] = ""
	ApplyAIEnvSeeds(
		func(k string) string { return store[k] },
		func(k, v string) { store[k] = v },
		func(k string) string { return env[k] },
	)
	if store["ai_image_filtering_mode"] != "grok" {
		t.Fatalf("mode = %q", store["ai_image_filtering_mode"])
	}
	if store["enable_ai_image_filtering"] != "true" {
		t.Fatal("enable flag")
	}
}
