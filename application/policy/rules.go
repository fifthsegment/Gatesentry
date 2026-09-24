package policy

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"
)

// Conditions a rule can carry that only the proxy can evaluate. DNS reports
// them as proxy-only instead of pretending a domain decision enforced them.
const (
	ConditionURL         = "url_regex"
	ConditionContentType = "content_type"
	ConditionUser        = "user"
)

// NormalizeDomainPattern canonicalizes one domain entry typed by an
// administrator: lowercased, without a scheme, path, port, or trailing dot, and
// with a leading "*." kept. It rejects entries that cannot be a host name, so a
// typo is refused instead of being stored as a pattern that never matches.
func NormalizeDomainPattern(raw string) (string, error) {
	pattern := strings.ToLower(strings.TrimSpace(raw))
	if pattern == "" {
		return "", nil
	}
	if idx := strings.Index(pattern, "://"); idx >= 0 {
		pattern = pattern[idx+3:]
	}
	if idx := strings.IndexAny(pattern, "/?#"); idx >= 0 {
		pattern = pattern[:idx]
	}
	if host, _, err := net.SplitHostPort(pattern); err == nil {
		pattern = host
	}
	pattern = strings.TrimSuffix(pattern, ".")
	wildcard := strings.HasPrefix(pattern, "*.")
	host := strings.TrimPrefix(pattern, "*.")
	if host == "" || strings.ContainsAny(host, " *\t") {
		return "", fmt.Errorf("%q is not a domain", strings.TrimSpace(raw))
	}
	if wildcard {
		return "*." + host, nil
	}
	return host, nil
}

func normalizeDomainList(label string, raw []string) ([]string, error) {
	seen := make(map[string]bool, len(raw))
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		pattern, err := NormalizeDomainPattern(entry)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		if pattern == "" || seen[pattern] {
			continue
		}
		seen[pattern] = true
		out = append(out, pattern)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeUsers(raw []string) ([]string, error) {
	out := make([]string, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	for _, user := range raw {
		user = strings.TrimSpace(user)
		if user == "" || seen[user] {
			continue
		}
		if isAddressLike(user) {
			return nil, fmt.Errorf("%q is an address, not a proxy user; assign the device instead", user)
		}
		seen[user] = true
		out = append(out, user)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// NormalizeGroup validates a policy before it is persisted and returns the
// canonical form. A malformed entry is a rejection, never a record that
// silently matches nothing.
func NormalizeGroup(group PolicyGroup) (PolicyGroup, error) {
	group.Name = strings.TrimSpace(group.Name)
	group.Description = strings.TrimSpace(group.Description)
	if group.Name == "" {
		return group, errors.New("policy needs a name")
	}
	var err error
	if group.BlockedCategories, err = NormalizeCategories(group.BlockedCategories); err != nil {
		return group, err
	}
	if group.BlockedDomains, err = normalizeDomainList("blocked domains", group.BlockedDomains); err != nil {
		return group, err
	}
	if group.AllowedDomains, err = normalizeDomainList("allowed domains", group.AllowedDomains); err != nil {
		return group, err
	}
	if group.Users, err = normalizeUsers(group.Users); err != nil {
		return group, err
	}
	if group.Rules, err = NormalizeGroupRules(group.Rules); err != nil {
		return group, err
	}
	return group, nil
}

// NormalizeGroupRules validates rules and returns the canonical form: trimmed
// values, lowercased media types, and an ID on every rule. Disabled rules are
// kept as written but still have to be valid, so enabling one later cannot
// produce a rule the evaluator would refuse.
func NormalizeGroupRules(rules []GroupRule) ([]GroupRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	normalized := make([]GroupRule, 0, len(rules))
	for i, rule := range rules {
		n := i + 1
		rule.Name = strings.TrimSpace(rule.Name)
		if rule.ID == "" {
			rule.ID = newGroupID()
		}
		switch rule.Action {
		case ActionAllow, ActionBlock:
		default:
			return nil, fmt.Errorf("rule %d needs an allow or block action", n)
		}
		var err error
		if rule.Target.Domains, err = normalizeDomainList(fmt.Sprintf("rule %d", n), rule.Target.Domains); err != nil {
			return nil, err
		}
		if rule.Target.Categories, err = NormalizeCategories(rule.Target.Categories); err != nil {
			return nil, fmt.Errorf("rule %d: %w", n, err)
		}
		if rule.Target.AllTraffic {
			// "All traffic" is the whole target; a list next to it would
			// suggest a narrower rule than the one that is enforced.
			rule.Target.Domains = nil
			rule.Target.Categories = nil
		}
		if rule.Enabled && !rule.Target.AllTraffic && len(rule.Target.Domains) == 0 && len(rule.Target.Categories) == 0 {
			return nil, fmt.Errorf("rule %d needs a target: all traffic, domains, or categories", n)
		}
		patterns := make([]string, 0, len(rule.URLRegexes))
		for _, pattern := range rule.URLRegexes {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return nil, fmt.Errorf("rule %d URL pattern %q is not a valid regular expression", n, pattern)
			}
			patterns = append(patterns, pattern)
		}
		types := make([]string, 0, len(rule.BlockedContentTypes))
		for _, contentType := range rule.BlockedContentTypes {
			contentType = strings.ToLower(strings.TrimSpace(contentType))
			if contentType == "" {
				continue
			}
			types = append(types, contentType)
		}
		if (len(patterns) > 0 || len(types) > 0) && rule.Action != ActionBlock {
			return nil, fmt.Errorf("rule %d: URL and content conditions narrow a block; an allow rule cannot carry them", n)
		}
		if rule.Users, err = normalizeUsers(rule.Users); err != nil {
			return nil, fmt.Errorf("rule %d: %w", n, err)
		}
		if err := rule.Schedule.Validate(); err != nil {
			return nil, fmt.Errorf("rule %d: %w", n, err)
		}
		rule.URLRegexes = nilIfEmpty(patterns)
		rule.BlockedContentTypes = nilIfEmpty(types)
		normalized = append(normalized, rule)
	}
	return normalized, nil
}

func nilIfEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return values
}

// RuleConditionsNeedingProxy lists the conditions in a rule that only the
// proxy can evaluate.
func RuleConditionsNeedingProxy(rule GroupRule) []string {
	conditions := make([]string, 0, 3)
	if len(rule.URLRegexes) > 0 {
		conditions = append(conditions, ConditionURL)
	}
	if len(rule.BlockedContentTypes) > 0 {
		conditions = append(conditions, ConditionContentType)
	}
	if len(rule.Users) > 0 {
		conditions = append(conditions, ConditionUser)
	}
	return conditions
}

// matchTarget reports what part of a rule's target covers a domain: the
// domain pattern, "category:<id>", or "*" for all traffic. An empty result
// means the rule does not apply to the domain.
func matchTarget(target RuleTarget, domain string, index *CategoryIndex) string {
	if target.AllTraffic {
		return "*"
	}
	if pattern := matchDomainList(target.Domains, domain); pattern != "" {
		return pattern
	}
	if id := matchCategoryList(target.Categories, domain, index); id != "" {
		return "category:" + id
	}
	return ""
}

func matchDomainList(patterns []string, domain string) string {
	for _, pattern := range patterns {
		if matchDomain(pattern, domain) {
			return pattern
		}
	}
	return ""
}

func matchCategoryList(ids []string, domain string, index *CategoryIndex) string {
	for _, id := range ids {
		if index.Contains(id, domain) {
			return id
		}
	}
	return ""
}

// evalRequest is everything one policy decision depends on. It is plain data,
// so live enforcement and preview evaluate the same request the same way.
type evalRequest struct {
	domain string
	user   string
	now    time.Time
	layer  DecisionLayer
	// paused lifts every block the policy itself would apply; allows and the
	// gateway-wide categories still apply.
	paused bool
	index  *CategoryIndex
	// gatewayCategories is the gateway-wide category selection, which applies
	// to every policy after the policy's own layers.
	gatewayCategories []string
}

// TraceStep is one line of the explanation of a decision: a stage of the
// evaluation, whether it decided, and why. Preview returns the steps, so the
// explanation an administrator reads is produced by the evaluator itself.
type TraceStep struct {
	Stage   string       `json:"stage"`
	Applied bool         `json:"applied"`
	Action  PolicyAction `json:"action,omitempty"`
	Detail  string       `json:"detail,omitempty"`
}

// groupOutcome is what one policy decides for one request on one layer.
type groupOutcome struct {
	// Action is the decisive outcome: block, allow, or none when nothing in
	// the policy or the gateway categories covers the request.
	Action  PolicyAction
	RuleID  string
	Matched string
	Reason  string
	// URLRegexes and ContentTypes collect the conditional block rules the
	// proxy tests before the decisive outcome applies; DNS never fills them.
	URLRegexes   []string
	ContentTypes []string
	// ConditionRuleID and ConditionReason name the first conditional rule, so
	// a URL or response blocked by a condition is traced to the rule that
	// carried it rather than to the decisive outcome.
	ConditionRuleID string
	ConditionReason string
	// ProxyOnly names the conditions DNS skipped.
	ProxyOnly []string
	Trace     []TraceStep
}

// evaluateGroup walks one policy for one request, first match wins: rules top
// to bottom, then the allowed domains, then the blocked domains and
// categories, then the gateway-wide categories. It is pure: the caller
// supplies the clock, the pause state, and the category data.
//
// A rule the requesting layer cannot fully evaluate never decides on that
// layer. On DNS, a rule scoped to proxy users or narrowed by URL or content
// conditions is skipped, because a domain answer cannot be limited to one
// user or one path. On the proxy, a conditional block rule adds its conditions
// to the outcome and evaluation continues, so a request that does not match
// the condition is decided by whatever comes next.
func evaluateGroup(group PolicyGroup, req evalRequest) groupOutcome {
	out := groupOutcome{Action: ActionNone}
	proxy := req.layer != LayerDNS
	step := func(s TraceStep) { out.Trace = append(out.Trace, s) }

	for i, rule := range group.Rules {
		label := ruleLabel(rule, i)
		if !rule.Enabled {
			continue
		}
		matched := matchTarget(rule.Target, req.domain, req.index)
		if matched == "" {
			continue
		}
		if rule.Schedule != nil && !rule.Schedule.IsActive(req.now) {
			step(TraceStep{Stage: "rule", Detail: label + ": outside its schedule"})
			continue
		}
		if rule.Action == ActionBlock && req.paused {
			step(TraceStep{Stage: "rule", Detail: label + ": paused"})
			continue
		}
		conditions := RuleConditionsNeedingProxy(rule)
		if !proxy && len(conditions) > 0 {
			out.ProxyOnly = appendUnique(out.ProxyOnly, conditions...)
			step(TraceStep{Stage: "rule", Detail: label + ": decided by the proxy (" + strings.Join(conditions, ", ") + ")"})
			continue
		}
		if proxy && len(rule.Users) > 0 && !containsString(rule.Users, req.user) {
			continue
		}
		if proxy && (len(rule.URLRegexes) > 0 || len(rule.BlockedContentTypes) > 0) {
			out.URLRegexes = append(out.URLRegexes, rule.URLRegexes...)
			out.ContentTypes = append(out.ContentTypes, rule.BlockedContentTypes...)
			if out.ConditionRuleID == "" {
				out.ConditionRuleID = rule.ID
				out.ConditionReason = "rule " + label
			}
			step(TraceStep{Stage: "rule", Applied: true, Action: ActionBlock, Detail: label + ": blocks matching URLs and response types"})
			continue
		}
		out.Action = rule.Action
		out.RuleID = rule.ID
		out.Matched = matched
		out.Reason = "rule " + label
		step(TraceStep{Stage: "rule", Applied: true, Action: rule.Action, Detail: label + " matched " + matched})
		return out
	}

	if pattern := matchDomainList(group.AllowedDomains, req.domain); pattern != "" {
		out.decide(ActionAllow, pattern, "allowed domain "+pattern)
		step(TraceStep{Stage: "allowed_domains", Applied: true, Action: ActionAllow, Detail: pattern})
		return out
	}
	if pattern := matchDomainList(group.BlockedDomains, req.domain); pattern != "" {
		if req.paused {
			step(TraceStep{Stage: "blocked_domains", Detail: pattern + ": paused"})
		} else {
			out.decide(ActionBlock, pattern, "blocked domain "+pattern)
			step(TraceStep{Stage: "blocked_domains", Applied: true, Action: ActionBlock, Detail: pattern})
			return out
		}
	}
	if id := matchCategoryList(group.BlockedCategories, req.domain, req.index); id != "" {
		if req.paused {
			step(TraceStep{Stage: "blocked_categories", Detail: id + ": paused"})
		} else {
			out.decide(ActionBlock, "category:"+id, "blocked category "+id)
			step(TraceStep{Stage: "blocked_categories", Applied: true, Action: ActionBlock, Detail: id})
			return out
		}
	}
	if id := matchCategoryList(req.gatewayCategories, req.domain, req.index); id != "" {
		out.decide(ActionBlock, "category:"+id, "gateway category "+id)
		step(TraceStep{Stage: "gateway_categories", Applied: true, Action: ActionBlock, Detail: id})
		return out
	}
	step(TraceStep{Stage: "policy", Detail: "nothing in the policy covers this domain"})
	return out
}

// decide records a list match as the decisive outcome.
func (o *groupOutcome) decide(action PolicyAction, matched, reason string) {
	o.Action = action
	o.Matched = matched
	o.Reason = reason
}

func ruleLabel(rule GroupRule, index int) string {
	if rule.Name != "" {
		return fmt.Sprintf("%q", rule.Name)
	}
	return fmt.Sprintf("#%d", index+1)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUnique(values []string, more ...string) []string {
	for _, value := range more {
		if !containsString(values, value) {
			values = append(values, value)
		}
	}
	return values
}
