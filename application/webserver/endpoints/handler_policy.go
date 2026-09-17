package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
)

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
