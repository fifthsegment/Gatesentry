package policy

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// LegacyDevice is the pre-policy device metadata shape. Owner and Category
// are display metadata, never authenticated identity; migration converts
// them into explicit, editable group assignments.
type LegacyDevice struct {
	ID       string
	Owner    string
	Category string
}

// MigratedGroupName returns a stable group ID for a legacy label.
func MigratedGroupName(label string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(label), "-"))
	if normalized == "" {
		return ""
	}
	return normalized
}

// MigrateLegacyDevices converts legacy category/owner metadata into policy
// groups and device assignments. Category wins because it is already a
// grouping field; owner is the fallback so no legacy label is lost. Existing
// user-based rules are untouched: they continue to apply in their existing
// precedence stage and migration only adds group records.
func MigrateLegacyDevices(devices map[string]LegacyDevice) ([]PolicyGroup, []DeviceAssignment, error) {
	groups := map[string]PolicyGroup{}
	assignments := make([]DeviceAssignment, 0, len(devices))
	for id, device := range devices {
		if id == "" {
			return nil, nil, fmt.Errorf("legacy device has empty ID")
		}
		label := device.Category
		if label == "" {
			label = device.Owner
		}
		if label == "" {
			continue
		}
		groupName := MigratedGroupName(label)
		if groupName == "" {
			continue
		}
		groupID := "category-" + groupName
		if device.Category == "" {
			groupID = "owner-" + groupName
		}
		now := time.Now().UTC()
		if _, exists := groups[groupID]; !exists {
			kind := "category"
			if device.Category == "" {
				kind = "owner"
			}
			groups[groupID] = PolicyGroup{
				ID:          groupID,
				Name:        label,
				Description: "Migrated from legacy device " + kind + " metadata; edit before relying on it.",
				UpdatedAt:   now,
				CreatedAt:   now,
			}
		}
		assignments = append(assignments, DeviceAssignment{DeviceID: id, GroupID: groupID, AssignedAt: now})
	}
	ordered := make([]PolicyGroup, 0, len(groups))
	for _, group := range groups {
		ordered = append(ordered, group)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	sort.Slice(assignments, func(i, j int) bool { return assignments[i].DeviceID < assignments[j].DeviceID })
	return ordered, assignments, nil
}
