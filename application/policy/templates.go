package policy

// PolicyTemplate describes a safe starting point for an ordinary policy
// group. Templates are catalog data, not a second policy store: applying one
// creates a normal PolicyGroup that can be edited or deleted independently.
//
// The current policy engine has no bundled age, work, IoT, or content
// category feeds. The catalog therefore describes default behavior and
// limitations instead of pretending that a label supplies coverage.
type PolicyTemplate struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	GroupName   string       `json:"group_name"`
	Description string       `json:"description"`
	Protections []string     `json:"protections"`
	Limitations []string     `json:"limitations"`
	Action      PolicyAction `json:"action"`
	Domains     []string     `json:"domains"`
	GroupID     string       `json:"group_id"`
	Available   bool         `json:"available"`
}

const templatePolicyLimitation = "No bundled category feed is configured; this starter does not infer age, work, guest, or IoT classifications."
const templateDNSLimitation = "DNS group policy covers explicit domain patterns only; URL, MIME, keyword, schedule, and HTTPS inspection rules require the advanced editor and their relevant enforcement path."

var builtInTemplates = []PolicyTemplate{
	{
		ID: "child", Name: "Child starter", GroupName: "Child",
		Description: "A transparent child profile starter for families who want to add explicit domains before assigning the profile to a device.",
		Protections: []string{"Uses the gateway default policy, including any currently configured global blocklist."},
		Limitations: []string{templatePolicyLimitation, templateDNSLimitation},
		Action:      ActionNone, GroupID: "template-child", Available: true,
	},
	{
		ID: "teen", Name: "Teen starter", GroupName: "Teen",
		Description: "A transparent teen profile starter with an editable domain list and the gateway default behavior until rules are added.",
		Protections: []string{"Uses the gateway default policy, including any currently configured global blocklist."},
		Limitations: []string{templatePolicyLimitation, templateDNSLimitation},
		Action:      ActionNone, GroupID: "template-teen", Available: true,
	},
	{
		ID: "adult-default", Name: "Adult / default starter", GroupName: "Adult / default",
		Description: "The default-policy starting point for an adult or general household device.",
		Protections: []string{"Keeps the gateway's existing default policy and global filtering behavior."},
		Limitations: []string{"No additional domains are blocked or allowed until you edit this group.", templateDNSLimitation},
		Action:      ActionNone, GroupID: "template-adult-default", Available: true,
	},
	{
		ID: "guest", Name: "Guest starter", GroupName: "Guest",
		Description: "A guest profile starter that can receive explicit domain rules without changing the gateway's default policy for other devices.",
		Protections: []string{"Keeps the gateway's existing default policy for this group until you add explicit rules."},
		Limitations: []string{"This does not isolate guests or create a separate network; use network controls for isolation.", templateDNSLimitation},
		Action:      ActionNone, GroupID: "template-guest", Available: true,
	},
	{
		ID: "work", Name: "Work starter", GroupName: "Work",
		Description: "A work profile starter for explicit allow or block domains that you review before assigning it.",
		Protections: []string{"Keeps the gateway's existing default policy while you build a reviewed work domain list."},
		Limitations: []string{"This does not classify business traffic or guarantee availability of any service.", templateDNSLimitation},
		Action:      ActionNone, GroupID: "template-work", Available: true,
	},
	{
		ID: "iot", Name: "IoT starter", GroupName: "IoT",
		Description: "An IoT profile starter for explicit device service domains; network segmentation remains a separate control.",
		Protections: []string{"Keeps the gateway's existing default policy while you build an explicit service domain list."},
		Limitations: []string{"This does not identify IoT devices automatically or replace network segmentation.", templateDNSLimitation},
		Action:      ActionNone, GroupID: "template-iot", Available: true,
	},
	{
		ID: "unrestricted", Name: "Unrestricted starter", GroupName: "Unrestricted",
		Description: "A clearly labeled empty starter for a device that should have no additional group rules.",
		Protections: []string{"Adds no group rule; the device continues to use the gateway default policy."},
		Limitations: []string{"Unrestricted does not bypass the global blocklist or existing advanced rules. Remove the assignment only when the gateway default is the desired behavior.", templateDNSLimitation},
		Action:      ActionNone, GroupID: "template-unrestricted", Available: true,
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
	}
}
