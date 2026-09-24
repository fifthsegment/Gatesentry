package gatesentryWebserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTailscaleRoutesRequireAuthenticationAndDisableCaching(t *testing.T) {
	settings := authStore(t)
	auth, err := NewAuthManager(settings, "")
	if err != nil {
		t.Fatal(err)
	}
	server := NewGsWeb("/")
	registerTailscaleRoutes(server, authenticationMiddlewareFor(auth), settings)

	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "/api/tailscale/status"},
		{http.MethodPut, "/api/tailscale/config"},
		{http.MethodGet, "/api/tailscale/peers"},
		{http.MethodPut, "/api/devices/device-1/tailscale"},
		{http.MethodDelete, "/api/devices/device-1/tailscale/node-1"},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			recorder := httptest.NewRecorder()
			server.router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q", got)
			}
		})
	}
}
