package domainlist

// DomainList represents a named, reusable collection of domains.
// It is a pure data container — its purpose (block/allow) is determined
// by who references it (DNS config, proxy rules), not by any flag here.
type DomainList struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"` // e.g., "Ads", "Malware", "Adult", "Social Media", "Custom"
	Source      string   `json:"source"`   // "url" or "local" (mutually exclusive)
	URL         string   `json:"url,omitempty"`
	Domains     []string `json:"domains,omitempty"` // Only for source="local" — admin-managed entries
	EntryCount  int      `json:"entry_count"`       // Computed — number of domains loaded into index
	LastUpdated string   `json:"last_updated"`
	CreatedAt   string   `json:"created_at"`
}

// DomainListCollection is the top-level wrapper for JSON persistence.
type DomainListCollection struct {
	Lists []DomainList `json:"lists"`
}

// DomainListSummary is a lightweight view returned by list-all endpoints
// (excludes the Domains array to avoid sending 300K entries over the wire).
type DomainListSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Source      string `json:"source"`
	URL         string `json:"url,omitempty"`
	EntryCount  int    `json:"entry_count"`
	LastUpdated string `json:"last_updated"`
	CreatedAt   string `json:"created_at"`
}

// ToSummary converts a DomainList to its lightweight summary form.
func (dl *DomainList) ToSummary() DomainListSummary {
	return DomainListSummary{
		ID:          dl.ID,
		Name:        dl.Name,
		Description: dl.Description,
		Category:    dl.Category,
		Source:      dl.Source,
		URL:         dl.URL,
		EntryCount:  dl.EntryCount,
		LastUpdated: dl.LastUpdated,
		CreatedAt:   dl.CreatedAt,
	}
}
