package GatesentryTypes

// RuleAction defines what action to take when a rule matches
type RuleAction string

const (
	RuleActionAllow RuleAction = "allow"
	RuleActionBlock RuleAction = "block"
)

// MITMAction defines whether to perform SSL MITM on matching traffic
type MITMAction string

const (
	MITMActionEnable  MITMAction = "enable"
	MITMActionDisable MITMAction = "disable"
	MITMActionDefault MITMAction = "default" // Use global setting
)

// BlockType defines what type of blocking to apply after MITM
type BlockType string

const (
	BlockTypeNone        BlockType = "none"
	BlockTypeContentType BlockType = "content_type"
	BlockTypeURLRegex    BlockType = "url_regex"
	BlockTypeBoth        BlockType = "both"
	BlockTypeDomainList  BlockType = "domain_list" // Block embedded resources by domain list membership
	BlockTypeAll         BlockType = "all"         // All three: content_type + url_regex + domain_list
)

// Rule represents a filtering rule based on domain/SNI
type Rule struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Enabled    bool       `json:"enabled"`
	Priority   int        `json:"priority"` // Lower number = higher priority
	Domain     string     `json:"domain"`   // Legacy single pattern (still works, backward compat)
	Action     RuleAction `json:"action"`   // allow or block
	MITMAction MITMAction `json:"mitm_action"`
	BlockType  BlockType  `json:"block_type"`

	// Domain matching — multiple patterns and/or domain list membership (Phase 3)
	// A rule matches if ANY of Domain, DomainPatterns, or DomainLists match.
	DomainPatterns []string `json:"domain_patterns,omitempty"` // Multiple wildcard patterns (e.g., ["*.ads.com", "tracker.io"])
	DomainLists    []string `json:"domain_lists,omitempty"`    // Domain List IDs to match against (O(1) index lookup)

	// Content-Type blocking (when BlockType includes content_type)
	BlockedContentTypes []string `json:"blocked_content_types"` // e.g., ["image/jpeg", "video/mp4"]

	// URL path regex blocking (when BlockType includes url_regex)
	URLRegexPatterns []string `json:"url_regex_patterns"` // e.g., ["/ads/.*", "/tracker.*"]

	// Domain list content blocking (when BlockType includes domain_list)
	// Block embedded resources (images, scripts, etc.) whose domain is in any of these lists.
	ContentDomainLists []string `json:"content_domain_lists,omitempty"` // Domain List IDs for embedded resource blocking

	// Optional: Time-based restrictions
	TimeRestriction *TimeRestriction `json:"time_restriction,omitempty"`

	// Optional: User-based restrictions
	Users []string `json:"users,omitempty"` // Empty means all users

	// Metadata
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// TimeRestriction defines time-based rule activation
type TimeRestriction struct {
	From string `json:"from"` // Format: "HH:MM"
	To   string `json:"to"`   // Format: "HH:MM"
}

// RuleList is a collection of rules
type RuleList struct {
	Rules []Rule `json:"rules"`
}

// RuleMatch represents the result of matching a request against rules
type RuleMatch struct {
	Matched                 bool
	Rule                    *Rule
	ShouldMITM              bool
	ShouldBlock             bool
	BlockContentTypes       []string
	BlockURLRegexes         []string
	BlockContentDomainLists []string // Domain List IDs — block sub-requests whose domain is in these lists
}
