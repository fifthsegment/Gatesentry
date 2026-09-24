package policy

import "time"

// PolicyTemplate describes a safe starting point for an ordinary policy
// group. Templates are catalog data, not a second policy store: applying one
// creates a normal PolicyGroup that can be edited or deleted independently.
//
// A template states the categories it blocks and the one caveat that belongs
// to it. Caveats that hold for every starter live in
// TemplateSharedLimitations, so no sentence is repeated across the catalog.
// Categories are upstream feeds, so coverage keeps updating without a
// Gatesentry release, but a category is never age verification, device
// classification, or a review of what a household actually uses.
type PolicyTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	GroupName   string   `json:"group_name"`
	Description string   `json:"description"`
	Limitations []string `json:"limitations"`
	// Categories are the categories the starter blocks, listed so the catalog
	// can show them before the policy is created.
	Categories []string    `json:"categories"`
	SafeSearch bool        `json:"safe_search"`
	Rules      []GroupRule `json:"rules,omitempty"`
	GroupID    string      `json:"group_id"`
	Available  bool        `json:"available"`
}

// Every starter is enforced for the devices assigned to it. These hold for the
// whole catalog, so they are stated once instead of inside every template.
var templateSharedLimitations = []string{
	"A starter applies only to the devices and proxy users you assign to it; everyone else stays on the default policy.",
	"DNS enforces domains and categories for devices that use GateSentry for DNS. URL and response-type rules need the proxy.",
}

// TemplateSharedLimitations returns a copy of the caveats that apply to every
// starter, so a caller can state them once above the catalog.
func TemplateSharedLimitations() []string {
	return append([]string(nil), templateSharedLimitations...)
}

// bedtimeRule blocks all traffic overnight on school nights. The time zone is
// filled in when the starter is applied, so the window follows the gateway's
// own clock.
func bedtimeRule() GroupRule {
	preset := PresetByID("bedtime")
	return GroupRule{
		ID:      "bedtime",
		Name:    "Bedtime",
		Enabled: true,
		Action:  ActionBlock,
		Target:  RuleTarget{AllTraffic: true},
		Schedule: &Schedule{
			Weekdays: append([]time.Weekday(nil), preset.Weekdays...),
			Windows:  append([]TimeWindow(nil), preset.Windows...),
			Preset:   preset.ID,
		},
	}
}

var builtInTemplates = []PolicyTemplate{
	{
		ID: "child", Name: "Young child", GroupName: "Young child",
		Description: "Blocks adult, gambling, social media, piracy, and malware; forces safe search; no internet at bedtime on school nights.",
		Limitations: []string{
			"Categories are shared upstream feeds, not age verification.",
		},
		Categories: []string{"adult", "gambling", "social", "piracy", "malware"},
		SafeSearch: true,
		Rules:      []GroupRule{bedtimeRule()},
		GroupID:    "template-child", Available: true,
	},
	{
		ID: "teen", Name: "Teen", GroupName: "Teen",
		Description: "Blocks adult, gambling, and malware; forces safe search; no internet at bedtime on school nights.",
		Limitations: []string{
			"Social media stays reachable; add the Social media category to block it.",
		},
		Categories: []string{"adult", "gambling", "malware"},
		SafeSearch: true,
		Rules:      []GroupRule{bedtimeRule()},
		GroupID:    "template-teen", Available: true,
	},
	{
		ID: "guest", Name: "Guest", GroupName: "Guest",
		Description: "Blocks adult content, malware, and piracy for visitor devices.",
		Limitations: []string{
			"Does not isolate guests or create a separate network; use network controls for that.",
		},
		Categories: []string{"adult", "malware", "piracy"},
		GroupID:    "template-guest", Available: true,
	},
	{
		ID: "work", Name: "Focused work", GroupName: "Focused work",
		Description: "Blocks social media, ads, and malware during weekday working hours only.",
		Limitations: []string{
			"Working hours are 09:00-17:00 Monday to Friday; edit the rule to change them.",
		},
		Categories: []string{"malware"},
		Rules: []GroupRule{{
			ID: "work-hours", Name: "Working hours", Enabled: true, Action: ActionBlock,
			Target: RuleTarget{Categories: []string{"social", "ads"}},
			Schedule: &Schedule{
				Weekdays: []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday},
				Windows:  []TimeWindow{{From: "09:00", To: "17:00"}},
			},
		}},
		GroupID: "template-work", Available: true,
	},
	{
		ID: "iot", Name: "Smart devices", GroupName: "Smart devices",
		Description: "Blocks ads, trackers, and malware for TVs, speakers, and appliances.",
		Limitations: []string{
			"Does not identify IoT devices or replace network segmentation.",
		},
		Categories: []string{"ads", "malware"},
		GroupID:    "template-iot", Available: true,
	},
}

// PolicyTemplates returns a copy of the built-in catalog so callers cannot
// mutate process-wide definitions or share slice backing arrays.
func PolicyTemplates() []PolicyTemplate {
	result := make([]PolicyTemplate, len(builtInTemplates))
	for i, template := range builtInTemplates {
		result[i] = template.copy()
	}
	return result
}

// GetPolicyTemplate returns a copy of one built-in template.
func GetPolicyTemplate(id string) (PolicyTemplate, bool) {
	for _, template := range builtInTemplates {
		if template.ID == id {
			return template.copy(), true
		}
	}
	return PolicyTemplate{}, false
}

func (template PolicyTemplate) copy() PolicyTemplate {
	out := template
	out.Limitations = append([]string(nil), template.Limitations...)
	out.Categories = append([]string(nil), template.Categories...)
	out.Rules = copyRules(template.Rules)
	return out
}

func copyRules(rules []GroupRule) []GroupRule {
	if rules == nil {
		return nil
	}
	out := make([]GroupRule, len(rules))
	for i, rule := range rules {
		out[i] = rule
		out[i].Target.Domains = append([]string(nil), rule.Target.Domains...)
		out[i].Target.Categories = append([]string(nil), rule.Target.Categories...)
		out[i].URLRegexes = append([]string(nil), rule.URLRegexes...)
		out[i].BlockedContentTypes = append([]string(nil), rule.BlockedContentTypes...)
		out[i].Users = append([]string(nil), rule.Users...)
		if rule.Schedule != nil {
			schedule := *rule.Schedule
			schedule.Weekdays = append([]time.Weekday(nil), rule.Schedule.Weekdays...)
			schedule.Windows = append([]TimeWindow(nil), rule.Schedule.Windows...)
			out[i].Schedule = &schedule
		}
	}
	return out
}

// Group returns the ordinary editable policy record produced by this
// template, with every schedule set to the gateway's time zone. It carries no
// template-only field or behavior.
func (template PolicyTemplate) Group(timezone string) PolicyGroup {
	rules := copyRules(template.Rules)
	for i := range rules {
		if rules[i].Schedule != nil {
			rules[i].Schedule.Timezone = timezone
		}
	}
	return PolicyGroup{
		ID: template.GroupID, Name: template.GroupName, Description: template.Description,
		BlockedCategories: append([]string(nil), template.Categories...),
		SafeSearch:        template.SafeSearch,
		Rules:             rules,
	}
}
