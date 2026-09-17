package gatesentryWebserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthRouteIsPublicAndMinimal(t *testing.T) {
	for _, basePath := range []string{"/", "/gatesentry"} {
		t.Run(basePath, func(t *testing.T) {
			web := NewGsWeb(basePath)
			web.Get("/health", healthHandler)

			url := "/health"
			if basePath != "/" {
				url = basePath + "/health"
			}
			request := httptest.NewRequest(http.MethodGet, url, nil)
			// A readiness probe must not depend on credentials; an invalid
			// authorization header proves the route sits outside the
			// authentication middleware.
			request.Header.Set("Authorization", "Bearer invalid-token")
			response := httptest.NewRecorder()
			web.router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("GET %s returned %d", url, response.Code)
			}
			want := "{\"status\":\"ok\"}"
			if body := strings.TrimSpace(response.Body.String()); body != want {
				t.Fatalf("health response body = %q", body)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
				t.Fatalf("health Content-Type = %q", contentType)
			}
			if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
				t.Fatalf("health Cache-Control = %q", cacheControl)
			}
		})
	}
}
