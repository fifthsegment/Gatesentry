package policy

import "time"

// PolicyAction is the enforcement outcome a policy group applies to a domain.
type PolicyAction string

const (
	// ActionNone means the group expresses no opinion about the domain and
	// the next precedence stage decides.
	ActionNone PolicyAction = ""
	// ActionAllow explicitly permits resolution/proxying for the group.
	ActionAllow PolicyAction = "allow"
	// ActionBlock explicitly denies resolution/proxying for the group.
	ActionBlock PolicyAction = "block"
)

// PolicyGroup is one named policy group. Groups are durable policy records;
// they never own device observations. Enforcement reads them through the
// policy service.
type PolicyGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Domains are wildcard domain patterns (same syntax as Rule.Domain:
	// exact match or "*." suffix).
	Domains []string `json:"domains,omitempty"`
	// Action applies to all Domains entries. ActionNone keeps the group
	// informational-only and is equivalent to no group policy.
	Action PolicyAction `json:"action"`
	// UnknownDevicePolicy is the precedence rule for how this group treats
	// devices that resolve to "unknown" (no stable device identity):
	// "default" keeps existing global behavior, "block" applies the group
	// block list, "allow" exempts the group block list.
	UnknownDevicePolicy string `json:"unknown_device_policy,omitempty"`
	// Users maps authenticated proxy users to this group. The map is keyed by
	// the authenticated username; IP addresses are never valid keys.
	Users []string `json:"users,omitempty"`
	// Priority orders groups when a device has an explicit assignment to
	// multiple groups; lower numbers win. Within one group, domain
	// evaluation is first-match over the persisted list order.
	Priority  int       `json:"priority"`
	Schedule  *Schedule `json:"schedule,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeviceAssignment records a stable device-to-group assignment.
type DeviceAssignment struct {
	DeviceID   string    `json:"device_id"`
	GroupID    string    `json:"group_id"`
	AssignedAt time.Time `json:"assigned_at"`
}

// PolicySnapshot is the immutable evaluation view of the persisted policy
// state. Adapters read a snapshot per evaluation; mutations replace the whole
// snapshot atomically.
type PolicySnapshot struct {
	Groups      map[string]PolicyGroup
	Assignments map[string]string // device ID -> group ID
	Version     int
	UpdatedAt   time.Time
}

// PolicyDocument is the persisted JSON shape.
type PolicyDocument struct {
	Version     int                `json:"version"`
	Groups      []PolicyGroup      `json:"groups"`
	Assignments []DeviceAssignment `json:"assignments"`
	// MigratedFrom records the source of a one-time migration so repeated
	// loads do not re-apply legacy owner/category mappings.
	MigratedFrom string    `json:"migrated_from,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// SourceKind identifies what identity a request was resolved to.
type SourceKind string

const (
	SourceDevice      SourceKind = "device"
	SourceAuthUser    SourceKind = "auth_user"
	SourceUnknown     SourceKind = "unknown"
	SourceUnknownNAT  SourceKind = "unknown_nat"
	SourceStaleDevice SourceKind = "stale_device"
)

// Identity is the resolved request context. IP is a lookup key only, never a
// durable identity: it is not persisted and never treated as authenticated.
type Identity struct {
	DeviceID string     `json:"device_id,omitempty"`
	AuthUser string     `json:"auth_user,omitempty"`
	GroupID  string     `json:"group_id,omitempty"`
	Source   SourceKind `json:"source"`
	// Explanation describes why this identity was chosen. It never contains
	// stable identifiers or credentials.
	Explanation string `json:"explanation,omitempty"`
}

// DNSDecision is the DNS adapter outcome including the conditions that DNS
// cannot evaluate (URL paths, MIME types, TLS content inspection).
type DNSDecision struct {
	Action        PolicyAction
	GroupID       string
	MatchedDomain string
	// InapplicableConditions lists proxy-only conditions (url_regex,
	// content_type, mitm) that DNS cannot enforce for this group.
	InapplicableConditions []string
	Reason                 string
}
