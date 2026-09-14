package gatesentryWebserver

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	gatesentryWebserverFrontend "bitbucket.org/abdullah_irfan/gatesentryf/webserver/frontend"
)

func TestDashboardIndexAssetURLsAreServedByRuntimeRouter(t *testing.T) {
	assetReference := regexp.MustCompile("(?:src|href)=[\"'](/[^\"']+)[\"']")
	matches := assetReference.FindAllStringSubmatch(string(gatesentryWebserverFrontend.GetIndexHtml()), -1)
	if len(matches) == 0 {
		t.Fatal("embedded dashboard index contains no root-relative assets")
	}

	for _, basePath := range []string{"/", "/gatesentry"} {
		t.Run(basePath, func(t *testing.T) {
			web := NewGsWeb(basePath)
			registerDashboardAssetRoutes(web.router)
			for _, match := range matches {
				assetURL := match[1]
				t.Run(assetURL, func(t *testing.T) {
					response := httptest.NewRecorder()
					web.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, assetURL, nil))
					if response.Code != http.StatusOK {
						t.Fatalf("GET %s returned %d", assetURL, response.Code)
					}
					if response.Body.Len() == 0 {
						t.Fatalf("GET %s returned an empty body", assetURL)
					}
				})
			}
		})
	}
}
