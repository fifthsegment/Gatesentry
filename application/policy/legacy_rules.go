package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// LegacyRule is one record from the retired standalone rule store. Rules are
// not a second policy engine any more: they live inside a policy group, so a
// rule and the group that owns it are decided by one evaluator. This shape
// exists only to read the old records once and carry them into groups.
type LegacyRule struct {
	ID                  string                 `json:"id"`
	Name                string                 `json:"name"`
	Enabled             bool                   `json:"enabled"`
	Priority            int                    `json:"priority"`
	Domain              string                 `json:"domain"`
	Action              string                 `json:"action"`
	MITMAction          string                 `json:"mitm_action"`
	BlockType           string                 `json:"block_type"`
	BlockedContentTypes []string               `json:"blocked_content_types"`
	URLRegexPatterns    []string               `json:"url_regex_patterns"`
	TimeRestriction     *LegacyTimeRestriction `json:"time_restriction"`
	Users               []string               `json:"users"`
	Description         string                 `json:"description"`
}

// LegacyTimeRestriction is the old single-window time restriction: a from/to
// pair of "HH:MM" wall-clock times in the gateway's own time zone.
type LegacyTimeRestriction struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// DecodeLegacyRules reads the retired rule store value. The store held a bare
// JSON array before it held a RuleList object, so both shapes are accepted; an
// empty value means no legacy rules.
func DecodeLegacyRules(raw string) ([]LegacyRule, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var list struct {
		Rules []LegacyRule `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err == nil && list.Rules != nil {
		return list.Rules, nil
	}
	var rules []LegacyRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("decode legacy rules: %w", err)
	}
	return rules, nil
}

// MigrateLegacyRules carries the retired rule records into policy groups. Each
// legacy rule becomes one policy holding one rule that targets the domain it
// named. The policies are created unassigned, so importing them changes no
// device's filtering until an administrator assigns one.
//
// The conversion keeps what the old engine enforced: the action, the
// authenticated-user scope, the time window (in the gateway's time zone), and
// the URL and media-type conditions that only applied after inspection. A
// disabled rule imports disabled, so a rule that enforced nothing cannot start
// enforcing by being imported.
func MigrateLegacyRules(rules []LegacyRule, timezone string) ([]PolicyGroup, error) {
	groups := make([]PolicyGroup, 0, len(rules))
	for i, rule := range rules {
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(rule.Domain)), "*.")
		if domain == "" {
			return nil, fmt.Errorf("legacy rule %d has no domain", i+1)
		}
		action := PolicyAction(strings.TrimSpace(rule.Action))
		switch action {
		case ActionAllow, ActionBlock:
		default:
			return nil, fmt.Errorf("legacy rule %d has action %q", i+1, rule.Action)
		}
		conditions := legacyRuleConditions(rule)
		groupID := "legacy-rule-" + strings.TrimSpace(rule.ID)
		if strings.TrimSpace(rule.ID) == "" {
			groupID = "legacy-rule-" + newGroupID()
		}
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			name = domain
		}
		description := "Imported from the retired rule store. Assign devices to it before it changes any device's filtering."
		schedule, dropped := legacySchedule(rule.TimeRestriction, timezone)
		if dropped != "" {
			description += " " + dropped
		}
		if strings.TrimSpace(rule.Description) != "" {
			description += " " + strings.TrimSpace(rule.Description)
		}
		group := PolicyGroup{
			ID:          groupID,
			Name:        name,
			Description: description,
			Rules: []GroupRule{{
				ID:                  rule.ID,
				Name:                name,
				Enabled:             rule.Enabled,
				Action:              action,
				Target:              RuleTarget{Domains: []string{domain}},
				URLRegexes:          conditions.urls,
				BlockedContentTypes: conditions.contentTypes,
				Schedule:            schedule,
				Users:               append([]string(nil), rule.Users...),
			}},
		}
		normalized, err := NormalizeGroup(group)
		if err != nil {
			return nil, fmt.Errorf("legacy rule %d: %w", i+1, err)
		}
		groups = append(groups, normalized)
	}
	return groups, nil
}

type legacyConditions struct {
	urls         []string
	contentTypes []string
}

// legacyRuleConditions returns the URL and media-type conditions the old
// engine actually applied. It only applied them when the rule inspected TLS,
// and only to a block action: an allow rule kept its URL patterns but the
// proxy never tested them, so importing them would invent enforcement.
func legacyRuleConditions(rule LegacyRule) legacyConditions {
	if strings.TrimSpace(rule.MITMAction) != MITMActionEnableV1 {
		return legacyConditions{}
	}
	if PolicyAction(strings.TrimSpace(rule.Action)) != ActionBlock {
		return legacyConditions{}
	}
	conditions := legacyConditions{}
	switch strings.TrimSpace(rule.BlockType) {
	case "content_type":
		conditions.contentTypes = rule.BlockedContentTypes
	case "url_regex":
		conditions.urls = rule.URLRegexPatterns
	case "both":
		conditions.urls = rule.URLRegexPatterns
		conditions.contentTypes = rule.BlockedContentTypes
	}
	// A pattern the old engine could not compile matched nothing, so it is
	// dropped here instead of failing the import.
	urls := make([]string, 0, len(conditions.urls))
	for _, pattern := range conditions.urls {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			continue
		}
		urls = append(urls, pattern)
	}
	conditions.urls = urls
	return conditions
}

// legacySchedule converts the old single window into a group rule schedule.
// The old window was evaluated in the gateway's own time zone and every day of
// the week, so the schedule keeps that zone and leaves the weekday set empty.
// It returns a sentence to add to the group description when the window cannot
// be represented, so the difference is visible instead of silent.
func legacySchedule(restriction *LegacyTimeRestriction, timezone string) (*Schedule, string) {
	if restriction == nil {
		return nil, ""
	}
	schedule := &Schedule{
		Timezone: strings.TrimSpace(timezone),
		Windows:  []TimeWindow{{From: strings.TrimSpace(restriction.From), To: strings.TrimSpace(restriction.To)}},
		Preset:   "legacy-time-restriction",
	}
	if err := schedule.Validate(); err != nil {
		return nil, "The old time window " + restriction.From + "-" + restriction.To + " could not be represented and was dropped; edit this rule to set one."
	}
	return schedule, ""
}
