package policy

import (
	"testing"
	"time"
)

// TestExceptionDeviceScopeOverridesGroupBlock verifies that a device-scoped
// exception exempts one device from a group block without affecting others.
func TestExceptionDeviceScopeOverridesGroupBlock(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}

	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	// Before exception: domain is blocked for device-1.
	identity := svc.ResolveIdentityForDNS("192.0.2.10", "")
	decision := svc.EvaluateDNS(identity, "chess.games.example")
	if decision.Action != ActionBlock {
		t.Fatalf("before exception: action = %s, want block", decision.Action)
	}

	// Create a device-scoped exception for device-1.
	exc, err := svc.CreateException(CreateExceptionInput{
		Domain:   "*.games.example",
		Scope:    ScopeDevice,
		DeviceID: "device-1",
		Duration: "10m",
		Reason:   "false positive",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if exc.ID == "" || !exc.Active {
		t.Fatalf("exception not created properly: %+v", exc)
	}

	// After exception: domain is allowed (bypassed) for device-1.
	decision = svc.EvaluateDNS(identity, "chess.games.example")
	if decision.Action != ActionAllow {
		t.Fatalf("after exception: action = %s, want allow", decision.Action)
	}
	if decision.Reason == "" {
		t.Fatal("decision reason should explain the exception")
	}
}

// TestExceptionScopeIsolation verifies that a device-scoped exception does
// not apply to other devices in the same group.
func TestExceptionScopeIsolation(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{
		"192.0.2.10": "device-1",
		"192.0.2.20": "device-2",
	}}

	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-2", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	// Exception for device-1 only.
	if _, err := svc.CreateException(CreateExceptionInput{
		Domain:   "*.games.example",
		Scope:    ScopeDevice,
		DeviceID: "device-1",
		Duration: "10m",
	}, "admin"); err != nil {
		t.Fatal(err)
	}

	id1 := svc.ResolveIdentityForDNS("192.0.2.10", "")
	id2 := svc.ResolveIdentityForDNS("192.0.2.20", "")

	// device-1 is exempted.
	if d := svc.EvaluateDNS(id1, "chess.games.example"); d.Action != ActionAllow {
		t.Fatalf("device-1: action = %s, want allow", d.Action)
	}
	// device-2 is still blocked.
	if d := svc.EvaluateDNS(id2, "chess.games.example"); d.Action != ActionBlock {
		t.Fatalf("device-2: action = %s, want block", d.Action)
	}
}

// TestExceptionGroupScopeOverridesGroupBlock verifies a group-scoped
// exception applies to all devices in that group.
func TestExceptionGroupScopeOverridesGroupBlock(t *testing.T) {
	svc := newTestService(t)
	svc.devices = &mapResolver{devices: map[string]string{
		"192.0.2.10": "device-1",
		"192.0.2.20": "device-2",
	}}

	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedDomains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("device-2", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateException(CreateExceptionInput{
		Domain:   "*.games.example",
		Scope:    ScopeGroup,
		GroupID:  "kids",
		Duration: "30m",
	}, "admin"); err != nil {
		t.Fatal(err)
	}

	for _, ip := range []string{"192.0.2.10", "192.0.2.20"} {
		id := svc.ResolveIdentityForDNS(ip, "")
		if d := svc.EvaluateDNS(id, "chess.games.example"); d.Action != ActionAllow {
			t.Fatalf("%s: action = %s, want allow", ip, d.Action)
		}
	}
}

// TestExceptionInstallationScopeOverridesGroupBlock verifies an
// installation-scoped exception applies to all devices regardless of group.
func TestExceptionInstallationScopeOverridesGroupBlock(t *testing.T) {
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

	if _, err := svc.CreateException(CreateExceptionInput{
		Domain:   "ads.example",
		Scope:    ScopeInstallation,
		Duration: "1h",
	}, "admin"); err != nil {
		t.Fatal(err)
	}

	// Unknown device (no group) also gets the exception.
	id := Identity{Source: SourceUnknown}
	if d := svc.EvaluateDNS(id, "ads.example"); d.Action != ActionAllow {
		t.Fatalf("installation exception for unknown device: action = %s, want allow", d.Action)
	}
}

// TestExceptionExpiry verifies that an expired exception is ignored during
// evaluation.
func TestExceptionExpiry(t *testing.T) {
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

	// Create an exception that is already expired by writing it directly.
	now := time.Now().UTC()
	exc := Exception{
		ID:        "test-expired",
		Domain:    "ads.example",
		Scope:     ScopeDevice,
		DeviceID:  "device-1",
		CreatedBy: "admin",
		CreatedAt: now.Add(-2 * time.Minute),
		ExpiresAt: now.Add(-1 * time.Minute), // expired
		Active:    true,
	}
	// Manually inject the expired exception into the snapshot.
	svc.exc.mu.Lock()
	svc.exc.snapshot.Exceptions = append(svc.exc.snapshot.Exceptions, exc)
	svc.exc.mu.Unlock()

	id := svc.ResolveIdentityForDNS("192.0.2.10", "")
	decision := svc.EvaluateDNS(id, "ads.example")
	if decision.Action != ActionBlock {
		t.Fatalf("expired exception: action = %s, want block", decision.Action)
	}
}

// TestExceptionRevocation verifies that a revoked exception stops applying.
func TestExceptionRevocation(t *testing.T) {
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

	exc, err := svc.CreateException(CreateExceptionInput{
		Domain:   "ads.example",
		Scope:    ScopeDevice,
		DeviceID: "device-1",
		Duration: "10m",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}

	id := svc.ResolveIdentityForDNS("192.0.2.10", "")
	if d := svc.EvaluateDNS(id, "ads.example"); d.Action != ActionAllow {
		t.Fatalf("before revoke: action = %s, want allow", d.Action)
	}

	if err := svc.RevokeException(exc.ID, "admin2"); err != nil {
		t.Fatal(err)
	}

	if d := svc.EvaluateDNS(id, "ads.example"); d.Action != ActionBlock {
		t.Fatalf("after revoke: action = %s, want block", d.Action)
	}

	// Revoking again should fail.
	if err := svc.RevokeException(exc.ID, "admin2"); err == nil {
		t.Fatal("double revoke should fail")
	}
}

// TestExceptionRestartRecovery verifies that exceptions persist across a
// service reload (simulating a restart).
func TestExceptionRestartRecovery(t *testing.T) {
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

	if _, err := svc.CreateException(CreateExceptionInput{
		Domain:   "ads.example",
		Scope:    ScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
	}, "admin"); err != nil {
		t.Fatal(err)
	}

	// Simulate restart: reload both groups and exceptions.
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	id := svc.ResolveIdentityForDNS("192.0.2.10", "")
	if d := svc.EvaluateDNS(id, "ads.example"); d.Action != ActionAllow {
		t.Fatalf("after restart: action = %s, want allow (exception should persist)", d.Action)
	}
}

// TestExceptionPrecedenceDeviceOverGroup verifies that a device-scoped
// exception takes precedence over a group-scoped one.
func TestExceptionPrecedenceDeviceOverGroup(t *testing.T) {
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

	// Create both a group-scoped and a device-scoped exception.
	if _, err := svc.CreateException(CreateExceptionInput{
		Domain: "ads.example", Scope: ScopeGroup, GroupID: "kids", Duration: "1h",
	}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateException(CreateExceptionInput{
		Domain: "ads.example", Scope: ScopeDevice, DeviceID: "device-1", Duration: "1h",
	}, "admin"); err != nil {
		t.Fatal(err)
	}

	id := svc.ResolveIdentityForDNS("192.0.2.10", "")
	exc, ok := svc.EvaluateException(id, "ads.example")
	if !ok {
		t.Fatal("expected an exception match")
	}
	if exc.Scope != ScopeDevice {
		t.Fatalf("precedence: matched scope = %s, want device", exc.Scope)
	}
}

// TestExceptionValidation rejects malformed inputs.
func TestExceptionValidation(t *testing.T) {
	cases := []struct {
		name string
		in   CreateExceptionInput
	}{
		{"empty domain", CreateExceptionInput{Scope: ScopeDevice, DeviceID: "d1", Duration: "5m"}},
		{"empty scope", CreateExceptionInput{Domain: "ads.example", Duration: "5m"}},
		{"device scope no device", CreateExceptionInput{Domain: "ads.example", Scope: ScopeDevice, Duration: "5m"}},
		{"group scope no group", CreateExceptionInput{Domain: "ads.example", Scope: ScopeGroup, Duration: "5m"}},
		{"empty duration", CreateExceptionInput{Domain: "ads.example", Scope: ScopeInstallation}},
		{"invalid duration", CreateExceptionInput{Domain: "ads.example", Scope: ScopeInstallation, Duration: "abc"}},
		{"too short", CreateExceptionInput{Domain: "ads.example", Scope: ScopeInstallation, Duration: "1s"}},
		{"too long", CreateExceptionInput{Domain: "ads.example", Scope: ScopeInstallation, Duration: "800h"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.in.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestExceptionGroupScopedWithUnknownGroup verifies that creating a
// group-scoped exception for a non-existent group fails.
func TestExceptionGroupScopedWithUnknownGroup(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CreateException(CreateExceptionInput{
		Domain:   "ads.example",
		Scope:    ScopeGroup,
		GroupID:  "nonexistent",
		Duration: "10m",
	}, "admin")
	if err == nil {
		t.Fatal("expected error for unknown group")
	}
}

// TestExceptionCreateWithoutCreatedBy verifies that created_by is required.
func TestExceptionCreateWithoutCreatedBy(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.CreateException(CreateExceptionInput{
		Domain:   "ads.example",
		Scope:    ScopeInstallation,
		Duration: "10m",
	}, "")
	if err == nil {
		t.Fatal("expected error for empty created_by")
	}
}
