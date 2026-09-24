package policy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// documentV1 is the version 1 policy document. A version 1 group applied one
// action to a shared set of domains and categories, and its rules could only
// refine that action for the same coverage. The shape is kept only to read an
// existing document once and carry it into version 2.
type documentV1 struct {
	Groups       []groupV1          `json:"groups"`
	Assignments  []DeviceAssignment `json:"assignments"`
	MigratedFrom string             `json:"migrated_from,omitempty"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type groupV1 struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Domains     []string     `json:"domains,omitempty"`
	Categories  []string     `json:"categories,omitempty"`
	Rules       []ruleV1     `json:"rules,omitempty"`
	Action      PolicyAction `json:"action"`
	Users       []string     `json:"users,omitempty"`
	Priority    int          `json:"priority"`
	Schedule    *Schedule    `json:"schedule,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type ruleV1 struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name,omitempty"`
	Enabled             bool         `json:"enabled"`
	Action              PolicyAction `json:"action"`
	MITMAction          string       `json:"mitm_action,omitempty"`
	URLRegexes          []string     `json:"url_regexes,omitempty"`
	BlockedContentTypes []string     `json:"blocked_content_types,omitempty"`
	Schedule            *Schedule    `json:"schedule,omitempty"`
	Users               []string     `json:"users,omitempty"`
	Priority            int          `json:"priority"`
}

// upgradeDocumentV1 converts a version 1 document into version 2 while keeping
// what it enforced for every device that was assigned to a group.
func upgradeDocumentV1(raw string) (PolicyDocument, error) {
	var old documentV1
	if err := json.Unmarshal([]byte(raw), &old); err != nil {
		return PolicyDocument{}, fmt.Errorf("parse version 1 policy groups: %w", err)
	}
	doc := PolicyDocument{
		Version:      DocumentVersion,
		Groups:       make([]PolicyGroup, 0, len(old.Groups)),
		Assignments:  old.Assignments,
		MigratedFrom: old.MigratedFrom,
		UpdatedAt:    old.UpdatedAt,
	}
	for _, group := range old.Groups {
		doc.Groups = append(doc.Groups, upgradeGroupV1(group))
	}
	return doc, nil
}

// upgradeGroupV1 carries one version 1 group into version 2.
//
// A version 1 rule applied to the group's whole coverage, so it becomes a
// version 2 rule targeting exactly that coverage, in the same evaluation order.
// The group action becomes the blocked or allowed lists. A group schedule made
// the whole group inactive outside its windows, so a scheduled group's lists
// become one rule carrying that schedule instead of always-on lists.
//
// Version 1 read "*.example.com" as the domain and its subdomains, which is
// what a plain "example.com" means now, so wildcards are carried as plain
// domains. The per-rule TLS inspection switch is gone: conditions that need
// inspection turn it on by themselves, and a rule's own "inspect" or "never
// inspect" setting is dropped with a note in the description.
func upgradeGroupV1(old groupV1) PolicyGroup {
	group := PolicyGroup{
		ID:          old.ID,
		Name:        old.Name,
		Description: old.Description,
		Users:       old.Users,
		Priority:    old.Priority,
		CreatedAt:   old.CreatedAt,
		UpdatedAt:   old.UpdatedAt,
	}
	if group.Name == "" {
		group.Name = old.ID
	}
	domains := make([]string, 0, len(old.Domains))
	for _, domain := range old.Domains {
		domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "*.")
		if domain != "" && !containsString(domains, domain) {
			domains = append(domains, domain)
		}
	}
	categories := append([]string(nil), old.Categories...)
	coverage := RuleTarget{Domains: domains, Categories: categories}
	hasCoverage := len(domains) > 0 || len(categories) > 0

	rules := append([]ruleV1(nil), old.Rules...)
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
	droppedInspection := false
	for _, rule := range rules {
		if rule.MITMAction == MITMActionEnableV1 || rule.MITMAction == MITMActionDisableV1 {
			droppedInspection = true
		}
		if !hasCoverage {
			continue
		}
		schedule := rule.Schedule
		if schedule == nil {
			schedule = old.Schedule
		}
		group.Rules = append(group.Rules, GroupRule{
			ID:                  rule.ID,
			Name:                rule.Name,
			Enabled:             rule.Enabled,
			Action:              rule.Action,
			Target:              RuleTarget{Domains: append([]string(nil), domains...), Categories: append([]string(nil), categories...)},
			URLRegexes:          rule.URLRegexes,
			BlockedContentTypes: rule.BlockedContentTypes,
			Schedule:            schedule,
			Users:               rule.Users,
		})
	}

	if hasCoverage && (old.Action == ActionBlock || old.Action == ActionAllow) {
		if old.Schedule != nil {
			group.Rules = append(group.Rules, GroupRule{
				ID:       newGroupID(),
				Name:     "Scheduled " + string(old.Action),
				Enabled:  true,
				Action:   old.Action,
				Target:   coverage,
				Schedule: old.Schedule,
			})
		} else if old.Action == ActionBlock {
			group.BlockedDomains = domains
			group.BlockedCategories = categories
		} else {
			group.AllowedDomains = domains
			if len(categories) > 0 {
				group.Rules = append(group.Rules, GroupRule{
					ID:      newGroupID(),
					Name:    "Allowed categories",
					Enabled: true,
					Action:  ActionAllow,
					Target:  RuleTarget{Categories: categories},
				})
			}
		}
	}
	if droppedInspection {
		note := "Upgraded: a rule's own TLS inspection setting was removed; URL and response-type conditions turn inspection on by themselves."
		group.Description = strings.TrimSpace(group.Description + " " + note)
	}
	return group
}

// The version 1 per-rule inspection settings, kept only so an upgrade can tell
// the administrator which rules carried one.
const (
	MITMActionEnableV1  = "enable"
	MITMActionDisableV1 = "disable"
)
