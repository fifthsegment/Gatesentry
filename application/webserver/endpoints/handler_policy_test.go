package gatesentryWebserverEndpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	"github.com/gorilla/mux"
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
		Identity gatesentryPolicy.Identity
		Active   struct {
			Action                 gatesentryPolicy.PolicyAction
			GroupID                string   `json:"group_id"`
			MatchedDomain          string   `json:"matched_domain"`
			InapplicableConditions []string `json:"inapplicable_conditions"`
			Reason                 string
			Stages                 []struct {
				Name    string
				Applied bool
				Action  gatesentryPolicy.PolicyAction
				Detail  string
			}
		}
		Proposed struct {
			Action                 gatesentryPolicy.PolicyAction
			InapplicableConditions []string `json:"inapplicable_conditions"`
		}
		Changed bool
	}
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if previewBody.Active.Action != gatesentryPolicy.ActionBlock {
		t.Fatalf("preview active action = %s, want block", previewBody.Active.Action)
	}
	if previewBody.Active.GroupID != "kids" {
		t.Fatalf("preview group = %s, want kids", previewBody.Active.GroupID)
	}
	if previewBody.Active.MatchedDomain != "*.games.example" {
		t.Fatalf("preview matched domain = %s, want *.games.example", previewBody.Active.MatchedDomain)
	}
	if len(previewBody.Active.InapplicableConditions) != 3 {
		t.Fatalf("inapplicable conditions = %v, want url_regex/content_type/mitm", previewBody.Active.InapplicableConditions)
	}
	// No proposed policy => proposed equals active and nothing changed.
	if previewBody.Proposed.Action != gatesentryPolicy.ActionBlock {
		t.Fatalf("preview proposed action = %s, want block", previewBody.Proposed.Action)
	}
	if previewBody.Changed {
		t.Fatalf("preview changed = true, want false when no proposed policy is supplied")
	}
	if len(previewBody.Active.Stages) == 0 {
		t.Fatalf("preview stages empty; expected a precedence trail")
	}
}

// TestPolicyPreviewProposedComparison verifies the handler compares a proposed
// policy revision against the active one and reports the change without
// persisting anything.
func TestPolicyPreviewProposedComparison(t *testing.T) {
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

	body := `{"client_ip":"192.0.2.10","domain":"chess.games.example","proposed":{"groups":[{"id":"unrestricted","action":"allow","domains":["*.games.example"]}],"assignments":[{"device_id":"device-1","group_id":"unrestricted"}]}}`
	preview := httptest.NewRequest(http.MethodPost, "/api/policy/preview", bytes.NewBufferString(body))
	previewRecorder := httptest.NewRecorder()
	GSApiPolicyPreview(previewRecorder, preview)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var previewBody struct {
		Active struct {
			Action  gatesentryPolicy.PolicyAction
			GroupID string `json:"group_id"`
		}
		Proposed struct {
			Action  gatesentryPolicy.PolicyAction
			GroupID string `json:"group_id"`
		}
		Changed bool
	}
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if previewBody.Active.Action != gatesentryPolicy.ActionBlock {
		t.Fatalf("active action = %s, want block", previewBody.Active.Action)
	}
	if previewBody.Proposed.Action != gatesentryPolicy.ActionAllow {
		t.Fatalf("proposed action = %s, want allow", previewBody.Proposed.Action)
	}
	if previewBody.Proposed.GroupID != "unrestricted" {
		t.Fatalf("proposed group = %s, want unrestricted", previewBody.Proposed.GroupID)
	}
	if !previewBody.Changed {
		t.Fatalf("changed = false, want true when proposed alters the decision")
	}

	// Active policy must be untouched by the preview.
	get := httptest.NewRequest(http.MethodGet, "/api/policy/groups", nil)
	getRecorder := httptest.NewRecorder()
	GSApiPolicyGroupsGet(getRecorder, get)
	if !strings.Contains(getRecorder.Body.String(), "kids") {
		t.Fatalf("active groups lost kids after preview: %s", getRecorder.Body.String())
	}
	if strings.Contains(getRecorder.Body.String(), "unrestricted") {
		t.Fatalf("proposed group persisted; preview must not write: %s", getRecorder.Body.String())
	}
}

func TestPolicyPreviewRejectsMissingDomain(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	preview := httptest.NewRequest(http.MethodPost, "/api/policy/preview", bytes.NewBufferString(`{"client_ip":"192.0.2.10"}`))
	previewRecorder := httptest.NewRecorder()
	GSApiPolicyPreview(previewRecorder, preview)
	if previewRecorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing domain", previewRecorder.Code)
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

func TestPolicyTemplatesPreviewAndApply(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	preview := httptest.NewRequest(http.MethodGet, "/api/policy/templates", nil)
	previewRecorder := httptest.NewRecorder()
	GSApiPolicyTemplatesGet(previewRecorder, preview)
	if previewRecorder.Code != http.StatusOK {
		t.Fatalf("template preview status = %d, body = %s", previewRecorder.Code, previewRecorder.Body.String())
	}
	var previewBody struct {
		Templates         []gatesentryPolicy.PolicyTemplate `json:"templates"`
		SharedLimitations []string                          `json:"shared_limitations"`
	}
	if err := json.Unmarshal(previewRecorder.Body.Bytes(), &previewBody); err != nil {
		t.Fatal(err)
	}
	if len(previewBody.Templates) != 7 {
		t.Fatalf("template count = %d, want 7", len(previewBody.Templates))
	}
	if len(previewBody.SharedLimitations) == 0 {
		t.Fatal("template preview omitted the shared limitations")
	}
	for _, template := range previewBody.Templates {
		if len(template.Limitations) == 0 || template.Description == "" {
			t.Fatalf("template %s omitted its description or limitation", template.ID)
		}
	}

	apply := httptest.NewRequest(http.MethodPost, "/api/policy/templates/child/apply", nil)
	apply = mux.SetURLVars(apply, map[string]string{"id": "child"})
	applyRecorder := httptest.NewRecorder()
	GSApiPolicyTemplateApply(applyRecorder, apply)
	if applyRecorder.Code != http.StatusCreated {
		t.Fatalf("template apply status = %d, body = %s", applyRecorder.Code, applyRecorder.Body.String())
	}
	if got := policySnapshotGroup(t, "template-child"); got.Name != "Child" {
		t.Fatalf("applied group = %+v", got)
	}

	apply = httptest.NewRequest(http.MethodPost, "/api/policy/templates/child/apply", nil)
	apply = mux.SetURLVars(apply, map[string]string{"id": "child"})
	applyRecorder = httptest.NewRecorder()
	GSApiPolicyTemplateApply(applyRecorder, apply)
	if applyRecorder.Code != http.StatusConflict {
		t.Fatalf("reapply status = %d, body = %s", applyRecorder.Code, applyRecorder.Body.String())
	}

	missing := httptest.NewRequest(http.MethodPost, "/api/policy/templates/missing/apply", nil)
	missing = mux.SetURLVars(missing, map[string]string{"id": "missing"})
	missingRecorder := httptest.NewRecorder()
	GSApiPolicyTemplateApply(missingRecorder, missing)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing template status = %d", missingRecorder.Code)
	}
}

func TestPolicyGroupCRUDPreservesCustomizedRecords(t *testing.T) {
	service, cleanup := policyTestService(t)
	defer cleanup()

	create := httptest.NewRequest(http.MethodPost, "/api/policy/groups", strings.NewReader("{\"name\":\"Custom\",\"description\":\"review me\",\"action\":\"block\",\"domains\":[\"*.example.test\"]}"))
	createRecorder := httptest.NewRecorder()
	GSApiPolicyGroupCreate(createRecorder, create)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRecorder.Code, createRecorder.Body.String())
	}
	var created struct {
		Group gatesentryPolicy.PolicyGroup `json:"group"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Group.ID == "" || created.Group.Name != "Custom" {
		t.Fatalf("created group = %+v", created.Group)
	}

	update := httptest.NewRequest(http.MethodPut, "/api/policy/groups/"+created.Group.ID, strings.NewReader("{\"name\":\"Custom edited\",\"description\":\"changed\",\"action\":\"allow\",\"domains\":[\"allowed.example.test\"]}"))
	update = mux.SetURLVars(update, map[string]string{"id": created.Group.ID})
	updateRecorder := httptest.NewRecorder()
	GSApiPolicyGroupUpdate(updateRecorder, update)
	if updateRecorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updateRecorder.Code, updateRecorder.Body.String())
	}
	if service.Snapshot().Groups[created.Group.ID].Name != "Custom edited" {
		t.Fatal("updated group was not reloaded into the service")
	}

	if err := service.SetDeviceAssignment("device-1", created.Group.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	remove := httptest.NewRequest(http.MethodDelete, "/api/policy/groups/"+created.Group.ID, nil)
	remove = mux.SetURLVars(remove, map[string]string{"id": created.Group.ID})
	removeRecorder := httptest.NewRecorder()
	GSApiPolicyGroupDelete(removeRecorder, remove)
	if removeRecorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", removeRecorder.Code, removeRecorder.Body.String())
	}
	if _, exists := service.Snapshot().Groups[created.Group.ID]; exists {
		t.Fatal("deleted group remains")
	}
	if _, exists := service.Snapshot().Assignments["device-1"]; exists {
		t.Fatal("deleting group left a device assignment")
	}
}

func policySnapshotGroup(t *testing.T, groupID string) gatesentryPolicy.PolicyGroup {
	t.Helper()
	svc := gatesentryDnsServer.GetPolicyService()
	if svc == nil {
		t.Fatal("policy service unavailable")
	}
	group, ok := svc.Snapshot().Groups[groupID]
	if !ok {
		t.Fatalf("group %s not found", groupID)
	}
	return group
}

func TestPolicyCategoriesRoundTrip(t *testing.T) {
	service, cleanup := policyTestService(t)
	defer cleanup()

	get := httptest.NewRequest(http.MethodGet, "/api/policy/categories", nil)
	getRecorder := httptest.NewRecorder()
	GSApiPolicyCategoriesGet(getRecorder, get)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRecorder.Code, getRecorder.Body.String())
	}
	var getBody struct {
		Categories []gatesentryPolicy.CategoryStatus `json:"categories"`
	}
	if err := json.Unmarshal(getRecorder.Body.Bytes(), &getBody); err != nil {
		t.Fatal(err)
	}
	if len(getBody.Categories) != len(gatesentryPolicy.CategoryCatalog()) {
		t.Fatalf("category count = %d, want the full catalog", len(getBody.Categories))
	}
	for _, status := range getBody.Categories {
		if status.Enabled {
			t.Fatalf("category %s reported enabled before any selection", status.ID)
		}
		if status.Name == "" || status.Description == "" {
			t.Fatalf("category %s omitted its description: %+v", status.ID, status)
		}
	}

	put := httptest.NewRequest(http.MethodPut, "/api/policy/categories", strings.NewReader(`{"categories":["social","ads"]}`))
	putRecorder := httptest.NewRecorder()
	GSApiPolicyCategoriesPut(putRecorder, put)
	if putRecorder.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", putRecorder.Code, putRecorder.Body.String())
	}
	var putBody struct {
		Categories []gatesentryPolicy.CategoryStatus `json:"categories"`
	}
	if err := json.Unmarshal(putRecorder.Body.Bytes(), &putBody); err != nil {
		t.Fatal(err)
	}
	enabled := make(map[string]bool)
	for _, status := range putBody.Categories {
		if status.Enabled {
			enabled[status.ID] = true
		}
	}
	if len(enabled) != 2 || !enabled["social"] || !enabled["ads"] {
		t.Fatalf("enabled after PUT = %v, want social and ads", enabled)
	}
	if got := service.EnabledCategories(); len(got) != 2 {
		t.Fatalf("persisted categories = %v, want two", got)
	}

	unknown := httptest.NewRequest(http.MethodPut, "/api/policy/categories", strings.NewReader(`{"categories":["social","typo-category"]}`))
	unknownRecorder := httptest.NewRecorder()
	GSApiPolicyCategoriesPut(unknownRecorder, unknown)
	if unknownRecorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown category status = %d, body = %s", unknownRecorder.Code, unknownRecorder.Body.String())
	}
	if got := service.EnabledCategories(); len(got) != 2 {
		t.Fatalf("a rejected PUT changed the stored selection: %v", got)
	}

	badJSON := httptest.NewRequest(http.MethodPut, "/api/policy/categories", strings.NewReader("{"))
	badJSONRecorder := httptest.NewRecorder()
	GSApiPolicyCategoriesPut(badJSONRecorder, badJSON)
	if badJSONRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON status = %d", badJSONRecorder.Code)
	}
}

func TestPolicyGroupCategorySelectionIsValidatedAndNormalized(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	unknown := httptest.NewRequest(http.MethodPost, "/api/policy/groups", strings.NewReader(`{"name":"Kids","action":"block","categories":["social","typo-category"]}`))
	unknownRecorder := httptest.NewRecorder()
	GSApiPolicyGroupCreate(unknownRecorder, unknown)
	if unknownRecorder.Code != http.StatusBadRequest {
		t.Fatalf("create with unknown category status = %d, body = %s", unknownRecorder.Code, unknownRecorder.Body.String())
	}

	replace := httptest.NewRequest(http.MethodPut, "/api/policy/groups", strings.NewReader(`{"groups":[{"id":"kids","name":"Kids","action":"block","categories":["typo-category"]}]}`))
	replaceRecorder := httptest.NewRecorder()
	GSApiPolicyGroupsReplace(replaceRecorder, replace)
	if replaceRecorder.Code != http.StatusBadRequest {
		t.Fatalf("replace with unknown category status = %d, body = %s", replaceRecorder.Code, replaceRecorder.Body.String())
	}

	// A selection that only differs by case must be stored normalized, or the
	// rule would be persisted in a form that never matches the catalog.
	valid := httptest.NewRequest(http.MethodPost, "/api/policy/groups", strings.NewReader(`{"id":"kids","name":"Kids","action":"block","categories":[" Social ", "social"]}`))
	validRecorder := httptest.NewRecorder()
	GSApiPolicyGroupCreate(validRecorder, valid)
	if validRecorder.Code != http.StatusCreated {
		t.Fatalf("valid create status = %d, body = %s", validRecorder.Code, validRecorder.Body.String())
	}
	if got := policySnapshotGroup(t, "kids").Categories; len(got) != 1 || got[0] != "social" {
		t.Fatalf("stored categories = %v, want [social]", got)
	}
}
