package gatesentry2filters

import (
	"strings"
)

// AIEnvBindings maps GSSettings keys to process environment variables used
// to seed empty values (docker compose, systemd, .env). The admin UI can
// change the stored value afterward; a later start does not overwrite a
// non-empty setting.
var AIEnvBindings = []struct {
	Setting string
	Env     string
}{
	{Setting: "ai_grok_api_key", Env: "GS_AI_GROK_API_KEY"},
	{Setting: "ai_openai_api_key", Env: "GS_AI_OPENAI_API_KEY"},
	{Setting: "ai_local_llm_url", Env: "GS_AI_OLLAMA_URL"},
	{Setting: "ai_local_llm_model", Env: "GS_AI_OLLAMA_MODEL"},
	{Setting: "ai_grok_model", Env: "GS_AI_GROK_MODEL"},
	{Setting: "ai_openai_model", Env: "GS_AI_OPENAI_MODEL"},
	{Setting: "ai_image_filtering_mode", Env: "GS_AI_IMAGE_FILTERING_MODE"},
}

// EnvLooksUnset is true for empty or placeholder secrets (CHANGE_ME, etc.).
func EnvLooksUnset(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	u := strings.ToUpper(v)
	switch {
	case u == "CHANGE_ME", u == "CHANGEME", u == "TODO", u == "PLACEHOLDER":
		return true
	case strings.HasPrefix(u, "CHANGE_ME"), strings.HasPrefix(u, "YOUR-"), strings.HasPrefix(u, "PASTE_"):
		return true
	}
	return false
}

// SeedEmpty copies env into current only when current is blank and env is a real value.
func SeedEmpty(current, env string) (string, bool) {
	if strings.TrimSpace(current) != "" {
		return current, false
	}
	if EnvLooksUnset(env) {
		return current, false
	}
	return strings.TrimSpace(env), true
}

// ApplyAIEnvSeeds writes env presets into empty settings. getenv may be os.Getenv.
func ApplyAIEnvSeeds(get func(string) string, set func(string, string), getenv func(string) string) []string {
	if get == nil || set == nil || getenv == nil {
		return nil
	}
	var seeded []string
	for _, b := range AIEnvBindings {
		next, ok := SeedEmpty(get(b.Setting), getenv(b.Env))
		if !ok {
			continue
		}
		if b.Setting == "ai_image_filtering_mode" {
			switch strings.ToLower(next) {
			case AIModeDisabled, AIModeGrok, AIModeChatGPT, AIModeLocal:
				next = strings.ToLower(next)
			default:
				continue
			}
		}
		set(b.Setting, next)
		seeded = append(seeded, b.Setting)
		if b.Setting == "ai_image_filtering_mode" && next != AIModeDisabled {
			set("enable_ai_image_filtering", "true")
		}
	}
	return seeded
}
