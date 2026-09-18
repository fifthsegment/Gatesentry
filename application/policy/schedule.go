package policy

import (
	"fmt"
	"strings"
	"time"
)

// Schedule defines when a policy group's action is active. It replaces the
// single from/to TimeRestriction with weekdays, multiple windows, an explicit
// IANA time zone, and overnight (past-midnight) support. A nil schedule means
// the group is always active, preserving pre-PER-39 behavior.
type Schedule struct {
	// Timezone is the IANA time zone name (e.g. "America/New_York"). An
	// empty string defaults to UTC. The time zone is required for DST
	// transitions to land on the correct wall-clock day.
	Timezone string `json:"timezone"`
	// Weekdays lists the days of the week the schedule is active. An empty
	// slice means every day. Values use time.Weekday (Sunday=0 .. Saturday=6).
	Weekdays []time.Weekday `json:"weekdays,omitempty"`
	// Windows is the ordered list of time windows. Multiple windows allow
	// separate on/off periods in one day. An overnight window (From > To)
	// spans midnight: the portion after midnight belongs to the previous
	// day's weekday, so a Monday bedtime of 20:00-07:00 covers Tuesday
	// morning.
	Windows []TimeWindow `json:"windows"`
	// Preset records the built-in preset this schedule was derived from, if
	// any. It is informational; Windows and Weekdays are what enforcement
	// evaluates.
	Preset string `json:"preset,omitempty"`
}

// TimeWindow is one on-period within a schedule. From and To are "HH:MM"
// 24-hour wall-clock times in the schedule's time zone. When From > To the
// window crosses midnight.
type TimeWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ParseTimeWindow converts "HH:MM" to minutes-since-midnight. It returns
// ok=false on malformed input so callers can skip bad windows without
// aborting evaluation.
func parseTimeOfDay(s string) (minutes int, ok bool) {
	s = strings.TrimSpace(s)
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	h, err1 := atoi(s[:2])
	m, err2 := atoi(s[3:])
	if err1 != nil || err2 != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// atoi is a small parse helper that avoids importing strconv in the hot path.
func atoi(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// Validate checks that the schedule is well-formed: time zone is loadable,
// every window parses, and at least one window exists. An empty schedule
// (no windows) is invalid — use a nil Schedule on the group instead.
func (sc *Schedule) Validate() error {
	if sc == nil {
		return nil
	}
	if len(sc.Windows) == 0 {
		return fmt.Errorf("schedule needs at least one time window")
	}
	if sc.Timezone != "" {
		if _, err := time.LoadLocation(sc.Timezone); err != nil {
			return fmt.Errorf("schedule timezone %q: %w", sc.Timezone, err)
		}
	}
	for i, w := range sc.Windows {
		fromMin, ok := parseTimeOfDay(w.From)
		if !ok {
			return fmt.Errorf("window %d: from %q is not HH:MM", i, w.From)
		}
		toMin, ok := parseTimeOfDay(w.To)
		if !ok {
			return fmt.Errorf("window %d: to %q is not HH:MM", i, w.To)
		}
		if fromMin == toMin {
			return fmt.Errorf("window %d: from and to are identical", i)
		}
	}
	// Validate weekday values are in range (defensive; time.Weekday is an int).
	for i, wd := range sc.Weekdays {
		if wd < time.Sunday || wd > time.Saturday {
			return fmt.Errorf("weekday %d is out of range (0-6)", i)
		}
	}
	return nil
}

// isWeekdayActive reports whether the given day is in the schedule's weekday
// set. An empty set means every day is active.
func (sc *Schedule) isWeekdayActive(wd time.Weekday) bool {
	if len(sc.Weekdays) == 0 {
		return true
	}
	for _, d := range sc.Weekdays {
		if d == wd {
			return true
		}
	}
	return false
}

// IsActive reports whether the schedule is active at the given instant. It
// converts the instant to the schedule's time zone, then checks each window.
// Overnight windows (From > To) are split at midnight: the pre-midnight
// portion checks the current day's weekday, and the post-midnight portion
// checks the previous day's weekday, so a Monday 20:00-07:00 bedtime covers
// Tuesday morning.
func (sc *Schedule) IsActive(now time.Time) bool {
	if sc == nil {
		return true
	}
	loc := time.UTC
	if sc.Timezone != "" {
		if l, err := time.LoadLocation(sc.Timezone); err == nil {
			loc = l
		}
	}
	local := now.In(loc)
	wd := local.Weekday()
	tod := local.Hour()*60 + local.Minute() // time of day in minutes

	for _, w := range sc.Windows {
		fromMin, okF := parseTimeOfDay(w.From)
		toMin, okT := parseTimeOfDay(w.To)
		if !okF || !okT {
			continue
		}
		if fromMin <= toMin {
			// Same-day window: active between from and to on the same day.
			if tod >= fromMin && tod < toMin {
				if sc.isWeekdayActive(wd) {
					return true
				}
			}
		} else {
			// Overnight window (fromMin > toMin): active from fromMin to
			// midnight (today) and from midnight to toMin (yesterday).
			if tod >= fromMin {
				if sc.isWeekdayActive(wd) {
					return true
				}
			} else if tod < toMin {
				// Post-midnight portion belongs to the previous day's
				// schedule.
				prevWd := (wd + 6) % 7 // Sunday(0) -> Saturday(6), etc.
				if sc.isWeekdayActive(prevWd) {
					return true
				}
			}
		}
	}
	return false
}

// SchedulePreset is a named starting point for a schedule. Applying a preset
// creates a normal Schedule that can be edited independently.
type SchedulePreset struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Timezone    string         `json:"timezone"`
	Weekdays    []time.Weekday `json:"weekdays"`
	Windows     []TimeWindow   `json:"windows"`
}

// SchedulePresets returns the built-in preset catalog. The time zone is a
// placeholder; the caller should override it with the installation's time
// zone when applying a preset to a group.
func SchedulePresets() []SchedulePreset {
	return []SchedulePreset{
		{
			ID:          "bedtime",
			Name:        "Bedtime",
			Description: "Blocks from 8 PM to 7 AM on school nights (Sunday through Thursday). The overnight portion covers the next morning.",
			Weekdays:    []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday},
			Windows:     []TimeWindow{{From: "20:00", To: "07:00"}},
		},
		{
			ID:          "school-day",
			Name:        "School Day",
			Description: "Blocks during school hours, 8 AM to 3:30 PM, Monday through Friday.",
			Weekdays:    []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
			Windows:     []TimeWindow{{From: "08:00", To: "15:30"}},
		},
	}
}

// PresetByID returns the preset with the given ID, or nil if not found.
func PresetByID(id string) *SchedulePreset {
	for _, p := range SchedulePresets() {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

// ApplyPreset returns a Schedule derived from a preset, with the given time
// zone. The preset ID is recorded so the UI can show the origin.
func ApplyPreset(presetID, timezone string) (*Schedule, error) {
	p := PresetByID(presetID)
	if p == nil {
		return nil, fmt.Errorf("unknown schedule preset %q", presetID)
	}
	wd := make([]time.Weekday, len(p.Weekdays))
	copy(wd, p.Weekdays)
	ws := make([]TimeWindow, len(p.Windows))
	copy(ws, p.Windows)
	return &Schedule{
		Timezone: timezone,
		Weekdays: wd,
		Windows:  ws,
		Preset:   presetID,
	}, nil
}
