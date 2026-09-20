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
	// Categories are catalog category IDs (see CategoryCatalog) whose
	// self-updating domain feeds this group enforces with Action. A category
	// covers the listed registrable domains and their subdomains, so a group
	// does not need one pattern per host.
	Categories []string `json:"categories,omitempty"`
	// Rules are the group's enforcement rules. A rule does not carry its own
	// domain list: Domains and Categories are the coverage a rule refines, so
	// "what does this apply to" has one answer per group. Rules add the
	// conditions DNS cannot see (URL patterns, response media types, TLS
	// inspection) and can scope the group's action to a time window or to
	// authenticated proxy users.
	Rules []GroupRule `json:"rules,omitempty"`
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

// TLS inspection settings a group rule can request for the traffic it covers.
const (
	// MITMActionDefault keeps the gateway's own HTTPS inspection setting.
	MITMActionDefault = "default"
	// MITMActionEnable inspects the traffic this rule covers.
	MITMActionEnable = "enable"
	// MITMActionDisable passes the traffic this rule covers through
	// uninspected.
	MITMActionDisable = "disable"
)

// GroupRule is one enforcement rule inside a policy group. A rule is decided
// by the layer that can see its conditions: a rule with only a schedule, a
// user scope, or an inspection setting is decided by DNS and the proxy alike,
// while URL patterns and response media types exist only inside a decrypted
// request and are decided by the proxy.
type GroupRule struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
	// Action is the outcome for requests this rule matches. ActionAllow is an
	// exception inside the group; ActionBlock denies the request.
	Action PolicyAction `json:"action"`
	// MITMAction is the TLS inspection setting for the traffic this rule
	// covers: "enable", "disable", or "default" (the gateway setting). A rule
	// with URLRegexes or BlockedContentTypes always inspects, because those
	// conditions exist only inside a decrypted request.
	MITMAction string `json:"mitm_action,omitempty"`
	// URLRegexes are Go regular expressions tested against the request URL.
	URLRegexes []string `json:"url_regexes,omitempty"`
	// BlockedContentTypes are response media types, matched as substrings so
	// "video/" covers every video type.
	BlockedContentTypes []string `json:"blocked_content_types,omitempty"`
	// Schedule limits the rule to the listed windows. Nil means always active.
	// Windows are evaluated in the schedule's own time zone.
	Schedule *Schedule `json:"schedule,omitempty"`
	// Users limits the rule to authenticated proxy users. Empty means every
	// user the group applies to.
	Users []string `json:"users,omitempty"`
	// Priority orders rules inside one group; lower numbers win.
	Priority int `json:"priority"`
}

// ProxyMatch is the proxy enforcement outcome for one request. The field names
// are a contract: gatesentryproxy reads Matched, ShouldBlock, ShouldMITM,
// BlockURLRegexes, and BlockContentTypes from this struct by reflection, so
// the policy service answers in the shape the proxy already consumes.
//
// Matched reports whether a group decided this request at all. Matched false
// means no policy applies and the gateway's own settings stay in charge,
// including TLS inspection. ShouldBlock denies the request by domain.
// BlockURLRegexes and BlockContentTypes are conditions the proxy tests after
// the response is available; ShouldBlock is then true only when a URL pattern
// also matched, so a rule can deny one path instead of a whole host.
type ProxyMatch struct {
	Matched           bool
	GroupID           string
	RuleID            string
	MatchedDomain     string
	Reason            string
	ShouldBlock       bool
	ShouldMITM        bool
	BlockURLRegexes   []string
	BlockContentTypes []string
	// UnresolvedConditions lists rule conditions that could not be decided for
	// the context this match was asked about.
	UnresolvedConditions []string
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
	// RuleID names the group rule that decided the query, when a rule rather
	// than the group action produced the outcome.
	RuleID string
	// InapplicableConditions lists proxy-only conditions (url_regex,
	// content_type, mitm) that DNS cannot enforce for this group.
	InapplicableConditions []string
	Reason                 string
}
