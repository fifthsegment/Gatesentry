package gatesentryWebserverEndpoints

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	"github.com/gorilla/mux"
)

// TestPauseCreateListRevoke exercises the admin CRUD lifecycle and confirms a
// revoked pause is audited but no longer suppresses the block.
func TestPauseCreateListRevoke(t *testing.T) {
	svc, cleanup := policyTestService(t)
	defer cleanup()

	// Create a block group so the pause has something to suppress.
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	// Create a pause.
	body := jsonBody(t, map[string]string{
		"scope":     "device",
		"device_id": "device-1",
		"duration":  "1h",
		"reason":    "homework break",
	})
	req := withAdmin(newJSONRequest(http.MethodPost, "/api/pauses", body), "admin")
	rec := httptest.NewRecorder()
	GSApiPauseCreate(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	created := decodeMap(t, rec.Body.Bytes())
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("create response missing id: %s", rec.Body.String())
	}
	if created["scope"] != "device" {
		t.Fatalf("scope = %v", created["scope"])
	}
	if created["active"] != true {
		t.Fatalf("active = %v", created["active"])
	}

	// List pauses.
	req = newJSONRequest(http.MethodGet, "/api/pauses", nil)
	rec = httptest.NewRecorder()
	GSApiPausesGET(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	listed := decodeMap(t, rec.Body.Bytes())
	pauses, _ := listed["pauses"].([]interface{})
	if len(pauses) != 1 {
		t.Fatalf("pauses = %d, want 1", len(pauses))
	}

	// Revoke.
	req = withAdmin(mux.SetURLVars(newJSONRequest(http.MethodDelete, "/api/pauses/"+id, nil), map[string]string{"id": id}), "admin")
	rec = httptest.NewRecorder()
	GSApiPauseRevoke(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// List again — still 1 pause (revoked, audited).
	req = newJSONRequest(http.MethodGet, "/api/pauses", nil)
	rec = httptest.NewRecorder()
	GSApiPausesGET(rec, req)
	listed = decodeMap(t, rec.Body.Bytes())
	pauses, _ = listed["pauses"].([]interface{})
	if len(pauses) != 1 {
		t.Fatalf("pauses after revoke = %d, want 1 (audited)", len(pauses))
	}
}

func TestPauseCreateRequiresAuth(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	body := jsonBody(t, map[string]string{
		"scope":    "installation",
		"duration": "30m",
	})
	req := newJSONRequest(http.MethodPost, "/api/pauses", body)
	rec := httptest.NewRecorder()
	GSApiPauseCreate(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestPauseCreateRejectsBadInput(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	body := jsonBody(t, map[string]string{
		"scope":    "bogus",
		"duration": "30m",
	})
	req := withAdmin(newJSONRequest(http.MethodPost, "/api/pauses", body), "admin")
	rec := httptest.NewRecorder()
	GSApiPauseCreate(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPauseRevokeNotFound(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	req := withAdmin(mux.SetURLVars(newJSONRequest(http.MethodDelete, "/api/pauses/nonexistent", nil), map[string]string{"id": "nonexistent"}), "admin")
	rec := httptest.NewRecorder()
	GSApiPauseRevoke(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSchedulePresetsList(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	req := newJSONRequest(http.MethodGet, "/api/policy/schedule-presets", nil)
	rec := httptest.NewRecorder()
	GSApiSchedulePresetsGET(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	resp := decodeMap(t, rec.Body.Bytes())
	presets, _ := resp["presets"].([]interface{})
	if len(presets) < 2 {
		t.Fatalf("presets = %d, want at least 2", len(presets))
	}
}

func TestSchedulePresetApply(t *testing.T) {
	svc, cleanup := policyTestService(t)
	defer cleanup()

	// Create a group to apply the preset to.
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	applyBody, _ := json.Marshal(map[string]string{
		"preset_id": "bedtime",
		"timezone":  "America/New_York",
		"group_id":  "kids",
	})
	req := withAdmin(newJSONRequest(http.MethodPost, "/api/policy/schedule-presets/apply", applyBody), "admin")
	rec := httptest.NewRecorder()
	GSApiSchedulePresetApply(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	resp := decodeMap(t, rec.Body.Bytes())
	schedule, _ := resp["schedule"].(map[string]interface{})
	if schedule == nil {
		t.Fatalf("schedule missing: %s", rec.Body.String())
	}
	if schedule["preset"] != "bedtime" {
		t.Fatalf("preset = %v, want bedtime", schedule["preset"])
	}

	// Applying a preset adds a rule that blocks all traffic in its window.
	group := svc.Snapshot().Groups["kids"]
	if len(group.Rules) != 1 {
		t.Fatalf("rules after apply = %+v, want one scheduled rule", group.Rules)
	}
	rule := group.Rules[0]
	if !rule.Target.AllTraffic || rule.Action != gatesentryPolicy.ActionBlock || rule.Schedule == nil || rule.Schedule.Preset != "bedtime" {
		t.Fatalf("rule = %+v, want an all-traffic block on the bedtime schedule", rule)
	}
}

func TestSchedulePresetApplyUnknownGroup(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	applyBody, _ := json.Marshal(map[string]string{
		"preset_id": "bedtime",
		"timezone":  "America/New_York",
		"group_id":  "nonexistent",
	})
	req := withAdmin(newJSONRequest(http.MethodPost, "/api/policy/schedule-presets/apply", applyBody), "admin")
	rec := httptest.NewRecorder()
	GSApiSchedulePresetApply(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSchedulePresetApplyUnknownPreset(t *testing.T) {
	svc, cleanup := policyTestService(t)
	defer cleanup()

	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	applyBody, _ := json.Marshal(map[string]string{
		"preset_id": "nonexistent",
		"timezone":  "America/New_York",
		"group_id":  "kids",
	})
	req := withAdmin(newJSONRequest(http.MethodPost, "/api/policy/schedule-presets/apply", applyBody), "admin")
	rec := httptest.NewRecorder()
	GSApiSchedulePresetApply(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// Ensure the unused import for context doesn't trigger (withAdmin uses it).
var _ = context.Background
