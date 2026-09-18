package policy

import (
	"testing"
	"time"
)

// mustTime parses a wall-clock instant in a fixed location for deterministic
// schedule tests. Panics on parse failure so a typo surfaces immediately.
func mustTime(loc *time.Location, s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc)
	if err != nil {
		panic(err)
	}
	return t
}

func TestScheduleValidateRejectsBadInput(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	_ = loc

	tests := []struct {
		name    string
		sc      *Schedule
		wantErr bool
	}{
		{"nil schedule", nil, false},
		{"no windows", &Schedule{Timezone: "UTC"}, true},
		{"bad timezone", &Schedule{Timezone: "Mars/Olympus", Windows: []TimeWindow{{From: "09:00", To: "17:00"}}}, true},
		{"bad from format", &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "9:00", To: "17:00"}}}, true},
		{"bad to format", &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "09:00", To: "1700"}}}, true},
		{"identical from/to", &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "09:00", To: "09:00"}}}, true},
		{"valid same-day", &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "09:00", To: "17:00"}}}, false},
		{"valid overnight", &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "20:00", To: "07:00"}}}, false},
		{"valid multiple windows", &Schedule{Timezone: "UTC", Windows: []TimeWindow{{From: "09:00", To: "12:00"}, {From: "13:00", To: "15:30"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sc.Validate()
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestScheduleNilIsAlwaysActive(t *testing.T) {
	var sc *Schedule
	if !sc.IsActive(time.Now()) {
		t.Fatal("nil schedule must be active at all times")
	}
}

func TestScheduleSameDayWindow(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	// Inside the window.
	if !sc.IsActive(mustTime(loc, "2026-03-16 12:00:00")) {
		t.Fatal("expected active at noon")
	}
	// Before the window.
	if sc.IsActive(mustTime(loc, "2026-03-16 08:59:00")) {
		t.Fatal("expected inactive before 09:00")
	}
	// At the boundary (inclusive start, exclusive end).
	if !sc.IsActive(mustTime(loc, "2026-03-16 09:00:00")) {
		t.Fatal("expected active exactly at 09:00:00 (inclusive start)")
	}
	if sc.IsActive(mustTime(loc, "2026-03-16 17:00:00")) {
		t.Fatal("expected inactive exactly at 17:00:00 (exclusive end)")
	}
}

func TestScheduleOvernightWindowSpansMidnight(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "20:00", To: "07:00"}},
	}
	// Monday 21:00 — inside the overnight window.
	if !sc.IsActive(mustTime(loc, "2026-03-16 21:00:00")) {
		t.Fatal("expected active Monday 21:00 (pre-midnight portion)")
	}
	// Tuesday 06:00 — also inside the overnight window (post-midnight portion).
	if !sc.IsActive(mustTime(loc, "2026-03-17 06:00:00")) {
		t.Fatal("expected active Tuesday 06:00 (post-midnight portion)")
	}
	// Tuesday 08:00 — outside.
	if sc.IsActive(mustTime(loc, "2026-03-17 08:00:00")) {
		t.Fatal("expected inactive Tuesday 08:00")
	}
	// Monday 19:00 — before window starts.
	if sc.IsActive(mustTime(loc, "2026-03-16 19:00:00")) {
		t.Fatal("expected inactive Monday 19:00 (before window)")
	}
}

func TestScheduleWeekdaySelection(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		// Monday through Friday only.
		Weekdays: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	// Monday noon — active.
	if !sc.IsActive(mustTime(loc, "2026-03-16 12:00:00")) {
		t.Fatal("expected active on Monday within weekday set")
	}
	// Saturday noon — inactive.
	if sc.IsActive(mustTime(loc, "2026-03-21 12:00:00")) {
		t.Fatal("expected inactive on Saturday (not in weekday set)")
	}
	// Sunday noon — inactive.
	if sc.IsActive(mustTime(loc, "2026-03-22 12:00:00")) {
		t.Fatal("expected inactive on Sunday (not in weekday set)")
	}
}

func TestScheduleOvernightWeekdayBelongsToPreviousDay(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		// Sunday through Thursday nights (school nights).
		Weekdays: []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday},
		Windows:  []TimeWindow{{From: "20:00", To: "07:00"}},
	}
	// Monday 21:00 (Monday is in the set) — active.
	if !sc.IsActive(mustTime(loc, "2026-03-16 21:00:00")) {
		t.Fatal("expected active Monday 21:00 (Monday in set)")
	}
	// Tuesday 06:00 — the post-midnight portion belongs to Monday's schedule.
	// Monday is in the set, so this should be active.
	if !sc.IsActive(mustTime(loc, "2026-03-17 06:00:00")) {
		t.Fatal("expected active Tuesday 06:00 (post-midnight belongs to Monday, which is in set)")
	}
	// Saturday 06:00 — post-midnight belongs to Friday's schedule. Friday is
	// NOT in the set, so this should be inactive.
	if sc.IsActive(mustTime(loc, "2026-03-21 06:00:00")) {
		t.Fatal("expected inactive Saturday 06:00 (post-midnight belongs to Friday, not in set)")
	}
}

func TestScheduleMultipleWindows(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows: []TimeWindow{
			{From: "09:00", To: "12:00"},
			{From: "13:00", To: "15:30"},
		},
	}
	// First window.
	if !sc.IsActive(mustTime(loc, "2026-03-16 10:00:00")) {
		t.Fatal("expected active in first window 10:00")
	}
	// Gap between windows.
	if sc.IsActive(mustTime(loc, "2026-03-16 12:30:00")) {
		t.Fatal("expected inactive in gap 12:30")
	}
	// Second window.
	if !sc.IsActive(mustTime(loc, "2026-03-16 14:00:00")) {
		t.Fatal("expected active in second window 14:00")
	}
	// After second window.
	if sc.IsActive(mustTime(loc, "2026-03-16 16:00:00")) {
		t.Fatal("expected inactive after 15:30")
	}
}

func TestScheduleDSTTransitionSpringForward(t *testing.T) {
	// Spring forward in America/New_York: 2026-03-08 02:00 -> 03:00.
	// A schedule active 09:00-17:00 should still work correctly across the
	// DST transition because the wall-clock hours don't land in the skipped
	// hour. Verify the schedule is evaluated in the correct local time.
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	// The day before DST (still EST).
	if !sc.IsActive(mustTime(loc, "2026-03-07 10:00:00")) {
		t.Fatal("expected active day before DST at 10:00 EST")
	}
	// The day of DST (now EDT).
	if !sc.IsActive(mustTime(loc, "2026-03-08 10:00:00")) {
		t.Fatal("expected active day of DST at 10:00 EDT")
	}
	// UTC instant that is 10:00 EDT on DST day.
	utc := time.Date(2026, 3, 8, 14, 0, 0, 0, time.UTC) // 14:00 UTC = 10:00 EDT
	if !sc.IsActive(utc) {
		t.Fatal("expected 14:00 UTC = 10:00 EDT to be active on DST day")
	}
}

func TestScheduleDSTTransitionFallBack(t *testing.T) {
	// Fall back in America/New_York: 2026-11-01 02:00 -> 01:00.
	// An overnight window 20:00-07:00 should cover both 01:00 instances
	// (the ambiguous hour) because wall-clock 01:00 is in range regardless
	// of which instance it is.
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "20:00", To: "07:00"}},
	}
	// 01:30 on fall-back day — wall-clock time is in the post-midnight portion.
	if !sc.IsActive(mustTime(loc, "2026-11-01 01:30:00")) {
		t.Fatal("expected active at 01:30 on fall-back day")
	}
}

func TestScheduleEmptyWeekdaysMeansEveryDay(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	sc := &Schedule{
		Timezone: "UTC",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	for _, day := range []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday} {
		// Find a date with this weekday.
		t2 := time.Date(2026, 3, int(day)+1, 12, 0, 0, 0, loc)
		if !sc.IsActive(t2) {
			t.Fatalf("expected active on %s with empty weekdays (every day)", day.String())
		}
	}
}

func TestScheduleOverlapBetweenWindows(t *testing.T) {
	// Two overlapping windows: 09:00-17:00 and 16:00-20:00.
	// The overlap region (16:00-17:00) should be active.
	loc, _ := time.LoadLocation("UTC")
	sc := &Schedule{
		Timezone: "UTC",
		Windows: []TimeWindow{
			{From: "09:00", To: "17:00"},
			{From: "16:00", To: "20:00"},
		},
	}
	// In the overlap.
	if !sc.IsActive(mustTime(loc, "2026-03-16 16:30:00")) {
		t.Fatal("expected active in overlap region 16:30")
	}
	// In first only.
	if !sc.IsActive(mustTime(loc, "2026-03-16 10:00:00")) {
		t.Fatal("expected active in first-only region 10:00")
	}
	// In second only.
	if !sc.IsActive(mustTime(loc, "2026-03-16 18:00:00")) {
		t.Fatal("expected active in second-only region 18:00")
	}
}

func TestSchedulePresetsExist(t *testing.T) {
	presets := SchedulePresets()
	if len(presets) < 2 {
		t.Fatalf("expected at least 2 presets, got %d", len(presets))
	}
	bedtime := PresetByID("bedtime")
	if bedtime == nil {
		t.Fatal("bedtime preset not found")
	}
	if bedtime.Name != "Bedtime" {
		t.Fatalf("bedtime name = %q, want Bedtime", bedtime.Name)
	}
	schoolDay := PresetByID("school-day")
	if schoolDay == nil {
		t.Fatal("school-day preset not found")
	}
	if schoolDay.Name != "School Day" {
		t.Fatalf("school-day name = %q, want School Day", schoolDay.Name)
	}
}

func TestApplyPresetCreatesEditableSchedule(t *testing.T) {
	sc, err := ApplyPreset("bedtime", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Preset != "bedtime" {
		t.Fatalf("preset = %q, want bedtime", sc.Preset)
	}
	if sc.Timezone != "America/New_York" {
		t.Fatalf("timezone = %q, want America/New_York", sc.Timezone)
	}
	if len(sc.Windows) != 1 {
		t.Fatalf("windows = %d, want 1", len(sc.Windows))
	}
	if sc.Windows[0].From != "20:00" || sc.Windows[0].To != "07:00" {
		t.Fatalf("window = %+v, want 20:00-07:00", sc.Windows[0])
	}
	// Editing the applied schedule must not affect the preset.
	sc.Windows[0].From = "21:00"
	original := PresetByID("bedtime")
	if original.Windows[0].From != "20:00" {
		t.Fatal("editing applied schedule leaked into the preset")
	}
}

func TestApplyPresetUnknownID(t *testing.T) {
	_, err := ApplyPreset("nonexistent", "UTC")
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
}

// scheduleTestHelper creates a group with a schedule and evaluates a DNS
// decision at a controlled time.
func scheduleTestHelper(t *testing.T, svc *Service, groupID string, schedule *Schedule, now time.Time, domain string) DNSDecision {
	t.Helper()
	svc.SetClock(now)
	groups := svc.Snapshot().Groups
	g := groups[groupID]
	g.Schedule = schedule
	if err := svc.SaveGroups([]PolicyGroup{g}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	identity := Identity{GroupID: groupID, Source: SourceDevice, DeviceID: "device-1"}
	return svc.EvaluateDNS(identity, domain)
}

func TestScheduleInactiveSuppressesBlock(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Action: ActionBlock, Domains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	// At 20:00 (outside the window) the block should not apply.
	decision := scheduleTestHelper(t, svc, "kids", sc, mustTime(loc, "2026-03-16 20:00:00"), "chess.games.example")
	if decision.Action != ActionNone {
		t.Fatalf("decision = %+v, want none (schedule inactive)", decision)
	}
	if decision.Reason != "group schedule inactive" {
		t.Fatalf("reason = %q, want schedule inactive", decision.Reason)
	}
}

func TestScheduleActiveAppliesBlock(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Action: ActionBlock, Domains: []string{"*.games.example"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	// At 12:00 (inside the window) the block should apply.
	decision := scheduleTestHelper(t, svc, "kids", sc, mustTime(loc, "2026-03-16 12:00:00"), "chess.games.example")
	if decision.Action != ActionBlock {
		t.Fatalf("decision = %+v, want block (schedule active)", decision)
	}
}

func TestScheduleRestartDuringActivePeriod(t *testing.T) {
	svc := newTestService(t)
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "kids", Action: ActionBlock, Domains: []string{"*.games.example"}, Schedule: sc},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	// Simulate restart: create a new service from the same storage.
	restarted, err := NewService(svc.storage, &mapResolver{})
	if err != nil {
		t.Fatal(err)
	}
	// Set the clock to an active time.
	restarted.SetClock(mustTime(loc, "2026-03-16 12:00:00"))
	identity := Identity{GroupID: "kids", Source: SourceDevice, DeviceID: "device-1"}
	decision := restarted.EvaluateDNS(identity, "chess.games.example")
	if decision.Action != ActionBlock {
		t.Fatalf("decision after restart = %+v, want block", decision)
	}
	// Set the clock to an inactive time.
	restarted.SetClock(mustTime(loc, "2026-03-16 20:00:00"))
	decision = restarted.EvaluateDNS(identity, "chess.games.example")
	if decision.Action != ActionNone {
		t.Fatalf("decision after restart (inactive) = %+v, want none", decision)
	}
}

func TestScheduleDoesNotAffectAllowGroups(t *testing.T) {
	svc := newTestService(t)
	loc, _ := time.LoadLocation("America/New_York")
	sc := &Schedule{
		Timezone: "America/New_York",
		Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
	}
	if err := svc.SaveGroups([]PolicyGroup{
		{ID: "adults", Action: ActionAllow, Domains: []string{"tracker.example"}, Schedule: sc},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	// Even when schedule is inactive, an allow group falls through to
	// ActionNone (it does not block). The schedule only gates whether the
	// group expresses its action.
	svc.SetClock(mustTime(loc, "2026-03-16 20:00:00"))
	identity := Identity{GroupID: "adults", Source: SourceAuthUser}
	decision := svc.EvaluateDNS(identity, "tracker.example")
	if decision.Action != ActionNone {
		t.Fatalf("decision = %+v, want none (allow group with inactive schedule falls through)", decision)
	}
}
