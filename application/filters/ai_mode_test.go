package gatesentry2filters

import "testing"

func TestNormalizeAIImageMode(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		enableLegacy string
		scannerURL   string
		want         string
	}{
		{name: "empty is disabled", want: AIModeDisabled},
		{name: "explicit disabled", mode: "disabled", enableLegacy: "true", scannerURL: "http://scanner", want: AIModeDisabled},
		{name: "grok wins over legacy", mode: "Grok", enableLegacy: "true", scannerURL: "http://scanner", want: AIModeGrok},
		{name: "chatgpt", mode: " chatgpt ", want: AIModeChatGPT},
		{name: "local llm", mode: "local", want: AIModeLocal},
		{name: "local wins over legacy", mode: "local", enableLegacy: "true", scannerURL: "http://127.0.0.1:11434", want: AIModeLocal},
		{name: "legacy when old toggle and url", enableLegacy: "true", scannerURL: "http://127.0.0.1:5000", want: AIModeLegacy},
		{name: "old toggle without url", enableLegacy: "true", want: AIModeDisabled},
		{name: "unknown mode", mode: "claude", want: AIModeDisabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeAIImageMode(tt.mode, tt.enableLegacy, tt.scannerURL)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShouldRunLegacyImageScanner(t *testing.T) {
	if ShouldRunLegacyImageScanner("grok", "true", "http://scanner") {
		t.Fatal("grok must not run the local scanner")
	}
	if ShouldRunLegacyImageScanner("local", "true", "http://127.0.0.1:11434") {
		t.Fatal("local LLM must not run the legacy classifier")
	}
	if !ShouldRunLegacyImageScanner("", "true", "http://scanner") {
		t.Fatal("legacy toggle + URL should run local scanner")
	}
}
