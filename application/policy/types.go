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

// DefaultGroupID is the built-in policy every request falls back to when it
// does not resolve to an assigned policy: an unassigned device, a device whose
// identity is unknown, stale, or shared behind NAT, and a proxy user no policy
// names. It always exists, cannot be deleted, and starts empty, so it changes
// nothing until an administrator adds to it.
const DefaultGroupID = "default"

// PolicyGroup is one named policy. A request resolves to exactly one policy:
// the one its device or proxy user is assigned to, or the default policy.
// Groups are durable policy records; they never own device observations.
// Enforcement reads them through the policy service.
//
// A policy answers "what can this device reach" in three layers, evaluated in
// this order, first match wins:
//
//  1. Rules, top to bottom. A rule names its own target (domains, categories,
//     or all traffic) and can be limited to a schedule or to proxy users, so a
//     policy can say "block everything at bedtime" or "allow YouTube after
//     school" without a second policy.
//  2. AllowedDomains, which always win over the blocked lists below and over
//     the gateway-wide blocklist and categories.
//  3. BlockedDomains and BlockedCategories.
//
// Anything no layer matches falls through to the gateway-wide blocklist and
// categories, which apply to every device.
type PolicyGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// BlockedCategories are catalog category IDs (see CategoryCatalog) whose
	// self-updating domain feeds this policy blocks. A category covers the
	// listed registrable domains and their subdomains.
	BlockedCategories []string `json:"blocked_categories,omitempty"`
	// BlockedDomains are domain patterns this policy blocks. A plain domain
	// also covers its subdomains; see matchDomain.
	BlockedDomains []string `json:"blocked_domains,omitempty"`
	// AllowedDomains are domain patterns this policy always allows. They win
	// over the blocked lists and the gateway-wide blocklist, so they are how an
	// administrator recovers a false positive for everyone on the policy.
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	// Rules are evaluated top to bottom before the lists above; the first
	// active rule whose target covers the request decides.
	Rules []GroupRule `json:"rules,omitempty"`
	// SafeSearch forces the supported search engines and YouTube into their
	// restricted modes by answering their DNS names with the providers'
	// safe-search endpoints.
	SafeSearch bool `json:"safe_search,omitempty"`
	// Users assigns authenticated proxy users to this policy. An
	// authenticated user's policy wins over the device's, because the login
	// is the more specific identity. IP addresses are never valid entries.
	Users []string `json:"users,omitempty"`
	// Priority breaks ties when one proxy user is listed on several policies;
	// lower numbers win.
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeviceAssignment records a stable device-to-group assignment.
type DeviceAssignment struct {
	DeviceID   string    `json:"device_id"`
	GroupID    string    `json:"group_id"`
	AssignedAt time.Time `json:"assigned_at"`
}

// RuleTarget is what a rule applies to. AllTraffic covers every domain, which
// is how "no internet at bedtime" is written; otherwise the rule covers the
// listed domain patterns and categories.
type RuleTarget struct {
	AllTraffic bool     `json:"all_traffic,omitempty"`
	Domains    []string `json:"domains,omitempty"`
	Categories []string `json:"categories,omitempty"`
}

// GroupRule is one enforcement rule inside a policy. A rule is decided by the
// layer that can see its conditions: a rule with only a target, a schedule, or
// a user scope is decided by DNS and the proxy alike, while URL patterns and
// response media types exist only inside a decrypted request and are decided
// by the proxy.
type GroupRule struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
	// Action is the outcome for requests this rule matches.
	Action PolicyAction `json:"action"`
	Target RuleTarget   `json:"target"`
	// URLRegexes are Go regular expressions tested against the full request
	// URL. A block rule with URL patterns blocks only matching URLs, so DNS
	// leaves the domain resolvable and the proxy decides per request.
	URLRegexes []string `json:"url_regexes,omitempty"`
	// BlockedContentTypes are response media types, matched as prefixes so
	// "video/" covers every video type. Like URL patterns they narrow a block
	// rule to matching responses and are decided by the proxy.
	BlockedContentTypes []string `json:"blocked_content_types,omitempty"`
	// Schedule limits the rule to the listed windows. Nil means always active.
	// Windows are evaluated in the schedule's own time zone.
	Schedule *Schedule `json:"schedule,omitempty"`
	// Users limits the rule to authenticated proxy users. Empty means every
	// request the policy applies to. DNS never sees a user, so a user-scoped
	// rule is decided by the proxy only.
	Users []string `json:"users,omitempty"`
}

// ProxyMatch is the proxy enforcement outcome for one request. The field names
// are a contract: gatesentryproxy reads Matched, ShouldBlock, ShouldMITM,
// BlockURLRegexes, BlockContentTypes, and Reason from this struct by
// reflection, so the policy service answers in the shape the proxy consumes.
//
// Matched reports whether a policy decided this request at all. Matched false
// means no policy applies and the gateway's own settings stay in charge,
// including TLS inspection. ShouldBlock denies the whole request by domain.
// BlockURLRegexes and BlockContentTypes narrow a block: the proxy inspects the
// connection and denies only a request whose URL matches a pattern (tested
// before the request leaves the gateway) or whose response has a listed media
// type. Allowed reports an explicit allow, which exempts the request from the
// gateway-wide URL blocklist.
type ProxyMatch struct {
	Matched           bool
	Allowed           bool
	GroupID           string
	RuleID            string
	MatchedDomain     string
	Reason            string
	ShouldBlock       bool
	ShouldMITM        bool
	BlockURLRegexes   []string
	BlockContentTypes []string
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

// DNSDecision is the DNS adapter outcome. DNS decides by domain and client
// only; ProxyOnly lists the conditions of the matched rule that DNS cannot see
// (URL patterns, response types, user scope), which the proxy enforces.
type DNSDecision struct {
	Action        PolicyAction
	GroupID       string
	MatchedDomain string
	// RuleID names the rule that decided the query, when a rule rather than
	// the policy's lists produced the outcome.
	RuleID string
	// ProxyOnly lists the conditions DNS could not evaluate for this query;
	// a rule carrying them never decides a DNS query.
	ProxyOnly []string
	// SafeSearch reports that the resolved policy enforces safe search.
	SafeSearch bool
	Reason     string
}
