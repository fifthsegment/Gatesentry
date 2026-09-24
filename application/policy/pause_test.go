package policy

import (
	"os"
	"testing"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func TestCreatePauseValidateRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		in      CreatePauseInput
		wantErr bool
	}{
		{"device scope no id", CreatePauseInput{Scope: PauseScopeDevice, Duration: "30m"}, true},
		{"group scope no id", CreatePauseInput{Scope: PauseScopeGroup, Duration: "30m"}, true},
		{"invalid scope", CreatePauseInput{Scope: "bogus", Duration: "30m"}, true},
		{"no duration", CreatePauseInput{Scope: PauseScopeInstallation, Duration: ""}, true},
		{"bad duration", CreatePauseInput{Scope: PauseScopeInstallation, Duration: "abc"}, true},
		{"too short", CreatePauseInput{Scope: PauseScopeInstallation, Duration: "10s"}, true},
		{"too long", CreatePauseInput{Scope: PauseScopeInstallation, Duration: "48h"}, true},
		{"valid installation", CreatePauseInput{Scope: PauseScopeInstallation, Duration: "30m"}, false},
		{"valid device", CreatePauseInput{Scope: PauseScopeDevice, DeviceID: "dev-1", Duration: "1h"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestPauseSuppressesGroupBlock(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	decision := svc.EvaluateDNS(identity, "chess.games.example")
	if decision.Action != ActionBlock {
		t.Fatalf("before pause: decision = %+v, want block", decision)
	}
	pause, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
		Reason:   "homework break",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !pause.Active {
		t.Fatal("new pause should be active")
	}
	decision = svc.EvaluateDNS(identity, "chess.games.example")
	if decision.Action != ActionNone {
		t.Fatalf("after pause: decision = %+v, want none (block suppressed)", decision)
	}
	if decision.Reason != "paused: device" {
		t.Fatalf("reason = %q, want paused: device", decision.Reason)
	}
}

func TestPauseDoesNotSuppressAllowGroup(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "adults", AllowedDomains: []string{"tracker.example"}, Users: []string{"dana"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeInstallation,
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "adults", Source: SourceAuthUser}
	decision := svc.EvaluateDNS(identity, "tracker.example")
	if decision.Action != ActionAllow {
		t.Fatalf("decision = %+v, want allow (pauses do not affect allow groups)", decision)
	}
}

func TestPauseScopeIsolation(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	id1 := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	if d := svc.EvaluateDNS(id1, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("device-1 decision = %+v, want none (paused)", d)
	}
	id2 := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-2"}
	if d := svc.EvaluateDNS(id2, "chess.games.example"); d.Action != ActionBlock {
		t.Fatalf("device-2 decision = %+v, want block (not paused)", d)
	}
}

func TestPauseScopeGroup(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeGroup,
		GroupID:  "kids",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	id1 := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	if d := svc.EvaluateDNS(id1, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("device-1 decision = %+v, want none (group paused)", d)
	}
	id2 := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-2"}
	if d := svc.EvaluateDNS(id2, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("device-2 decision = %+v, want none (group paused)", d)
	}
}

func TestPauseScopeInstallation(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
		{ID: "teens", BlockedDomains: []string{"*.social.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeInstallation,
		Duration: "30m",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	id1 := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	if d := svc.EvaluateDNS(id1, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("kids decision = %+v, want none (installation paused)", d)
	}
	id2 := Identity{GroupID: "teens", Source: SourceDevice, DeviceID: "device-2"}
	if d := svc.EvaluateDNS(id2, "social.example"); d.Action != ActionNone {
		t.Fatalf("teens decision = %+v, want none (installation paused)", d)
	}
}

func TestPausePrecedenceDeviceOverGroupOverInstallation(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeInstallation,
		Duration: "1h",
		Reason:   "installation pause",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeGroup,
		GroupID:  "kids",
		Duration: "1h",
		Reason:   "group pause",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	id1 := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	d := svc.EvaluateDNS(id1, "chess.games.example")
	if d.Reason != "paused: group" {
		t.Fatalf("reason = %q, want paused: group (group > installation)", d.Reason)
	}
	_, err = svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
		Reason:   "device pause",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	d = svc.EvaluateDNS(id1, "chess.games.example")
	if d.Reason != "paused: device" {
		t.Fatalf("reason = %q, want paused: device (device > group > installation)", d.Reason)
	}
}

func TestPauseExpiry(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	pause, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "2m",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	svc.SetClock(pause.CreatedAt.Add(1 * time.Minute))
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("1 min after: decision = %+v, want none (paused)", d)
	}
	svc.SetClock(pause.Until.Add(1 * time.Second))
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionBlock {
		t.Fatalf("after expiry: decision = %+v, want block (pause expired)", d)
	}
}

func TestPauseExpiryBoundary(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	pause, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "2m",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	svc.SetClock(pause.Until)
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionBlock {
		t.Fatalf("at expiry boundary: decision = %+v, want block (pause expired at exact Until)", d)
	}
	svc.SetClock(pause.Until.Add(-1 * time.Second))
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("1s before expiry: decision = %+v, want none (paused)", d)
	}
}

func TestPauseRevoke(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	pause, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("before revoke: decision = %+v, want none", d)
	}
	if err := svc.RevokePause(pause.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionBlock {
		t.Fatalf("after revoke: decision = %+v, want block", d)
	}
	if err := svc.RevokePause(pause.ID, "admin"); err == nil {
		t.Fatal("expected error revoking already-revoked pause")
	}
}

func TestPauseSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewService(store, &mapResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Reload(); err != nil {
		t.Fatal(err)
	}
	pause, err := first.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	_ = pause
	restarted, err := NewService(store, &mapResolver{})
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	d := restarted.EvaluateDNS(identity, "chess.games.example")
	if d.Action != ActionNone {
		t.Fatalf("after restart: decision = %+v, want none (pause persisted)", d)
	}
}

func TestPauseAllPausesIncludesRevoked(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	pause, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	snap := svc.PauseSnapshot()
	if len(snap.Pauses) != 1 {
		t.Fatalf("active pauses = %d, want 1", len(snap.Pauses))
	}
	if len(svc.AllPauses()) != 1 {
		t.Fatalf("all pauses = %d, want 1", len(svc.AllPauses()))
	}
	if err := svc.RevokePause(pause.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	snap = svc.PauseSnapshot()
	if len(snap.Pauses) != 0 {
		t.Fatalf("active pauses after revoke = %d, want 0", len(snap.Pauses))
	}
	if len(svc.AllPauses()) != 1 {
		t.Fatalf("all pauses after revoke = %d, want 1 (includes revoked)", len(svc.AllPauses()))
	}
}

func TestPauseUnknownGroupRejected(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeGroup,
		GroupID:  "nonexistent",
		Duration: "1h",
	}, "admin")
	if err == nil {
		t.Fatal("expected error for unknown group")
	}
}

func TestPauseDomainEvaluationPath(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	if a := svc.EvaluateDomain(identity, "chess.games.example"); a != ActionBlock {
		t.Fatalf("EvaluateDomain before pause = %s, want block", a)
	}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if a := svc.EvaluateDomain(identity, "chess.games.example"); a != ActionNone {
		t.Fatalf("EvaluateDomain after pause = %s, want none", a)
	}
}

func TestPauseAndScheduleInteraction(t *testing.T) {
	svc := newTestService(t)
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	if err := svc.SaveGroups([]PolicyGroup{
		scheduledBlock("kids", sc, "games.example"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	_, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1h",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	svc.SetClock(mustTime(loc, "2026-03-16 12:00:00"))
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("active schedule + pause: decision = %+v, want none", d)
	}
	svc.SetClock(mustTime(loc, "2026-03-16 20:00:00"))
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.Action != ActionNone {
		t.Fatalf("inactive schedule + pause: decision = %+v, want none (schedule inactive)", d)
	}
	if d := svc.EvaluateDNS(identity, "chess.games.example"); d.RuleID != "" {
		t.Fatalf("inactive schedule + pause: decision = %+v, want no rule deciding", d)
	}
}

func TestPausePruneExpired(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", BlockedDomains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	pause, err := svc.CreatePause(CreatePauseInput{
		Scope:    PauseScopeDevice,
		DeviceID: "device-1",
		Duration: "1m",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	svc.SetClock(pause.Until.Add(MaxPauseDuration + time.Hour))
	pruned, err := svc.PruneExpiredPauses()
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	if len(svc.AllPauses()) != 0 {
		t.Fatalf("after prune: all pauses = %d, want 0", len(svc.AllPauses()))
	}
}
