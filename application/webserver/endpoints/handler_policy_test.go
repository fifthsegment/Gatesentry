package gatesentryWebserverEndpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func policyTestService(t *testing.T) (*gatesentryPolicy.Service, func()) {
	t.Helper()
	// The DNS server package owns the singleton the handlers read.
	original := gatesentryDnsServer.GetPolicyService()
	dir := t.TempDir()
	oldBase := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(oldBase) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := gatesentryPolicy.NewService(store, staticResolver{})
	if err != nil {
		t.Fatal(err)
	}
	gatesentryDnsServer.SetPolicyServiceForTests(svc)
	return svc, func() { gatesentryDnsServer.SetPolicyServiceForTests(original) }
}

type staticResolver struct{}

func (staticResolver) ResolveDeviceByIP(ip string) (string, bool, bool) {
	if ip == "192.0.2.10" {
		return "device-1", false, false
	}
	return "", false, false
}

func TestPolicyGroupsRoundTrip(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	body := `{"groups":[{"id":"kids","name":"Kids","action":"block","domains":["*.games.example"],"users":["dana"],"priority":1}]}`
	put := httptest.NewRequest(http.MethodPut, "/api/policy/groups", bytes.NewBufferString(body))
	putRecorder := httptest.NewRecorder()
	GSApiPolicyGroupsReplace(putRecorder, put)
	if putRecorder.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", putRecorder.Code, putRecorder.Body.String())
	}
	var putBody struct {
		Groups []gatesentryPolicy.PolicyGroup
	}
	if err := json.Unmarshal(putRecorder.Body.Bytes(), &putBody); err != nil {
		t.Fatal(err)
	}
	if len(putBody.Groups) != 1 || putBody.Groups[0].ID != "kids" {
		t.Fatalf("PUT body = %s", putRecorder.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/policy/groups", nil)
	getRecorder := httptest.NewRecorder()
	GSApiPolicyGroupsGet(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d", getRecorder.Code)
	}
	var getBody struct {
		Groups []gatesentryPolicy.PolicyGroup
	}
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &getBody); err != nil {
		t.Fatal(err)
	}
	if len(getBody.Groups) != 1 || getBody.Groups[0].Action != gatesentryPolicy.ActionBlock {
		t.Fatalf("GET body = %s", getRecorder.Body.String())
	}
}

func TestPolicyAssignmentsRejectUnknownGroup(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	body := `{"assignments":[{"device_id":"device-1","group_id":"missing"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/policy/assignments", bytes.NewBufferString(body))
	recorder := httptest.NewRecorder()
	GSApiPolicyAssignmentsReplace(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPolicyPreviewReportsInapplicableConditions(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	groups := httptest.NewRequest(http.MethodPut, "/api/policy/groups", bytes.NewBufferString(
		`{"groups":[{"id":"kids","action":"block","domains":["*.games.example"]}]}`))
	groupsRecorder := httptest.NewRecorder()
	GSApiPolicyGroupsReplace(groupsRecorder, groups)
	if groupsRecorder.Code != http.StatusOK {
		t.Fatalf("groups status = %d", groupsRecorder.Code)
	}
	assignments := httptest.NewRequest(http.MethodPut, "/api/policy/assignments", bytes.NewBufferString(
		`{"assignments":[{"device_id":"device-1","group_id":"kids"}]}`))
	assignmentsRecorder := httptest.NewRecorder()
	GSApiPolicyAssignmentsReplace(assignmentsRecorder, assignments)
	if assignmentsRecorder.Code != http.StatusOK {
		t.Fatalf("assignments status = %d", assignmentsRecorder.Code)
	}

	preview := httptest.NewRequest(http.MethodPost, "/api/policy/preview", bytes.NewBufferString(
		`{"client_ip":"192.0.2.10","domain":"chess.games.example"}`))
	previewRecorder := httptest.NewRecorder()
	GSApiPolicyPreview(previewRecorder, preview)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var previewBody struct {
		Identity               gatesentryPolicy.Identity
		Action                 gatesentryPolicy.PolicyAction
		InapplicableConditions []string `json:"inapplicable_conditions"`
	}
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if previewBody.Action != gatesentryPolicy.ActionBlock {
		t.Fatalf("preview action = %s, want block", previewBody.Action)
	}
	if len(previewBody.InapplicableConditions) != 3 {
		t.Fatalf("inapplicable conditions = %v, want url_regex/content_type/mitm", previewBody.InapplicableConditions)
	}
}

func TestPolicyEndpointsUnavailableWithoutService(t *testing.T) {
	original := gatesentryDnsServer.GetPolicyService()
	gatesentryDnsServer.SetPolicyServiceForTests(nil)
	defer gatesentryDnsServer.SetPolicyServiceForTests(original)

	req := httptest.NewRequest(http.MethodGet, "/api/policy/groups", nil)
	recorder := httptest.NewRecorder()
	GSApiPolicyGroupsGet(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when the policy service is unavailable", recorder.Code)
	}
}
