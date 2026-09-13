package gatesentryf

import (
	"encoding/json"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
)

// RuleManager handles rule storage and matching
type RuleManager struct {
	storage *gatesentry2storage.MapStore
}

// NewRuleManager creates a new rule manager
func NewRuleManager(storage *gatesentry2storage.MapStore) *RuleManager {
	return &RuleManager{storage: storage}
}

// GetRules retrieves all rules from storage
func (rm *RuleManager) GetRules() ([]GatesentryTypes.Rule, error) {
	rulesJSON, err := rm.storage.GetE("rules")
	if err != nil {
		return nil, err
	}
	if rulesJSON == "" {
		return []GatesentryTypes.Rule{}, nil
	}

	// Try to unmarshal as RuleList first (new format)
	var ruleList GatesentryTypes.RuleList
	err = json.Unmarshal([]byte(rulesJSON), &ruleList)
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

	return rm.storage.Update("rules", string(rulesJSON))
}

func decodeRules(rulesJSON string) ([]GatesentryTypes.Rule, error) {
	if rulesJSON == "" {
		return []GatesentryTypes.Rule{}, nil
	}
	var ruleList GatesentryTypes.RuleList
	if err := json.Unmarshal([]byte(rulesJSON), &ruleList); err == nil {
		return ruleList.Rules, nil
	}
	var rules []GatesentryTypes.Rule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func encodeRules(rules []GatesentryTypes.Rule) (string, error) {
	b, err := json.Marshal(GatesentryTypes.RuleList{Rules: rules})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// AddRule adds a new rule and returns it with generated ID
func (rm *RuleManager) AddRule(rule GatesentryTypes.Rule) (GatesentryTypes.Rule, error) {
	// Set metadata
	now := time.Now().Format(time.RFC3339)
	rule.CreatedAt = now
	rule.UpdatedAt = now

	// Generate ID if not provided
	if rule.ID == "" {
		rule.ID = generateRuleID()
	}

	err := rm.storage.UpdateValue("rules", func(current string) (string, error) {
		rules, err := decodeRules(current)
		if err != nil {
			return "", err
		}
		return encodeRules(append(rules, rule))
	})
	return rule, err
}

// UpdateRule updates an existing rule
func (rm *RuleManager) UpdateRule(ruleID string, updatedRule GatesentryTypes.Rule) error {
	return rm.storage.UpdateValue("rules", func(current string) (string, error) {
		rules, err := decodeRules(current)
		if err != nil {
			return "", err
		}
		for i, rule := range rules {
			if rule.ID == ruleID {
				updatedRule.CreatedAt = rule.CreatedAt
				updatedRule.UpdatedAt = time.Now().Format(time.RFC3339)
				updatedRule.ID = ruleID
				rules[i] = updatedRule
				break
			}
		}
		return encodeRules(rules)
	})
}

// DeleteRule removes a rule by ID
func (rm *RuleManager) DeleteRule(ruleID string) error {
	return rm.storage.UpdateValue("rules", func(current string) (string, error) {
		rules, err := decodeRules(current)
		if err != nil {
			return "", err
		}
		filteredRules := make([]GatesentryTypes.Rule, 0, len(rules))
		for _, rule := range rules {
			if rule.ID != ruleID {
				filteredRules = append(filteredRules, rule)
			}
		}
		return encodeRules(filteredRules)
	})
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

	if pattern == domain {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:]
		return strings.HasSuffix(domain, "."+suffix) || domain == suffix
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

		if !matchDomain(rule.Domain, domain) {
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

		match.ShouldMITM = rule.MITMAction == GatesentryTypes.MITMActionEnable
		match.ShouldBlock = rule.Action == GatesentryTypes.RuleActionBlock

		if match.ShouldMITM {
			if rule.BlockType == GatesentryTypes.BlockTypeContentType ||
				rule.BlockType == GatesentryTypes.BlockTypeBoth {
				match.BlockContentTypes = rule.BlockedContentTypes
			}
			if rule.BlockType == GatesentryTypes.BlockTypeURLRegex ||
				rule.BlockType == GatesentryTypes.BlockTypeBoth {
				match.BlockURLRegexes = rule.URLRegexPatterns
			}
		}

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
