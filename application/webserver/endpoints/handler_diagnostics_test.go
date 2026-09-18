package gatesentryWebserverEndpoints

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatesentryDiagnostics "bitbucket.org/abdullah_irfan/gatesentryf/diagnostics"
)

func TestGSApiDiagnosticsGETReturnsReport(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	GSApiDiagnosticsGET(rr, req, DiagnosticsDeps{Diagnostics: gatesentryDiagnostics.Deps{Listener: func(context.Context, string, string) error { return nil }, UpstreamCheck: func(context.Context, string) error { return nil }}})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"listeners"`) {
		t.Fatalf("missing listener check: %s", rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
}

func TestGSApiDiagnosticsBundleGETRequiresStore(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/bundle", nil)
	GSApiDiagnosticsBundleGET(rr, req, DiagnosticsDeps{})
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}
