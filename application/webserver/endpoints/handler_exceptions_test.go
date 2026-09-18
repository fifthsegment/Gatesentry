package gatesentryWebserverEndpoints

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	"github.com/gorilla/mux"
)

// withAdmin returns r with the "username" context value the
// authenticationMiddleware sets, so auth-required handlers see an admin.
func withAdmin(r *http.Request, admin string) *http.Request {
	if admin == "" {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), "username", admin))
}

// jsonBody marshals a string map into a request body, avoiding hand-escaped
// JSON in test source.
func jsonBody(t *testing.T, m map[string]string) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decodeMap(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("decode %q: %v", string(body), err)
	}
	return m
}

func newJSONRequest(method, target string, body []byte) *http.Request {
	return httptest.NewRequest(method, target, bytes.NewReader(body))
}

// TestExceptionCreateListRevoke exercises the admin CRUD lifecycle and
// confirms a revoked exception is audited but no longer enforces.
func TestExceptionCreateListRevoke(t *testing.T) {
	svc, cleanup := policyTestService(t)
	defer cleanup()

	body := jsonBody(t, map[string]string{
		"domain":    "games.example",
		"scope":     "device",
		"device_id": "device-1",
		"duration":  "1h",
		"reason":    "one-off access",
	})
	req := withAdmin(newJSONRequest(http.MethodPost, "/api/exceptions", body), "admin")
	rec := httptest.NewRecorder()
	GSApiExceptionCreate(rec, req)
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

	// It enforces immediately.
	if _, ok := svc.EvaluateException(gatesentryPolicy.Identity{DeviceID: "device-1"}, "games.example"); !ok {
		t.Fatalf("exception should match before revoke")
	}

	// List shows one exception.
	lrec := httptest.NewRecorder()
	GSApiExceptionsGET(lrec, withAdmin(newJSONRequest(http.MethodGet, "/api/exceptions", nil), "admin"))
	if lrec.Code != http.StatusOK {
		t.Fatalf("list status = %d", lrec.Code)
	}
	listed := decodeMap(t, lrec.Body.Bytes())
	excs, _ := listed["exceptions"].([]interface{})
	if len(excs) != 1 {
		t.Fatalf("expected 1 exception, got %d: %s", len(excs), lrec.Body.String())
	}

	// Revoke.
	rreq := withAdmin(mux.SetURLVars(newJSONRequest(http.MethodDelete, "/api/exceptions/{id}", nil), map[string]string{"id": id}), "admin")
	rrec := httptest.NewRecorder()
	GSApiExceptionRevoke(rrec, rreq)
	if rrec.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, body = %s", rrec.Code, rrec.Body.String())
	}

	// Revoked exception is audited (still listed) but inactive and not enforcing.
	lrec2 := httptest.NewRecorder()
	GSApiExceptionsGET(lrec2, withAdmin(newJSONRequest(http.MethodGet, "/api/exceptions", nil), "admin"))
	listed2 := decodeMap(t, lrec2.Body.Bytes())
	excs2, _ := listed2["exceptions"].([]interface{})
	if len(excs2) != 1 {
		t.Fatalf("expected revoked exception still audited, got %d", len(excs2))
	}
	row, _ := excs2[0].(map[string]interface{})
	if row["active"] != false {
		t.Fatalf("expected active=false after revoke, got %v", row["active"])
	}
	if _, ok := svc.EvaluateException(gatesentryPolicy.Identity{DeviceID: "device-1"}, "games.example"); ok {
		t.Fatalf("revoked exception should not enforce")
	}
}

// TestExceptionPreview reports scope, precedence, and expiry without saving.
func TestExceptionPreview(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	// Valid proposal.
	body := jsonBody(t, map[string]string{
		"domain":    "games.example",
		"scope":     "device",
		"device_id": "device-1",
		"duration":  "1h",
	})
	rec := httptest.NewRecorder()
	GSApiExceptionPreview(rec, withAdmin(newJSONRequest(http.MethodPost, "/api/exceptions/preview", body), "admin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", rec.Code, rec.Body.String())
	}
	resp := decodeMap(t, rec.Body.Bytes())
	if resp["valid"] != true {
		t.Fatalf("expected valid=true, body = %s", rec.Body.String())
	}
	if resp["scope"] != "device" {
		t.Fatalf("scope = %v", resp["scope"])
	}
	if !strings.Contains(resp["precedence"].(string), "device-scoped") {
		t.Fatalf("precedence = %v", resp["precedence"])
	}
	if resp["expires_at"] == nil || resp["expires_at"] == "" {
		t.Fatalf("expected expires_at set, body = %s", rec.Body.String())
	}

	// Invalid proposal (missing domain) is reported but does not 500.
	bad := jsonBody(t, map[string]string{"scope": "device", "device_id": "device-1", "duration": "1h"})
	brec := httptest.NewRecorder()
	GSApiExceptionPreview(brec, withAdmin(newJSONRequest(http.MethodPost, "/api/exceptions/preview", bad), "admin"))
	if brec.Code != http.StatusOK {
		t.Fatalf("invalid preview status = %d, body = %s", brec.Code, brec.Body.String())
	}
	bresp := decodeMap(t, brec.Body.Bytes())
	if bresp["valid"] != false {
		t.Fatalf("expected valid=false, body = %s", brec.Body.String())
	}
	if !strings.Contains(bresp["error"].(string), "domain") {
		t.Fatalf("expected error mentioning domain, got %v", bresp["error"])
	}

	// Preview never persists an exception.
	// (A second GET to exceptions would be empty; verify via a fresh list.)
	lrec := httptest.NewRecorder()
	GSApiExceptionsGET(lrec, withAdmin(newJSONRequest(http.MethodGet, "/api/exceptions", nil), "admin"))
	listed := decodeMap(t, lrec.Body.Bytes())
	excs, _ := listed["exceptions"].([]interface{})
	if len(excs) != 0 {
		t.Fatalf("preview must not persist exceptions, got %d", len(excs))
	}
}

// TestExceptionCreateRequiresAuth ensures a block-page user cannot create an
// exception: only authenticated admins can.
func TestExceptionCreateRequiresAuth(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	body := jsonBody(t, map[string]string{
		"domain":    "games.example",
		"scope":     "device",
		"device_id": "device-1",
		"duration":  "1h",
	})
	// No admin context: simulate an unauthenticated request.
	rec := httptest.NewRecorder()
	GSApiExceptionCreate(rec, newJSONRequest(http.MethodPost, "/api/exceptions", body))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d, body = %s", rec.Code, rec.Body.String())
	}
}

// TestExceptionCreateRejectsUnknownGroup verifies group-scoped exceptions
// require an existing group.
func TestExceptionCreateRejectsUnknownGroup(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	body := jsonBody(t, map[string]string{
		"domain":   "games.example",
		"scope":    "group",
		"group_id": "missing",
		"duration": "1h",
	})
	rec := httptest.NewRecorder()
	GSApiExceptionCreate(rec, withAdmin(newJSONRequest(http.MethodPost, "/api/exceptions", body), "admin"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown group, got %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "group") {
		t.Fatalf("expected error mentioning group, got %s", rec.Body.String())
	}
}

// TestAccessRequestSubmitDoesNotGrant verifies the public submit endpoint is
// unauthenticated, returns the request id, and never creates an exception.
func TestAccessRequestSubmitDoesNotGrant(t *testing.T) {
	svc, cleanup := policyTestService(t)
	defer cleanup()

	body := jsonBody(t, map[string]string{"domain": "games.example", "reason": "need access"})
	// No withAdmin: this is a public endpoint.
	rec := httptest.NewRecorder()
	GSApiAccessRequestSubmit(rec, newJSONRequest(http.MethodPost, "/api/access-requests", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit status = %d, body = %s", rec.Code, rec.Body.String())
	}
	resp := decodeMap(t, rec.Body.Bytes())
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatalf("expected id in response, body = %s", rec.Body.String())
	}
	if resp["status"] != "pending" {
		t.Fatalf("expected status pending, got %v", resp["status"])
	}

	// No exception was created: the snapshot is empty.
	if _, ok := svc.EvaluateException(gatesentryPolicy.Identity{DeviceID: "device-1"}, "games.example"); ok {
		t.Fatalf("submit must not grant access (no exception created)")
	}
}

// TestAccessRequestRateLimited confirms per-IP rate limiting on the public
// submit endpoint.
func TestAccessRequestRateLimited(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	const over = gatesentryPolicy.MaxAccessRequestsPerMinute + 1
	var last int
	for i := 0; i < over; i++ {
		body := jsonBody(t, map[string]string{"domain": "games.example"})
		req := newJSONRequest(http.MethodPost, "/api/access-requests", body)
		req.Header.Set("X-Forwarded-For", "203.0.113.7")
		rec := httptest.NewRecorder()
		GSApiAccessRequestSubmit(rec, req)
		last = rec.Code
		if i < gatesentryPolicy.MaxAccessRequestsPerMinute {
			if rec.Code != http.StatusCreated {
				t.Fatalf("request %d: expected 201, got %d, body = %s", i, rec.Code, rec.Body.String())
			}
		}
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after rate limit, got %d", last)
	}
}

// TestAccessRequestApproveCreatesException approves a submitted request as an
// admin and confirms the exception enforces and the request is linked.
func TestAccessRequestApproveCreatesException(t *testing.T) {
	svc, cleanup := policyTestService(t)
	defer cleanup()

	// Submit a public request.
	sub := jsonBody(t, map[string]string{"domain": "games.example", "reason": "need access"})
	sreq := newJSONRequest(http.MethodPost, "/api/access-requests", sub)
	sreq.Header.Set("X-Forwarded-For", "203.0.113.9")
	srec := httptest.NewRecorder()
	GSApiAccessRequestSubmit(srec, sreq)
	if srec.Code != http.StatusCreated {
		t.Fatalf("submit status = %d, body = %s", srec.Code, srec.Body.String())
	}
	subResp := decodeMap(t, srec.Body.Bytes())
	reqID, _ := subResp["id"].(string)
	if reqID == "" {
		t.Fatalf("missing request id: %s", srec.Body.String())
	}

	// Approve as admin with a device-scoped exception.
	approve := jsonBody(t, map[string]string{
		"domain":    "games.example",
		"scope":     "device",
		"device_id": "device-1",
		"duration":  "1h",
		"reason":    "approved by admin",
	})
	areq := withAdmin(mux.SetURLVars(newJSONRequest(http.MethodPost, "/api/access-requests/{id}/approve", approve), map[string]string{"id": reqID}), "admin")
	arec := httptest.NewRecorder()
	GSApiAccessRequestApprove(arec, areq)
	if arec.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", arec.Code, arec.Body.String())
	}
	aresp := decodeMap(t, arec.Body.Bytes())
	reqObj, _ := aresp["request"].(map[string]interface{})
	if reqObj == nil || reqObj["status"] != "approved" {
		t.Fatalf("expected request status approved, body = %s", arec.Body.String())
	}
	if reqObj["reviewed_by"] != "admin" {
		t.Fatalf("expected reviewed_by admin, got %v", reqObj["reviewed_by"])
	}
	excObj, _ := aresp["exception"].(map[string]interface{})
	if excObj == nil || excObj["id"] == nil || excObj["id"] == "" {
		t.Fatalf("expected exception in approve response, body = %s", arec.Body.String())
	}

	// The exception now enforces.
	if _, ok := svc.EvaluateException(gatesentryPolicy.Identity{DeviceID: "device-1"}, "games.example"); !ok {
		t.Fatalf("approved request should create an enforcing exception")
	}
}

// TestAccessRequestReject confirms rejection creates no exception.
func TestAccessRequestReject(t *testing.T) {
	svc, cleanup := policyTestService(t)
	defer cleanup()

	sub := jsonBody(t, map[string]string{"domain": "games.example"})
	sreq := newJSONRequest(http.MethodPost, "/api/access-requests", sub)
	sreq.Header.Set("X-Forwarded-For", "203.0.113.10")
	srec := httptest.NewRecorder()
	GSApiAccessRequestSubmit(srec, sreq)
	reqID, _ := decodeMap(t, srec.Body.Bytes())["id"].(string)

	rreq := withAdmin(mux.SetURLVars(newJSONRequest(http.MethodPost, "/api/access-requests/{id}/reject", nil), map[string]string{"id": reqID}), "admin")
	rrec := httptest.NewRecorder()
	GSApiAccessRequestReject(rrec, rreq)
	if rrec.Code != http.StatusOK {
		t.Fatalf("reject status = %d, body = %s", rrec.Code, rrec.Body.String())
	}
	rr := decodeMap(t, rrec.Body.Bytes())
	reqObj, _ := rr["request"].(map[string]interface{})
	if reqObj == nil || reqObj["status"] != "rejected" {
		t.Fatalf("expected status rejected, body = %s", rrec.Body.String())
	}

	// No exception created.
	if _, ok := svc.EvaluateException(gatesentryPolicy.Identity{DeviceID: "device-1"}, "games.example"); ok {
		t.Fatalf("rejected request must not create an exception")
	}
}

// TestAccessRequestApproveRequiresAuth ensures only admins can approve.
func TestAccessRequestApproveRequiresAuth(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	approve := jsonBody(t, map[string]string{
		"domain":    "games.example",
		"scope":     "device",
		"device_id": "device-1",
		"duration":  "1h",
	})
	// No admin context.
	areq := mux.SetURLVars(newJSONRequest(http.MethodPost, "/api/access-requests/{id}/approve", approve), map[string]string{"id": "doesnotmatter"})
	arec := httptest.NewRecorder()
	GSApiAccessRequestApprove(arec, areq)
	if arec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d, body = %s", arec.Code, arec.Body.String())
	}
}

// TestAccessRequestCannotBeReviewedTwice confirms a reviewed request cannot
// change state again.
func TestAccessRequestCannotBeReviewedTwice(t *testing.T) {
	_, cleanup := policyTestService(t)
	defer cleanup()

	sub := jsonBody(t, map[string]string{"domain": "games.example"})
	sreq := newJSONRequest(http.MethodPost, "/api/access-requests", sub)
	sreq.Header.Set("X-Forwarded-For", "203.0.113.11")
	srec := httptest.NewRecorder()
	GSApiAccessRequestSubmit(srec, sreq)
	reqID, _ := decodeMap(t, srec.Body.Bytes())["id"].(string)

	// Approve.
	approve := jsonBody(t, map[string]string{
		"domain":    "games.example",
		"scope":     "device",
		"device_id": "device-1",
		"duration":  "1h",
	})
	areq := withAdmin(mux.SetURLVars(newJSONRequest(http.MethodPost, "/api/access-requests/{id}/approve", approve), map[string]string{"id": reqID}), "admin")
	arec := httptest.NewRecorder()
	GSApiAccessRequestApprove(arec, areq)
	if arec.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body = %s", arec.Code, arec.Body.String())
	}

	// Reject the same request: must fail (already reviewed).
	rreq := withAdmin(mux.SetURLVars(newJSONRequest(http.MethodPost, "/api/access-requests/{id}/reject", nil), map[string]string{"id": reqID}), "admin")
	rrec := httptest.NewRecorder()
	GSApiAccessRequestReject(rrec, rreq)
	if rrec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on second review, got %d, body = %s", rrec.Code, rrec.Body.String())
	}
}
