package policy

// PolicyTemplate describes a safe starting point for an ordinary policy
// group. Templates are catalog data, not a second policy store: applying one
// creates a normal PolicyGroup that can be edited or deleted independently.
//
// A template states the categories it blocks and the coverage it does not
// have. Categories are upstream feeds, so coverage keeps updating without a
// Gatesentry release, but a category is never age verification, device
// classification, or a review of what a household actually uses.
type PolicyTemplate struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	GroupName   string       `json:"group_name"`
	Description string       `json:"description"`
	Protections []string     `json:"protections"`
	Limitations []string     `json:"limitations"`
	Action      PolicyAction `json:"action"`
	Domains     []string     `json:"domains"`
	Categories  []string     `json:"categories"`
	GroupID     string       `json:"group_id"`
	Available   bool         `json:"available"`
}

// Every starter is enforced by the DNS server for assigned devices, and DNS
// decides by domain only. The UI states both once for the whole catalog rather
// than repeating them inside every tile.
const templateEnforcementLimitation = "Enforced by the DNS server: only devices that use Gatesentry for DNS are covered, and only after you assign them to the group."
const templateDNSScopeLimitation = "DNS decides by domain: URL, MIME, keyword, and HTTPS inspection rules need the advanced editor and the proxy path."

var builtInTemplates = []PolicyTemplate{
	{
		ID: "child", Name: "Child starter", GroupName: "Child",
		Description: "Blocks adult, gambling, malware, and piracy categories. Review it before you assign a child's device.",
		Protections: []string{
			"Blocks the Adult content, Gambling, Malware and phishing, and Piracy categories from upstream feeds.",
			"Keeps the gateway default policy for every other domain.",
		},
		Limitations: []string{
			templateEnforcementLimitation,
			templateDNSScopeLimitation,
			"Categories are shared upstream feeds, not age verification or a review of what your household uses.",
		},
		Action: ActionBlock, Categories: []string{"adult", "gambling", "malware", "piracy"},
		GroupID: "template-child", Available: true,
	},
	{
		ID: "teen", Name: "Teen starter", GroupName: "Teen",
		Description: "Blocks adult, gambling, and malware categories, with room to add explicit domains.",
		Protections: []string{
			"Blocks the Adult content, Gambling, and Malware and phishing categories from upstream feeds.",
			"Keeps the gateway default policy for every other domain.",
		},
		Limitations: []string{
			templateEnforcementLimitation,
			templateDNSScopeLimitation,
			"Categories are shared upstream feeds, not age verification.",
		},
		Action: ActionBlock, Categories: []string{"adult", "gambling", "malware"},
		GroupID: "template-teen", Available: true,
	},
	{
		ID: "adult-default", Name: "Adult / default starter", GroupName: "Adult / default",
		Description: "The default-policy starting point for an adult or general household device.",
		Protections: []string{"Keeps the gateway's existing default policy and global filtering behavior."},
		Limitations: []string{
			templateEnforcementLimitation,
			templateDNSScopeLimitation,
			"Blocks nothing by itself. Add categories or domains before you assign a device.",
		},
		Action: ActionNone, GroupID: "template-adult-default", Available: true,
	},
	{
		ID: "guest", Name: "Guest starter", GroupName: "Guest",
		Description: "Guests keep the gateway default policy until you add categories or explicit domains.",
		Protections: []string{
			"Starts from the gateway's default policy and global blocklist.",
			"Ready for a category or domain list you choose for guests.",
		},
		Limitations: []string{
			templateEnforcementLimitation,
			templateDNSScopeLimitation,
			"This does not isolate guests or create a separate network; use network controls for isolation.",
		},
		Action: ActionNone, GroupID: "template-guest", Available: true,
	},
	{
		ID: "work", Name: "Work starter", GroupName: "Work",
		Description: "A work profile starter for the categories or domains you review before assigning it.",
		Protections: []string{
			"Keeps the gateway's existing default policy while you build a reviewed list.",
			"Categories are upstream feeds, so their coverage keeps updating without an edit.",
		},
		Limitations: []string{
			templateEnforcementLimitation,
			templateDNSScopeLimitation,
			"This does not classify business traffic or guarantee availability of any service.",
		},
		Action: ActionNone, GroupID: "template-work", Available: true,
	},
	{
		ID: "iot", Name: "IoT starter", GroupName: "IoT",
		Description: "An IoT profile starter for explicit device service domains; network segmentation stays a separate control.",
		Protections: []string{"Keeps the gateway's existing default policy while you build an explicit service domain list."},
		Limitations: []string{
			templateEnforcementLimitation,
			templateDNSScopeLimitation,
			"This does not identify IoT devices automatically or replace network segmentation.",
		},
		Action: ActionNone, GroupID: "template-iot", Available: true,
	},
	{
		ID: "unrestricted", Name: "Unrestricted starter", GroupName: "Unrestricted",
		Description: "A clearly labeled empty starter for a device that should have no additional group rules.",
		Protections: []string{"Adds no group rule; the device continues to use the gateway default policy."},
		Limitations: []string{
			templateEnforcementLimitation,
			templateDNSScopeLimitation,
			"Unrestricted does not bypass the global blocklist or existing advanced rules. Remove the assignment only when the gateway default is the desired behavior.",
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
		result[i].Protections = append([]string(nil), template.Protections...)
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
			copy.Protections = append([]string(nil), template.Protections...)
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
