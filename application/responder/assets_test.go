package gatesentry2responder

import (
	"strings"
	"testing"
)

func TestBlockPageAssetsAreUsable(t *testing.T) {
	css := GetCssString()
	if len(css) < 100 || !strings.Contains(css, ".mdl-") {
		t.Fatal("block-page CSS is empty or invalid")
	}

	template := GetTemplate()
	for _, marker := range []string{"<html>", "_title_", "_content_", "_primarystyle_"} {
		if !strings.Contains(template, marker) {
			t.Fatalf("block-page template is missing %q", marker)
		}
	}

	page := BuildResponsePage([]string{"test reason"}, 10)
	for _, unresolved := range []string{"_title_", "_content_", "_primarystyle_", "_mainstyle_", "_colorclass_"} {
		if strings.Contains(page, unresolved) {
			t.Fatalf("rendered block page contains unresolved marker %q", unresolved)
		}
	}
	if !strings.Contains(page, "test reason") || !strings.Contains(page, "data:image/png;base64,") {
		t.Fatal("rendered block page is missing its reason or image asset")
	}
}
