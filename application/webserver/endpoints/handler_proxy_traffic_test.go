package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGSApiProxyTrafficGETReturnsAggregateSnapshot(t *testing.T) {
	started := time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/proxy/traffic", nil)
	GSApiProxyTrafficGET(rr, req, func() (uint64, uint64, time.Time) {
		return 12, 34, started
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["upload_bytes"] != float64(12) || body["download_bytes"] != float64(34) || body["total_bytes"] != float64(46) {
		t.Fatalf("body = %v", body)
	}
	for _, forbidden := range []string{"device", "ip", "url", "domain", "user"} {
		if strings.Contains(rr.Body.String(), forbidden) {
			t.Fatalf("response contains %q: %s", forbidden, rr.Body.String())
		}
	}
}
