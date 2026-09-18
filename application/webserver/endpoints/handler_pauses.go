package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	"github.com/gorilla/mux"
)

// writePauseJSONError writes a structured JSON error with an appropriate
// status code for pause-related errors.
func writePauseJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gatesentryPolicy.ErrInvalidPause):
		writePolicyJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, gatesentryPolicy.ErrPauseNotFound):
		writePolicyJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, gatesentryPolicy.ErrUnknownGroup):
		writePolicyJSONError(w, http.StatusBadRequest, "Unknown policy group")
	default:
		writePolicyJSONError(w, http.StatusInternalServerError, "Unable to process pause request")
	}
}

// GSApiPausesGET returns all pauses (active, expired, revoked) for the
// admin audit view.
// GET /api/pauses
func GSApiPausesGET(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	pauses := svc.AllPauses()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"pauses": pauses})
}

// GSApiPauseCreate creates a scoped temporary pause. Only authenticated
// administrators can create pauses.
// POST /api/pauses
func GSApiPauseCreate(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	admin := adminUsername(r)
	if admin == "" {
		writePolicyJSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var in gatesentryPolicy.CreatePauseInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	pause, err := svc.CreatePause(in, admin)
	if err != nil {
		writePauseJSONError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pause)
}

// GSApiPauseRevoke revokes (deactivates) a pause without deleting it,
// preserving the audit trail.
// DELETE /api/pauses/{id}
func GSApiPauseRevoke(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	admin := adminUsername(r)
	if admin == "" {
		writePolicyJSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	id := mux.Vars(r)["id"]
	if err := svc.RevokePause(id, admin); err != nil {
		writePauseJSONError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked", "id": id})
}

// GSApiSchedulePresetsGET returns the built-in schedule preset catalog.
// GET /api/policy/schedule-presets
func GSApiSchedulePresetsGET(w http.ResponseWriter, r *http.Request) {
	if policyServiceOrError(w) == nil {
		return
	}
	presets := gatesentryPolicy.SchedulePresets()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"presets": presets})
}

// schedulePresetApplyRequest is the body for applying a preset to a group.
type schedulePresetApplyRequest struct {
	PresetID string `json:"preset_id"`
	Timezone string `json:"timezone"`
	GroupID  string `json:"group_id"`
}

// GSApiSchedulePresetApply applies a named schedule preset to a policy group,
// setting the group's Schedule field. The timezone should be the
// installation's IANA time zone.
// POST /api/policy/schedule-presets/apply
func GSApiSchedulePresetApply(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	admin := adminUsername(r)
	if admin == "" {
		writePolicyJSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var in schedulePresetApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if strings.TrimSpace(in.GroupID) == "" {
		writePolicyJSONError(w, http.StatusBadRequest, "group_id is required")
		return
	}
	schedule, err := gatesentryPolicy.ApplyPreset(in.PresetID, in.Timezone)
	if err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshot := svc.Snapshot()
	group, exists := snapshot.Groups[in.GroupID]
	if !exists {
		writePolicyJSONError(w, http.StatusNotFound, "Policy group not found")
		return
	}
	group.Schedule = schedule
	if err := svc.UpdateGroup(in.GroupID, group); err != nil {
		writePolicyJSONError(w, http.StatusInternalServerError, "Unable to update policy group schedule")
		return
	}
	if !reloadPolicyOrError(w, svc) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"group_id":   in.GroupID,
		"schedule":   schedule,
		"applied_by": admin,
	})
}
