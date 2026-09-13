package gatesentry2filters

import "strings"

const (
	AIModeDisabled = "disabled"
	AIModeGrok     = "grok"
	AIModeChatGPT  = "chatgpt"
	AIModeLocal    = "local"
	AIModeLegacy   = "legacy"
)

// NormalizeAIImageMode maps persisted settings to a scanner mode.
//
// Grok and ChatGPT are remote vision APIs. They must not be invoked on the
// proxy request path (see AI_FILTERING_PLAN.md). The legacy local scanner is
// used only when no remote provider is selected and a scanner URL is set.
func NormalizeAIImageMode(mode, enableLegacy, scannerURL string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AIModeGrok:
		return AIModeGrok
	case AIModeChatGPT:
		return AIModeChatGPT
	case AIModeLocal:
		return AIModeLocal
	case AIModeDisabled:
		return AIModeDisabled
	}
	if enableLegacy == "true" && strings.TrimSpace(scannerURL) != "" {
		return AIModeLegacy
	}
	return AIModeDisabled
}

// ShouldRunLegacyImageScanner is true only for the old local HTTP classifier.
func ShouldRunLegacyImageScanner(mode, enableLegacy, scannerURL string) bool {
	return NormalizeAIImageMode(mode, enableLegacy, scannerURL) == AIModeLegacy
}
