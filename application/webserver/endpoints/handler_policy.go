package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	"github.com/gorilla/mux"
)

func writePolicyGroupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gatesentryPolicy.ErrGroupExists):
		writePolicyJSONError(w, http.StatusConflict, "Policy group already exists; edit the existing group instead of applying the template again")
	case errors.Is(err, gatesentryPolicy.ErrGroupNotFound):
		writePolicyJSONError(w, http.StatusNotFound, "Policy group not found")
	default:
		writePolicyJSONError(w, http.StatusInternalServerError, "Unable to persist policy group")
	}
}

func writePolicyJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func reloadPolicyOrError(w http.ResponseWriter, svc *gatesentryPolicy.Service) bool {
	if err := svc.Reload(); err != nil {
		http.Error(w, "Unable to reload policy", http.StatusInternalServerError)
		return false
	}
	return true
}

// policyServiceOrError returns the DNS-owned policy service or a 503.
func policyServiceOrError(w http.ResponseWriter) *gatesentryPolicy.Service {
	svc := gatesentryDnsServer.GetPolicyService()
	if svc == nil {
		http.Error(w, `{"error":"Policy service not initialized — DNS server may not be running"}`, http.StatusServiceUnavailable)
		return nil
	}
	return svc
}

// ensureCategoryFeeds schedules a blocklist refresh when a group selects a
// category the gateway has never downloaded. Category feeds download during a
// refresh, so without this a group could reference a category that matches
// nothing until the next scheduled refresh.
func ensureCategoryFeeds(groups ...gatesentryPolicy.PolicyGroup) {
	index := gatesentryDnsServer.GetCategoryIndex()
	if index == nil {
		return
	}
	for _, group := range groups {
		for _, categoryID := range group.Categories {
			if index.UpdatedAt(categoryID).IsZero() {
				gatesentryDnsServer.RequestBlocklistRefresh()
				return
			}
		}
	}
}

// GSApiPolicyCategoriesGet returns the category catalog with its current
// coverage and gateway-wide state. The UI shows domain counts from here so an
// administrator can see what a category actually covers.
// GET /api/policy/categories
func GSApiPolicyCategoriesGet(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"categories": svc.CategoryStatuses()})
}

// GSApiPolicyCategoriesPut saves the gateway-wide category selection and asks
// the DNS scheduler to download the feeds now, so the new coverage is applied
// without waiting for the next refresh interval. Categories selected here
// apply to every client that has no policy group assignment.
// PUT /api/policy/categories
func GSApiPolicyCategoriesPut(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	var body struct {
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if _, err := gatesentryPolicy.NormalizeCategories(body.Categories); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := svc.SetEnabledCategories(body.Categories); err != nil {
		if errors.Is(err, gatesentryPolicy.ErrUnknownCategory) {
			writePolicyJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writePolicyJSONError(w, http.StatusInternalServerError, "Unable to persist category selection")
		return
	}
	// The refresh runs on the scheduler goroutine. Reporting whether it was
	// queued keeps the response honest: the statuses below can still show the
	// previous coverage when a refresh was already in flight.
	refreshQueued := gatesentryDnsServer.RequestBlocklistRefresh()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"categories":        svc.CategoryStatuses(),
		"refresh_scheduled": refreshQueued,
	})
}

// GSApiPolicyGroupsGet returns the persisted policy groups.
// GET /api/policy/groups
func GSApiPolicyGroupsGet(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	snapshot := svc.Snapshot()
	groups := make([]gatesentryPolicy.PolicyGroup, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		groups = append(groups, group)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"groups": groups})
}

// GSApiPolicyTemplatesGet returns the built-in starter catalog. The response
// describes protections and limitations so the UI can show what a template
// will do before it creates an ordinary editable group.
// GET /api/policy/templates
func GSApiPolicyTemplatesGet(w http.ResponseWriter, r *http.Request) {
	if policyServiceOrError(w) == nil {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"templates": gatesentryPolicy.PolicyTemplates()})
}

// GSApiPolicyTemplateApply creates the ordinary group represented by a
// built-in template. It never updates an existing group, so customized
// records cannot be silently reset by a later template apply.
// POST /api/policy/templates/{id}/apply
func GSApiPolicyTemplateApply(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	template, ok := gatesentryPolicy.GetPolicyTemplate(mux.Vars(r)["id"])
	if !ok || !template.Available {
		http.Error(w, "Policy template not found", http.StatusNotFound)
		return
	}
	group := template.Group()
	if err := svc.CreateGroup(group); err != nil {
		writePolicyGroupError(w, err)
		return
	}
	if !reloadPolicyOrError(w, svc) {
		return
	}
	group = svc.Snapshot().Groups[group.ID]
	ensureCategoryFeeds(group)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"template": template, "group": group})
}

func decodePolicyGroup(r *http.Request) (gatesentryPolicy.PolicyGroup, error) {
	var group gatesentryPolicy.PolicyGroup
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		return group, errors.New("invalid JSON body")
	}
	if group.Name == "" {
		return group, errors.New("policy group needs a name")
	}
	switch group.Action {
	case gatesentryPolicy.ActionNone, gatesentryPolicy.ActionAllow, gatesentryPolicy.ActionBlock:
	default:
		return group, errors.New("invalid policy action")
	}
	// Category selections are validated against the shipped catalog so a typo
	// cannot be stored as a rule that silently never matches. The normalized
	// result is stored because category matching is case-sensitive against the
	// catalog IDs.
	normalized, err := gatesentryPolicy.NormalizeCategories(group.Categories)
	if err != nil {
		return group, err
	}
	group.Categories = normalized
	return group, nil
}

// GSApiPolicyGroupCreate creates one ordinary policy record. The server
// assigns an ID when the caller omits one.
// POST /api/policy/groups
func GSApiPolicyGroupCreate(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	group, err := decodePolicyGroup(r)
	if err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	gatesentryPolicy.EnsureGroupID(&group)
	if err := svc.CreateGroup(group); err != nil {
		writePolicyGroupError(w, err)
		return
	}
	if !reloadPolicyOrError(w, svc) {
		return
	}
	group = svc.Snapshot().Groups[group.ID]
	ensureCategoryFeeds(group)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"group": group})
}

// GSApiPolicyGroupUpdate edits one ordinary policy record without changing
// its stable ID or creation time.
// PUT /api/policy/groups/{id}
func GSApiPolicyGroupUpdate(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	group, err := decodePolicyGroup(r)
	if err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	groupID := mux.Vars(r)["id"]
	if err := svc.UpdateGroup(groupID, group); err != nil {
		writePolicyGroupError(w, err)
		return
	}
	if !reloadPolicyOrError(w, svc) {
		return
	}
	group = svc.Snapshot().Groups[groupID]
	ensureCategoryFeeds(group)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"group": group})
}

// GSApiPolicyGroupDelete removes one group and clears assignments to it.
// DELETE /api/policy/groups/{id}
func GSApiPolicyGroupDelete(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	groupID := mux.Vars(r)["id"]
	if err := svc.DeleteGroup(groupID); err != nil {
		writePolicyGroupError(w, err)
		return
	}
	if !reloadPolicyOrError(w, svc) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": groupID})
}

// GSApiPolicyGroupsReplace replaces the full group set atomically.
// PUT /api/policy/groups
func GSApiPolicyGroupsReplace(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	var body struct {
		Groups []gatesentryPolicy.PolicyGroup `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	for i := range body.Groups {
		if body.Groups[i].ID == "" {
			http.Error(w, `{"error":"Every policy group needs a stable id"}`, http.StatusBadRequest)
			return
		}
		switch body.Groups[i].Action {
		case gatesentryPolicy.ActionNone, gatesentryPolicy.ActionAllow, gatesentryPolicy.ActionBlock:
		default:
			http.Error(w, `{"error":"Invalid policy action"}`, http.StatusBadRequest)
			return
		}
		normalized, err := gatesentryPolicy.NormalizeCategories(body.Groups[i].Categories)
		if err != nil {
			http.Error(w, `{"error":"Unknown policy category in group "`+body.Groups[i].ID+`"}`, http.StatusBadRequest)
			return
		}
		body.Groups[i].Categories = normalized
	}
	if err := svc.SaveGroups(body.Groups); err != nil {
		http.Error(w, `{"error":"Unable to persist policy groups"}`, http.StatusInternalServerError)
		return
	}
	if err := svc.Reload(); err != nil {
		http.Error(w, `{"error":"Unable to reload policy"}`, http.StatusInternalServerError)
		return
	}
	ensureCategoryFeeds(body.Groups...)
	GSApiPolicyGroupsGet(w, r)
}

// GSApiPolicyAssignmentsGet returns device→group assignments.
// GET /api/policy/assignments
func GSApiPolicyAssignmentsGet(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	snapshot := svc.Snapshot()
	assignments := make([]gatesentryPolicy.DeviceAssignment, 0, len(snapshot.Assignments))
	for deviceID, groupID := range snapshot.Assignments {
		assignments = append(assignments, gatesentryPolicy.DeviceAssignment{DeviceID: deviceID, GroupID: groupID})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"assignments": assignments})
}

// GSApiPolicyAssignmentsReplace replaces all assignments atomically.
// PUT /api/policy/assignments
func GSApiPolicyAssignmentsReplace(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	var body struct {
		Assignments []gatesentryPolicy.DeviceAssignment `json:"assignments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	snapshot := svc.Snapshot()
	for _, assignment := range body.Assignments {
		if assignment.DeviceID == "" || assignment.GroupID == "" {
			http.Error(w, `{"error":"Assignments need both device_id and group_id"}`, http.StatusBadRequest)
			return
		}
		if _, exists := snapshot.Groups[assignment.GroupID]; !exists {
			http.Error(w, `{"error":"Unknown policy group id"}`, http.StatusBadRequest)
			return
		}
	}
	if err := svc.SaveAssignments(body.Assignments); err != nil {
		http.Error(w, `{"error":"Unable to persist assignments"}`, http.StatusInternalServerError)
		return
	}
	if err := svc.Reload(); err != nil {
		http.Error(w, `{"error":"Unable to reload policy"}`, http.StatusInternalServerError)
		return
	}
	GSApiPolicyAssignmentsGet(w, r)
}

// GSApiPolicyPreview reports the decision a specific request context would
// receive against the active policy and, when a proposed revision is supplied,
// against that revision, without persisting anything or making external
// classification requests. The response carries the matched-rules precedence
// trail, the conditions the selected layer cannot enforce (so DNS enforcement
// is not mistaken for URL/MIME/inspection enforcement), and whether the
// proposed revision would change the decision for this request.
// POST /api/policy/preview
func GSApiPolicyPreview(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	var body gatesentryPolicy.PreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Domain) == "" {
		http.Error(w, `{"error":"domain is required"}`, http.StatusBadRequest)
		return
	}
	result := svc.Preview(body)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
