package gatesentryf

import (
	"encoding/json"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	gatesentryDomainList "bitbucket.org/abdullah_irfan/gatesentryf/domainlist"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
)

// RuleManager handles rule storage and matching
type RuleManager struct {
	storage       *gatesentry2storage.MapStore
	domainListMgr *gatesentryDomainList.DomainListManager
}

// NewRuleManager creates a new rule manager
func NewRuleManager(storage *gatesentry2storage.MapStore) *RuleManager {
	return &RuleManager{storage: storage}
}

// SetDomainListManager sets the shared DomainListManager for domain list lookups.
// This is called after both the RuleManager and DomainListManager are created.
func (rm *RuleManager) SetDomainListManager(dlm *gatesentryDomainList.DomainListManager) {
	rm.domainListMgr = dlm
}

// GetRules retrieves all rules from storage
func (rm *RuleManager) GetRules() ([]GatesentryTypes.Rule, error) {
	rulesJSON := rm.storage.Get("rules")
	if rulesJSON == "" {
		return []GatesentryTypes.Rule{}, nil
	}

	// Try to unmarshal as RuleList first (new format)
	var ruleList GatesentryTypes.RuleList
	err := json.Unmarshal([]byte(rulesJSON), &ruleList)
	if err == nil {
		// Sort by priority (lower number = higher priority)
		sort.Slice(ruleList.Rules, func(i, j int) bool {
			return ruleList.Rules[i].Priority < ruleList.Rules[j].Priority
		})
		return ruleList.Rules, nil
	}

	// If that fails, try to unmarshal as array directly (old format)
	var rules []GatesentryTypes.Rule
	err = json.Unmarshal([]byte(rulesJSON), &rules)
	if err != nil {
		log.Printf("Error unmarshaling rules: %v", err)
		return []GatesentryTypes.Rule{}, err
	}

	// Sort by priority (lower number = higher priority)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority < rules[j].Priority
	})

	return rules, nil
}

// SaveRules saves rules to storage
func (rm *RuleManager) SaveRules(rules []GatesentryTypes.Rule) error {
	ruleList := GatesentryTypes.RuleList{Rules: rules}
	rulesJSON, err := json.Marshal(ruleList)
	if err != nil {
		log.Printf("Error marshaling rules: %v", err)
		return err
	}

	rm.storage.Update("rules", string(rulesJSON))
	return nil
}

// AddRule adds a new rule and returns it with generated ID
func (rm *RuleManager) AddRule(rule GatesentryTypes.Rule) (GatesentryTypes.Rule, error) {
	rules, err := rm.GetRules()
	if err != nil {
		return rule, err
	}

	// Set metadata
	now := time.Now().Format(time.RFC3339)
	rule.CreatedAt = now
	rule.UpdatedAt = now

	// Generate ID if not provided
	if rule.ID == "" {
		rule.ID = generateRuleID()
	}

	rules = append(rules, rule)
	err = rm.SaveRules(rules)
	return rule, err
}

// UpdateRule updates an existing rule
func (rm *RuleManager) UpdateRule(ruleID string, updatedRule GatesentryTypes.Rule) error {
	rules, err := rm.GetRules()
	if err != nil {
		return err
	}

	for i, rule := range rules {
		if rule.ID == ruleID {
			// Preserve creation time
			updatedRule.CreatedAt = rule.CreatedAt
			updatedRule.UpdatedAt = time.Now().Format(time.RFC3339)
			updatedRule.ID = ruleID
			rules[i] = updatedRule
			return rm.SaveRules(rules)
		}
	}

	return nil
}

// DeleteRule removes a rule by ID
func (rm *RuleManager) DeleteRule(ruleID string) error {
	rules, err := rm.GetRules()
	if err != nil {
		return err
	}

	filteredRules := []GatesentryTypes.Rule{}
	for _, rule := range rules {
		if rule.ID != ruleID {
			filteredRules = append(filteredRules, rule)
		}
	}

	return rm.SaveRules(filteredRules)
}

// GetRule retrieves a single rule by ID
func (rm *RuleManager) GetRule(ruleID string) (*GatesentryTypes.Rule, error) {
	rules, err := rm.GetRules()
	if err != nil {
		return nil, err
	}

	for _, rule := range rules {
		if rule.ID == ruleID {
			return &rule, nil
		}
	}

	return nil, nil
}

// MatchDomain checks if a domain matches a rule's domain pattern
func matchDomain(pattern, domain string) bool {
	pattern = strings.ToLower(pattern)
	domain = strings.ToLower(domain)

	// Universal wildcard — matches every domain
	if pattern == "*" {
		return true
	}

	if pattern == domain {
		return true
	}

	// Wildcard matching: * matches zero or more characters (including dots)
	// Supports patterns like *.example.com, ad*, *tracker*, etc.
	if strings.Contains(pattern, "*") {
		return globMatch(pattern, domain)
	}

	return false
}

// globMatch performs simple glob-style matching where * matches any sequence
// of characters (including dots/subdomains). Supports multiple * in pattern.
func globMatch(pattern, str string) bool {
	// Fast path for common *.suffix pattern
	if strings.HasPrefix(pattern, "*.") && !strings.Contains(pattern[2:], "*") {
		suffix := pattern[2:]
		return strings.HasSuffix(str, "."+suffix) || str == suffix
	}

	// General glob: split on * and check that parts appear in order
	parts := strings.Split(pattern, "*")

	// First part must be a prefix
	if !strings.HasPrefix(str, parts[0]) {
		return false
	}
	str = str[len(parts[0]):]

	// Middle parts must appear in order
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(str, parts[i])
		if idx < 0 {
			return false
		}
		str = str[idx+len(parts[i]):]
	}

	// Last part must be a suffix
	return strings.HasSuffix(str, parts[len(parts)-1])
}

// matchRuleDomain checks whether a domain matches any of a rule's domain
// criteria. A rule matches if ANY of the following are true:
//  1. rule.Domain is set and matchDomain(rule.Domain, domain) (legacy, backward compat)
//  2. Any pattern in rule.DomainPatterns matches (wildcard matching)
//  3. The domain appears in ANY domain list referenced by rule.DomainLists (O(1) index lookup)
func (rm *RuleManager) matchRuleDomain(rule *GatesentryTypes.Rule, domain string) bool {
	// 1. Legacy single-domain pattern
	if rule.Domain != "" && matchDomain(rule.Domain, domain) {
		return true
	}

	// 2. Multiple domain patterns (wildcards)
	for _, pattern := range rule.DomainPatterns {
		if matchDomain(pattern, domain) {
			return true
		}
	}

	// 3. Domain list membership (O(1) lookup via shared index)
	if len(rule.DomainLists) > 0 && rm.domainListMgr != nil && rm.domainListMgr.Index != nil {
		if rm.domainListMgr.Index.IsDomainInAnyList(strings.ToLower(domain), rule.DomainLists) {
			return true
		}
	}

	return false
}

// CheckTimeRestriction checks if current time is within the restriction
func checkTimeRestriction(restriction *GatesentryTypes.TimeRestriction) bool {
	if restriction == nil {
		return true
	}

	now := time.Now()
	currentTime := now.Format("15:04")

	if restriction.From <= restriction.To {
		return currentTime >= restriction.From && currentTime <= restriction.To
	} else {
		return currentTime >= restriction.From || currentTime <= restriction.To
	}
}

// MatchRule finds the first matching rule for a given domain and user
func (rm *RuleManager) MatchRule(domain, user string) GatesentryTypes.RuleMatch {
	rules, err := rm.GetRules()
	if err != nil {
		log.Printf("Error getting rules: %v", err)
		return GatesentryTypes.RuleMatch{Matched: false}
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		if !rm.matchRuleDomain(&rule, domain) {
			continue
		}

		if len(rule.Users) > 0 {
			userMatched := false
			for _, ruleUser := range rule.Users {
				if ruleUser == user {
					userMatched = true
					break
				}
			}
			if !userMatched {
				continue
			}
		}

		if !checkTimeRestriction(rule.TimeRestriction) {
			continue
		}

		match := GatesentryTypes.RuleMatch{
			Matched: true,
			Rule:    &rule,
		}

		// Resolve MITM state:
		//   "enable"  → true
		//   "disable" → false
		//   "default" → use global enable_https_filtering setting
		switch rule.MITMAction {
		case GatesentryTypes.MITMActionEnable:
			match.ShouldMITM = true
		case GatesentryTypes.MITMActionDisable:
			match.ShouldMITM = false
		default: // "default" or empty
			match.ShouldMITM = rm.storage.Get("enable_https_filtering") == "true"
		}
		match.ShouldBlock = rule.Action == GatesentryTypes.RuleActionBlock

		// Populate match criteria — these are always set regardless of MITM.
		// Content types and URL regexes are request-level match criteria.
		// Keyword filtering is content scanning (works on HTTP and HTTPS+MITM).
		if len(rule.BlockedContentTypes) > 0 {
			match.BlockContentTypes = rule.BlockedContentTypes
		}
		if len(rule.URLRegexPatterns) > 0 {
			match.BlockURLRegexes = rule.URLRegexPatterns
		}
		match.KeywordFilterEnabled = rule.KeywordFilterEnabled

		return match
	}

	return GatesentryTypes.RuleMatch{Matched: false}
}

// CheckContentTypeBlocked checks if a content type should be blocked based on rule
func CheckContentTypeBlocked(contentType string, blockedTypes []string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))

	for _, blocked := range blockedTypes {
		blocked = strings.ToLower(strings.TrimSpace(blocked))
		if strings.Contains(contentType, blocked) {
			return true
		}
	}

	return false
}

// CheckURLPathBlocked checks if a URL path matches any blocked regex pattern
func CheckURLPathBlocked(urlPath string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, urlPath)
		if err != nil {
			log.Printf("Error matching regex pattern %s: %v", pattern, err)
			continue
		}
		if matched {
			return true
		}
	}

	return false
}

func generateRuleID() string {
	return gatesentryUtils.RandomString(16)
}
