package gatesentryWebserverFrontend

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedDashboardAssets(t *testing.T) {
	index := GetIndexHtml()
	if len(index) == 0 || !strings.Contains(strings.ToLower(string(index)), "<html") {
		t.Fatal("embedded dashboard index is empty or invalid")
	}
	for _, reference := range []string{"/fs/bundle.js", "/vite.svg"} {
		if !strings.Contains(string(index), reference) {
			t.Fatalf("embedded dashboard index is missing asset reference %q", reference)
		}
	}

	assets := []string{"index.html", "fs/bundle.js", "vite.svg"}
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

func TestEmbeddedBlockPageMaterialStylesheet(t *testing.T) {
	css := GetBlockPageMaterialUIStylesheet()
	if len(css) < 100 || !strings.Contains(string(css), ".mdl-") {
		t.Fatal("embedded block-page Material stylesheet is empty or invalid")
	}
}

func TestEmbeddedIndexAssetsAreServedAtReferencedURLs(t *testing.T) {
	index := string(GetIndexHtml())
	assetReference := regexp.MustCompile(`(?:src|href)=["'](/[^"']+)["']`)
	matches := assetReference.FindAllStringSubmatch(index, -1)
	if len(matches) == 0 {
		t.Fatal("embedded dashboard index contains no root-relative assets")
	}

	handler := GetAssetHandler()
	for _, match := range matches {
		assetURL := match[1]
		t.Run(assetURL, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, assetURL, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d", assetURL, response.Code)
			}
			if response.Body.Len() == 0 {
				t.Fatalf("GET %s returned an empty body", assetURL)
			}
		})
	}
}
