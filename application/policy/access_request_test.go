package policy

import (
	"testing"
	"time"
)

// TestAccessRequestRateLimit verifies that a client IP is rate-limited after
// MaxAccessRequestsPerMinute requests.
func TestAccessRequestRateLimit(t *testing.T) {
	svc := newTestService(t)

	// Submit MaxAccessRequestsPerMinute requests.
	for i := 0; i < MaxAccessRequestsPerMinute; i++ {
		_, err := svc.CreateAccessRequest(AccessRequestInput{
			Domain: "ads.example",
		}, "192.0.2.10")
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
	}

	// The next request should be rate-limited.
	_, err := svc.CreateAccessRequest(AccessRequestInput{
		Domain: "ads.example",
	}, "192.0.2.10")
	if err == nil {
		t.Fatal("expected rate limit error")
	}

	// A different IP is not affected.
	_, err = svc.CreateAccessRequest(AccessRequestInput{
		Domain: "ads.example",
	}, "192.0.2.20")
	if err != nil {
		t.Fatalf("different IP should not be rate limited: %v", err)
	}
}

// TestAccessRequestDoesNotGrantAccess verifies that creating an access request
// does not create an exception or grant access.
func TestAccessRequestDoesNotGrantAccess(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}

	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"ads.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	// A blocked user submits a request.
	_, err := svc.CreateAccessRequest(AccessRequestInput{
		Domain: "ads.example",
		Reason: "needed for homework",
	}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}

	// The domain should still be blocked.
	id := svc.ResolveIdentityForDNS("192.0.2.10", "")
	if d := svc.EvaluateDNS(id, "ads.example"); d.Action != ActionBlock {
		t.Fatalf("after request: action = %s, want block (request must not grant access)", d.Action)
	}

	// No exceptions should exist.
	excs := svc.AllExceptions()
	if len(excs) != 0 {
		t.Fatalf("expected 0 exceptions, got %d", len(excs))
	}
}

// TestAccessRequestApprovalCreatesException verifies that approving a request
// creates an exception and links it.
func TestAccessRequestApprovalCreatesException(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}

	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"ads.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	req, err := svc.CreateAccessRequest(AccessRequestInput{
		Domain: "ads.example",
	}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}

	// Admin approves with a device-scoped exception.
	updatedReq, exc, err := svc.ApproveAccessRequest(req.ID, CreateExceptionInput{
		Domain:   "ads.example",
		Scope:    ScopeDevice,
		DeviceID: "device-1",
		Duration: "30m",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if updatedReq.Status != AccessRequestApproved {
		t.Fatalf("request status = %s, want approved", updatedReq.Status)
	}
	if updatedReq.ExceptionID != exc.ID {
		t.Fatalf("request exception ID = %s, want %s", updatedReq.ExceptionID, exc.ID)
	}

	// The domain is now allowed for device-1.
	id := svc.ResolveIdentityForDNS("192.0.2.10", "")
	if d := svc.EvaluateDNS(id, "ads.example"); d.Action != ActionAllow {
		t.Fatalf("after approval: action = %s, want allow", d.Action)
	}
}

// TestAccessRequestRejection verifies that rejecting a request does not create
// an exception.
func TestAccessRequestRejection(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}

	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"ads.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	req, err := svc.CreateAccessRequest(AccessRequestInput{
		Domain: "ads.example",
	}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}

	updatedReq, err := svc.RejectAccessRequest(req.ID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if updatedReq.Status != AccessRequestRejected {
		t.Fatalf("status = %s, want rejected", updatedReq.Status)
	}
	if updatedReq.ExceptionID != "" {
		t.Fatal("rejected request should not have an exception ID")
	}

	// Domain still blocked.
	id := svc.ResolveIdentityForDNS("192.0.2.10", "")
	if d := svc.EvaluateDNS(id, "ads.example"); d.Action != ActionBlock {
		t.Fatalf("after rejection: action = %s, want block", d.Action)
	}
}

// TestAccessRequestDoubleReviewFails verifies that a request cannot be
// approved or rejected twice.
func TestAccessRequestDoubleReviewFails(t *testing.T) {
	svc := newTestService(t)
	req, err := svc.CreateAccessRequest(AccessRequestInput{Domain: "ads.example"}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = svc.ApproveAccessRequest(req.ID, CreateExceptionInput{
		Domain: "ads.example", Scope: ScopeInstallation, Duration: "5m",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	// Approve again.
	_, _, err = svc.ApproveAccessRequest(req.ID, CreateExceptionInput{
		Domain: "ads.example", Scope: ScopeInstallation, Duration: "5m",
	}, "admin")
	if err == nil {
		t.Fatal("double approve should fail")
	}
	// Reject already-approved.
	_, err = svc.RejectAccessRequest(req.ID, "admin")
	if err == nil {
		t.Fatal("reject after approve should fail")
	}
}

// TestAccessRequestRestartRecovery verifies access requests persist across
// reload.
func TestAccessRequestRestartRecovery(t *testing.T) {
	svc := newTestService(t)
	req, err := svc.CreateAccessRequest(AccessRequestInput{Domain: "ads.example"}, "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate restart by reloading.
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	reqs := svc.ListAccessRequests()
	found := false
	for _, r := range reqs {
		if r.ID == req.ID && r.Status == AccessRequestPending {
			found = true
		}
	}
	if !found {
		t.Fatal("pending request not recovered after reload")
	}
}

// TestRateLimiterPrunesExpiredEntries verifies old entries are pruned.
func TestRateLimiterPrunesExpiredEntries(t *testing.T) {
	rl := newAccessRequestRateLimiter()
	rl.window = 100 * time.Millisecond
	rl.maxHits = 2

	if !rl.allow("1.2.3.4") {
		t.Fatal("first allow failed")
	}
	if !rl.allow("1.2.3.4") {
		t.Fatal("second allow failed")
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("third allow should be denied")
	}

	// Wait for window to expire.
	time.Sleep(150 * time.Millisecond)

	if !rl.allow("1.2.3.4") {
		t.Fatal("allow after window expiry failed")
	}
}
