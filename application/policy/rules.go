package policy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A group rule is decided by the layer that can see its conditions, so the
// evaluator is told which layer is asking: DNS sees a domain and the client's
// identity, the proxy also sees a URL path, the response media type, and (when
// it inspects TLS) the request inside an encrypted connection. Layers are the
// existing DecisionLayer vocabulary, so a decision and the layer that produced
// it cannot drift apart.
// NormalizeGroupRules validates rules before they are persisted and returns
// the canonical form: trimmed values, lowercased media types, an ID on every
// rule, and disabled rules kept as written. A malformed rule is a rejection,
// never a rule that silently matches nothing.
func NormalizeGroupRules(rules []GroupRule) ([]GroupRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	normalized := make([]GroupRule, 0, len(rules))
	for i, rule := range rules {
		rule.Name = strings.TrimSpace(rule.Name)
		rule.MITMAction = strings.TrimSpace(rule.MITMAction)
		if rule.ID == "" {
			rule.ID = newGroupID()
		}
		switch rule.Action {
		case ActionAllow, ActionBlock:
		default:
			return nil, fmt.Errorf("rule %d needs an allow or block action", i+1)
		}
		switch rule.MITMAction {
		case "", MITMActionDefault, MITMActionEnable, MITMActionDisable:
		default:
			return nil, fmt.Errorf("rule %d has an invalid TLS inspection setting", i+1)
		}
		patterns := make([]string, 0, len(rule.URLRegexes))
		for _, pattern := range rule.URLRegexes {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return nil, fmt.Errorf("rule %d URL pattern %q is not a valid regular expression", i+1, pattern)
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
		users := make([]string, 0, len(rule.Users))
		for _, user := range rule.Users {
			user = strings.TrimSpace(user)
			if user == "" {
				continue
			}
			users = append(users, user)
		}
		if err := rule.Schedule.Validate(); err != nil {
			return nil, fmt.Errorf("rule %d: %w", i+1, err)
		}
		proxyConditions := len(patterns) > 0 || len(types) > 0
		if proxyConditions && rule.Action != ActionBlock {
			return nil, fmt.Errorf("rule %d: URL and content conditions can only narrow a block; an allow rule cannot carry them", i+1)
		}
		if proxyConditions && rule.MITMAction == MITMActionDisable {
			return nil, fmt.Errorf("rule %d: URL and content conditions need TLS inspection, so the rule cannot disable it", i+1)
		}
		rule.URLRegexes = patterns
		rule.BlockedContentTypes = types
		rule.Users = users
		normalized = append(normalized, rule)
	}
	return normalized, nil
}

// EnabledGroupRules returns the group's enabled rules in evaluation order:
// lower priority first, then ID so the order never depends on map iteration or
// on the order the API happened to receive.
func EnabledGroupRules(group PolicyGroup) []GroupRule {
	rules := make([]GroupRule, 0, len(group.Rules))
	for _, rule := range group.Rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
	return rules
}

// RuleConditionsNeedingProxy lists the conditions in a rule that only the
// proxy can evaluate. DNS reports them as unresolved instead of pretending a
// domain decision enforced them.
func RuleConditionsNeedingProxy(rule GroupRule) []string {
	conditions := make([]string, 0, 2)
	if len(rule.URLRegexes) > 0 {
		conditions = append(conditions, "url_regex")
	}
	if len(rule.BlockedContentTypes) > 0 {
		conditions = append(conditions, "content_type")
	}
	return conditions
}

// ruleOutcome is the decision the rules of one group produce for one request.
type ruleOutcome struct {
	Action        PolicyAction
	RuleID        string
	MatchedDomain string
	Reason        string
	ShouldMITM    bool
	BlockURLRegexes   []string
	BlockContentTypes []string
	Unresolved        []string
}

// evaluateGroupRulesAt decides one group's outcome for a domain on a given
// enforcement layer. It is pure: the caller supplies the clock, so live
// enforcement and preview cannot disagree about a time window.
//
// The group action is the answer for the domains and categories the group
// covers. A rule narrows that answer: it applies only to the users and time
// windows it names, an allow rule turns the covered traffic into an exception,
// and a block rule can carry URL and content conditions that the proxy tests
// after the response arrives. On DNS, a rule's proxy-only conditions are
// reported as unresolved while the domain decision stands, because DNS cannot
// narrow a domain block to a URL path and must not widen an allow to a whole
// host.
func evaluateGroupRulesAt(group PolicyGroup, matched, categoryID string, user string, now time.Time, layer DecisionLayer) ruleOutcome {
	outcome := ruleOutcome{
		Action:        group.Action,
		MatchedDomain: matched,
		Reason:        "group policy",
	}
	if categoryID != "" {
		outcome.Reason = "group policy category " + categoryID
	}
	for _, rule := range EnabledGroupRules(group) {
		if len(rule.Users) > 0 && !ruleAppliesToUser(rule, user) {
			continue
		}
		if rule.Schedule != nil && !rule.Schedule.IsActive(now) {
			continue
		}
		outcome.RuleID = rule.ID
		outcome.Reason = ruleReason(rule, categoryID)
		outcome.Action = rule.Action
		if layer == LayerExplicitProxy {
			if rule.Action == ActionAllow {
				// An exception inside the group: the proxy forwards the
				// request and the gateway's own settings stay in charge.
				outcome.Action = ActionAllow
				return outcome
			}
			outcome.BlockURLRegexes = append([]string(nil), rule.URLRegexes...)
			outcome.BlockContentTypes = append([]string(nil), rule.BlockedContentTypes...)
			outcome.ShouldMITM = rule.MITMAction == MITMActionEnable || len(rule.URLRegexes) > 0 || len(rule.BlockedContentTypes) > 0
			return outcome
		}
		outcome.Unresolved = RuleConditionsNeedingProxy(rule)
		if rule.MITMAction == MITMActionEnable {
			outcome.ShouldMITM = true
		}
		return outcome
	}
	return outcome
}

// ruleAppliesToUser reports whether a rule's user scope covers the request.
// A rule with no user scope applies to every user the group applies to.
func ruleAppliesToUser(rule GroupRule, user string) bool {
	for _, candidate := range rule.Users {
		if candidate == user {
			return true
		}
	}
	return false
}

// ruleReason names the rule that decided a request, so a decision can be
// traced back to the record an administrator wrote.
func ruleReason(rule GroupRule, categoryID string) string {
	name := rule.Name
	if name == "" {
		name = rule.ID
	}
	if categoryID != "" {
		return "group rule " + name + " on category " + categoryID
	}
	return "group rule " + name
}
