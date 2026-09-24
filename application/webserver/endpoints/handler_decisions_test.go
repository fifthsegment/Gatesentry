package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
)

func newDecisionsTestLogger(t *testing.T) *gatesentryLogger.Log {
	t.Helper()
	l := gatesentryLogger.NewLogger(t.TempDir() + "/decisions.db")
	t.Cleanup(func() { _ = l.Database.Close() })
	return l
}

// seedDecisions writes a small set of decisions covering actions, layers,
// groups, IPs, and domains so the handlers can exercise filters.
func seedDecisions(t *testing.T, l *gatesentryLogger.Log) {
	t.Helper()
	now := time.Now().Unix()
	seed := func(action gatesentryPolicy.DecisionAction, layer gatesentryPolicy.DecisionLayer, domain, ip, group, rule, reason string, ts int64) {
		d := gatesentryPolicy.NewDecision(action, layer, domain)
		d.URL = domain
		d.ClientIP = ip
		d.GroupID = group
		d.MatchedRule = rule
		d.Reason = reason
		d.DeviceID = "secret-device-id"
		d.Timestamp = time.Unix(ts, 0)
		l.LogDecision(d)
	}
	seed(gatesentryPolicy.ActionDecisionBlock, gatesentryPolicy.LayerDNS, "ads.example", "192.0.2.10", "kids", "blocklist", "global blocklist", now-10)
	seed(gatesentryPolicy.ActionDecisionBlock, gatesentryPolicy.LayerDNS, "ads.example", "192.0.2.10", "kids", "blocklist", "global blocklist", now-5)
	seed(gatesentryPolicy.ActionDecisionBlock, gatesentryPolicy.LayerExplicitProxy, "tracker.example", "192.0.2.20", "guests", "url filter", "blocked URL", now-8)
	seed(gatesentryPolicy.ActionDecisionAllow, gatesentryPolicy.LayerDNS, "safe.example", "192.0.2.10", "kids", "", "no matching rule", now-3)
	seed(gatesentryPolicy.ActionDecisionError, gatesentryPolicy.LayerContent, "broken.example", "192.0.2.20", "", "filter", "filter error", now-1)
	time.Sleep(150 * time.Millisecond)
}

// TestDecisionsGETRedactsDeviceID verifies that a single decision record in
// the list response never carries the stable DeviceID. The field is dropped
// at the API boundary so one user's browsing history cannot be correlated to
// a device by another household member reading the decisions feed.
func TestDecisionsGETRedactsDeviceID(t *testing.T) {
	l := newDecisionsTestLogger(t)
	seedDecisions(t, l)

	req := httptest.NewRequest(http.MethodGet, "/api/decisions?from=0", nil)
	rec := httptest.NewRecorder()
	GSApiDecisionsGET(rec, req, l)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "device_id") {
		t.Fatalf("response leaked device_id: %s", rec.Body.String())
	}
	var body struct {
		Items []map[string]interface{} `json:"items"`
		Total int                      `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v, body=%s", err, rec.Body.String())
	}
	if body.Total == 0 {
		t.Fatalf("expected decisions, got total=%d", body.Total)
	}
}

func TestDecisionsGETFiltersByAction(t *testing.T) {
	l := newDecisionsTestLogger(t)
	seedDecisions(t, l)

	req := httptest.NewRequest(http.MethodGet, "/api/decisions?from=0&action="+string(gatesentryPolicy.ActionDecisionBlock), nil)
	rec := httptest.NewRecorder()
	GSApiDecisionsGET(rec, req, l)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			Action string `json:"action"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 3 {
		t.Fatalf("got %d block items, want 3", len(body.Items))
	}
	for _, it := range body.Items {
		if it.Action != string(gatesentryPolicy.ActionDecisionBlock) {
			t.Fatalf("non-block item: %+v", it)
		}
	}
}

func TestDecisionsGETPaginates(t *testing.T) {
	l := newDecisionsTestLogger(t)
	seedDecisions(t, l)

	// page of 2, skip 0
	req := httptest.NewRequest(http.MethodGet, "/api/decisions?from=0&limit=2&offset=0", nil)
	rec := httptest.NewRecorder()
	GSApiDecisionsGET(rec, req, l)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var p1 struct {
		Items []struct {
			Time int64 `json:"time"`
		} `json:"items"`
	}
	json.Unmarshal(rec.Body.Bytes(), &p1)
	if len(p1.Items) != 2 {
		t.Fatalf("page 1 size = %d, want 2", len(p1.Items))
	}

	// next page: offset 2
	req2 := httptest.NewRequest(http.MethodGet, "/api/decisions?from=0&limit=2&offset=2", nil)
	rec2 := httptest.NewRecorder()
	GSApiDecisionsGET(rec2, req2, l)
	var p2 struct {
		Items []struct {
			Time int64 `json:"time"`
		} `json:"items"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &p2)
	if len(p2.Items) != 2 {
		t.Fatalf("page 2 size = %d, want 2", len(p2.Items))
	}
	if p1.Items[0].Time == p2.Items[0].Time {
		t.Fatalf("pagination did not advance: both pages start at %d", p1.Items[0].Time)
	}
}

func TestDecisionsGETRejectsBadLimit(t *testing.T) {
	l := newDecisionsTestLogger(t)
	req := httptest.NewRequest(http.MethodGet, "/api/decisions?limit=abc", nil)
	rec := httptest.NewRecorder()
	GSApiDecisionsGET(rec, req, l)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestDecisionsGETNilLoggerReturns503(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/decisions", nil)
	rec := httptest.NewRecorder()
	GSApiDecisionsGET(rec, req, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestDecisionSummaryGETReturnsCounts(t *testing.T) {
	l := newDecisionsTestLogger(t)
	seedDecisions(t, l)

	req := httptest.NewRequest(http.MethodGet, "/api/decisions/summary?from=0", nil)
	rec := httptest.NewRecorder()
	GSApiDecisionSummaryGET(rec, req, l)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Total              int            `json:"total"`
		ByAction           map[string]int `json:"by_action"`
		InspectionFailures int            `json:"inspection_failures"`
		AffectedDevices    []struct {
			IP     string `json:"ip"`
			Device string `json:"device"`
			Count  int    `json:"count"`
		} `json:"affected_devices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v, body=%s", err, rec.Body.String())
	}
	if body.Total != 5 {
		t.Fatalf("total = %d, want 5", body.Total)
	}
	if body.ByAction[string(gatesentryPolicy.ActionDecisionBlock)] != 3 {
		t.Fatalf("block count = %d, want 3", body.ByAction[string(gatesentryPolicy.ActionDecisionBlock)])
	}
	if body.InspectionFailures != 1 {
		t.Fatalf("inspection failures = %d, want 1", body.InspectionFailures)
	}
	if len(body.AffectedDevices) < 2 {
		t.Fatalf("affected devices = %+v, want at least 2", body.AffectedDevices)
	}
	// 192.0.2.10 has 3 decisions; it must be the most affected.
	if body.AffectedDevices[0].IP != "192.0.2.10" || body.AffectedDevices[0].Count != 3 {
		t.Fatalf("top affected = %+v, want 192.0.2.10 (3)", body.AffectedDevices[0])
	}
	// Without a discovery match, the observed IPv4 address is the label.
	if body.AffectedDevices[0].Device != "192.0.2.10" {
		t.Fatalf("device name = %q, want IPv4 fallback", body.AffectedDevices[0].Device)
	}
}

func TestDecisionSummaryGETNilLoggerReturns503(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/decisions/summary", nil)
	rec := httptest.NewRecorder()
	GSApiDecisionSummaryGET(rec, req, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
