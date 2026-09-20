package policy

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
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	GroupName   string       `json:"group_name"`
	Description string       `json:"description"`
	Limitations []string     `json:"limitations"`
	Action      PolicyAction `json:"action"`
	Domains     []string     `json:"domains"`
	Categories  []string     `json:"categories"`
	GroupID     string       `json:"group_id"`
	Available   bool         `json:"available"`
}

// Every starter is enforced by the DNS server for assigned devices, and DNS
// decides by domain only. These hold for the whole catalog, so they are stated
// once instead of inside every template.
var templateSharedLimitations = []string{
	"Enforced by the DNS server: only devices that use Gatesentry for DNS are covered, and only after you assign them to the group.",
	"DNS decides by domain: URL and response-type conditions need a block rule with TLS inspection, which the proxy enforces.",
}

// TemplateSharedLimitations returns a copy of the caveats that apply to every
// starter, so a caller can state them once above the catalog.
func TemplateSharedLimitations() []string {
	return append([]string(nil), templateSharedLimitations...)
}

var builtInTemplates = []PolicyTemplate{
	{
		ID: "child", Name: "Child starter", GroupName: "Child",
		Description: "Blocks the categories below for a younger child's device.",
		Limitations: []string{
			"Categories are shared upstream feeds, not age verification.",
		},
		Action: ActionBlock, Categories: []string{"adult", "gambling", "malware", "piracy"},
		GroupID: "template-child", Available: true,
	},
	{
		ID: "teen", Name: "Teen starter", GroupName: "Teen",
		Description: "Blocks the categories below for a teenager's device, with room to add domains.",
		Limitations: []string{
			"Categories are shared upstream feeds, not age verification.",
		},
		Action: ActionBlock, Categories: []string{"adult", "gambling", "malware"},
		GroupID: "template-teen", Available: true,
	},
	{
		ID: "adult-default", Name: "Adult / default starter", GroupName: "Adult / default",
		Description: "For an adult or general household device. Adds no rules of its own.",
		Limitations: []string{
			"Blocks nothing by itself. Add categories or domains before you assign it.",
		},
		Action: ActionNone, GroupID: "template-adult-default", Available: true,
	},
	{
		ID: "guest", Name: "Guest starter", GroupName: "Guest",
		Description: "For visitor devices. Keeps the gateway default policy until you add rules.",
		Limitations: []string{
			"Does not isolate guests or create a separate network; use network controls for that.",
		},
		Action: ActionNone, GroupID: "template-guest", Available: true,
	},
	{
		ID: "work", Name: "Work starter", GroupName: "Work",
		Description: "For a work device. Starts empty so you can review every rule you add.",
		Limitations: []string{
			"Does not classify business traffic or guarantee that a service stays reachable.",
		},
		Action: ActionNone, GroupID: "template-work", Available: true,
	},
	{
		ID: "iot", Name: "IoT starter", GroupName: "IoT",
		Description: "For an appliance or smart device. Add the service domains it needs.",
		Limitations: []string{
			"Does not identify IoT devices or replace network segmentation.",
		},
		Action: ActionNone, GroupID: "template-iot", Available: true,
	},
	{
		ID: "unrestricted", Name: "Unrestricted starter", GroupName: "Unrestricted",
		Description: "For a device that should get no group rules.",
		Limitations: []string{
			"Does not bypass the global blocklist or the gateway default policy.",
		},
		Action: ActionNone, GroupID: "template-unrestricted", Available: true,
	},
}

// PolicyTemplates returns a copy of the built-in catalog so callers cannot
// mutate process-wide definitions or share slice backing arrays.
func PolicyTemplates() []PolicyTemplate {
	result := make([]PolicyTemplate, len(builtInTemplates))
	for i, template := range builtInTemplates {
		result[i] = template
		result[i].Limitations = append([]string(nil), template.Limitations...)
		result[i].Domains = append([]string(nil), template.Domains...)
		result[i].Categories = append([]string(nil), template.Categories...)
	}
	return result
}

// GetPolicyTemplate returns a copy of one built-in template.
func GetPolicyTemplate(id string) (PolicyTemplate, bool) {
	for _, template := range builtInTemplates {
		if template.ID == id {
			copy := template
			copy.Limitations = append([]string(nil), template.Limitations...)
			copy.Domains = append([]string(nil), template.Domains...)
			copy.Categories = append([]string(nil), template.Categories...)
			return copy, true
		}
	}
	return PolicyTemplate{}, false
}

// Group returns the ordinary editable policy record produced by this
// template. It intentionally carries no template-only field or behavior.
func (template PolicyTemplate) Group() PolicyGroup {
	return PolicyGroup{
		ID: template.GroupID, Name: template.GroupName, Description: template.Description,
		Domains: append([]string(nil), template.Domains...), Action: template.Action,
		Categories: append([]string(nil), template.Categories...),
	}
}
