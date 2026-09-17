package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"errors"
	"net/http"

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
	}
	if err := svc.SaveGroups(body.Groups); err != nil {
		http.Error(w, `{"error":"Unable to persist policy groups"}`, http.StatusInternalServerError)
		return
	}
	if err := svc.Reload(); err != nil {
		http.Error(w, `{"error":"Unable to reload policy"}`, http.StatusInternalServerError)
		return
	}
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
// receive, including the DNS-inapplicable conditions so callers cannot
// mistake DNS enforcement for URL/MIME/inspection enforcement.
// POST /api/policy/preview
func GSApiPolicyPreview(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	var body struct {
		Domain   string `json:"domain"`
		ClientIP string `json:"client_ip"`
		User     string `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"Invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	identity := svc.ResolveIdentity(body.ClientIP, body.User)
	decision := svc.EvaluateDNS(identity, body.Domain)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"identity":                identity,
		"action":                  decision.Action,
		"group_id":                decision.GroupID,
		"matched_domain":          decision.MatchedDomain,
		"inapplicable_conditions": decision.InapplicableConditions,
		"reason":                  decision.Reason,
	})
}
