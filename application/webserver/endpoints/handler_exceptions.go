package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	"github.com/gorilla/mux"
)

// adminUsername extracts the authenticated admin username from the request
// context. The authenticationMiddleware sets it; unauthenticated endpoints
// return an empty string.
func adminUsername(r *http.Request) string {
	if u, ok := r.Context().Value("username").(string); ok {
		return u
	}
	return ""
}

// clientIPFromRequest extracts the client IP from the request, preferring
// X-Forwarded-For (set by the proxy when forwarding) and falling back to
// RemoteAddr. It is used for the rate-limited public access-request endpoint.
func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeExceptionJSONError writes a structured JSON error with an appropriate
// status code for exception-related errors.
func writeExceptionJSONError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gatesentryPolicy.ErrInvalidException):
		writePolicyJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, gatesentryPolicy.ErrExceptionNotFound):
		writePolicyJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, gatesentryPolicy.ErrUnknownGroup):
		writePolicyJSONError(w, http.StatusBadRequest, "Unknown policy group")
	default:
		writePolicyJSONError(w, http.StatusInternalServerError, "Unable to process exception request")
	}
}

// GSApiExceptionsGET returns all exceptions (active, expired, revoked) for
// the admin audit view.
// GET /api/exceptions
func GSApiExceptionsGET(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	excs := svc.AllExceptions()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"exceptions": excs})
}

// GSApiExceptionCreate creates a scoped domain exception with explicit
// duration. Only authenticated administrators can create exceptions; public
// access requests cannot.
// POST /api/exceptions
func GSApiExceptionCreate(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	admin := adminUsername(r)
	if admin == "" {
		writePolicyJSONError(w, http.StatusUnauthorized, "Authentication required")
		return
	}
	var in gatesentryPolicy.CreateExceptionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	exc, err := svc.CreateException(in, admin)
	if err != nil {
		writeExceptionJSONError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(exc)
}

// GSApiExceptionRevoke revokes (deactivates) an exception without deleting
// it, preserving the audit trail.
// DELETE /api/exceptions/{id}
func GSApiExceptionRevoke(w http.ResponseWriter, r *http.Request) {
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
	if err := svc.RevokeException(id, admin); err != nil {
		writeExceptionJSONError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked", "id": id})
}

// exceptionPreviewResponse describes what an exception would do before it is
// saved, so the admin can verify scope, precedence, and expiry.
type exceptionPreviewResponse struct {
	Domain     string `json:"domain"`
	Scope      string `json:"scope"`
	DeviceID   string `json:"device_id,omitempty"`
	GroupID    string `json:"group_id,omitempty"`
	Duration   string `json:"duration"`
	ExpiresAt  string `json:"expires_at"`
	Reason     string `json:"reason,omitempty"`
	Precedence string `json:"precedence"`
	Valid      bool   `json:"valid"`
	Error      string `json:"error,omitempty"`
}

// GSApiExceptionPreview computes scope, precedence, and expiry for a
// proposed exception without saving it.
// POST /api/exceptions/preview
func GSApiExceptionPreview(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	var in gatesentryPolicy.CreateExceptionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	resp := exceptionPreviewResponse{
		Domain:   in.Domain,
		Scope:    string(in.Scope),
		DeviceID: in.DeviceID,
		GroupID:  in.GroupID,
		Duration: in.Duration,
		Reason:   in.Reason,
	}
	// Compute precedence description.
	switch in.Scope {
	case gatesentryPolicy.ScopeDevice:
		resp.Precedence = "device-scoped (highest precedence)"
	case gatesentryPolicy.ScopeGroup:
		resp.Precedence = "group-scoped (applies to all devices in the group)"
	case gatesentryPolicy.ScopeInstallation:
		resp.Precedence = "installation-scoped (applies to all devices)"
	default:
		resp.Precedence = "unknown scope"
	}
	// Validate and compute expiry.
	if err := in.Validate(); err != nil {
		resp.Valid = false
		resp.Error = err.Error()
	} else {
		resp.Valid = true
		d, _ := gatesentryPolicy.ParseDuration(in.Duration)
		resp.ExpiresAt = gatesentryPolicy.NowUTC().Add(d).Format("2006-01-02T15:04:05Z07:00")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GSApiAccessRequestsGET returns all access requests for the admin review
// view. Only authenticated administrators can list requests.
// GET /api/access-requests
func GSApiAccessRequestsGET(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	reqs := svc.ListAccessRequests()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"requests": reqs})
}

// GSApiAccessRequestSubmit is the public endpoint for a blocked user to
// request access to a domain. It is NOT behind authenticationMiddleware and
// is rate-limited per client IP. Submitting a request never grants access.
// POST /api/access-requests
func GSApiAccessRequestSubmit(w http.ResponseWriter, r *http.Request) {
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	var in gatesentryPolicy.AccessRequestInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	ip := clientIPFromRequest(r)
	req, err := svc.CreateAccessRequest(in, ip)
	if err != nil {
		if strings.Contains(err.Error(), "rate limit") {
			writePolicyJSONError(w, http.StatusTooManyRequests, err.Error())
			return
		}
		writeExceptionJSONError(w, err)
		return
	}
	// Return the request ID so the user can reference it; never return admin
	// state or other users' requests.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      req.ID,
		"status":  string(req.Status),
		"domain":  req.Domain,
		"message": "Your request has been submitted for review by an administrator.",
	})
}

// GSApiAccessRequestApprove approves a pending access request and creates a
// scoped exception. The admin supplies the exception scope and duration at
// approval time; the requester has no say in scope.
// POST /api/access-requests/{id}/approve
func GSApiAccessRequestApprove(w http.ResponseWriter, r *http.Request) {
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
	var in gatesentryPolicy.CreateExceptionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writePolicyJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	req, exc, err := svc.ApproveAccessRequest(id, in, admin)
	if err != nil {
		writeExceptionJSONError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"request":   req,
		"exception": exc,
	})
}

// GSApiAccessRequestReject rejects a pending access request without creating
// an exception.
// POST /api/access-requests/{id}/reject
func GSApiAccessRequestReject(w http.ResponseWriter, r *http.Request) {
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
	req, err := svc.RejectAccessRequest(id, admin)
	if err != nil {
		writeExceptionJSONError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"request": req})
}
