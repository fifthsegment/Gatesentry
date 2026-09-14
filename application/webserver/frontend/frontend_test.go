package gatesentryWebserverFrontend

import (
	"io"
	"strings"
	"testing"
)

func TestEmbeddedDashboardAssets(t *testing.T) {
	index := GetIndexHtml()
	if len(index) == 0 || !strings.Contains(strings.ToLower(string(index)), "<html") {
		t.Fatal("embedded dashboard index is empty or invalid")
	}

	assets := []string{"index.html", "fs/bundle.js", "fs/style.css"}
	for _, name := range assets {
		file, err := GetFSHandler().Open(name)
		if err != nil {
			t.Fatalf("open embedded dashboard asset %q: %v", name, err)
		}
		contents, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			t.Fatalf("read embedded dashboard asset %q: %v", name, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close embedded dashboard asset %q: %v", name, closeErr)
		}
		if len(contents) == 0 {
			t.Fatalf("embedded dashboard asset %q is empty", name)
		}
	}
}
