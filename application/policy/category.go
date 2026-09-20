package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// Category is one named, self-updating set of domains. The catalog is release
// data; the domain contents come from the upstream feeds on every blocklist
// refresh. Categories exist because hand-typed domain lists do not survive
// real traffic: a group that blocks social media has to track hundreds of
// domains that change every week.
//
// Categories are feed data, not device classification. Gatesentry cannot infer
// age, work, guest, or IoT from a category, and a category never assigns a
// device to a group.
type Category struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Sources     []string `json:"sources"`
}

// EnabledCategoriesKey is the settings key holding the gateway-wide category
// selection as a JSON array of catalog IDs. Categories selected here apply to
// every client the gateway resolves for, which is the default policy. A policy
// group selects its own categories instead.
const EnabledCategoriesKey = "dns_categories_enabled"

// ErrUnknownCategory is returned when a selection names a category that is not
// in the catalog.
var ErrUnknownCategory = errors.New("unknown domain category")

// builtInCategories is the shipped catalog. Every entry points at an upstream
// list that is maintained independently of this repository, so blocking
// coverage updates without a Gatesentry release.
var builtInCategories = []Category{
	{
		ID:          "ads",
		Name:        "Ads and trackers",
		Description: "Ad, tracker, and telemetry domains from HaGeZi's Light list, which keeps the false-positive rate low.",
		Sources: []string{
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/light-onlydomains.txt",
		},
	},
	{
		ID:          "social",
		Name:        "Social media",
		Description: "Social networks and discussion sites such as Facebook, Instagram, TikTok, X, Reddit, and Quora. Messaging apps and streaming sites are not included.",
		Sources: []string{
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/social-onlydomains.txt",
		},
	},
	{
		ID:          "adult",
		Name:        "Adult content",
		Description: "Adult and pornographic domains.",
		Sources: []string{
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/nsfw-onlydomains.txt",
		},
	},
	{
		ID:          "gambling",
		Name:        "Gambling",
		Description: "Gambling and betting domains.",
		Sources: []string{
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/gambling-onlydomains.txt",
		},
	},
	{
		ID:          "piracy",
		Name:        "Piracy",
		Description: "Piracy, warez, and unauthorised streaming domains.",
		Sources: []string{
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/anti.piracy-onlydomains.txt",
		},
	},
	{
		ID:          "malware",
		Name:        "Malware and phishing",
		Description: "Malware, phishing, and abuse domains from HaGeZi's threat intelligence feed, which the default blocklist already loads.",
		Sources: []string{
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/tif-onlydomains.txt",
		},
	},
	{
		ID:          "shorteners",
		Name:        "Link shorteners",
		Description: "Link shorteners and redirector domains.",
		Sources: []string{
			"https://raw.githubusercontent.com/hagezi/dns-blocklists/main/wildcard/urlshortener-onlydomains.txt",
		},
	},
}

// CategoryCatalog returns a copy of the built-in catalog so callers cannot
// mutate the process-wide definitions.
func CategoryCatalog() []Category {
	out := make([]Category, 0, len(builtInCategories))
	for _, category := range builtInCategories {
		copyOfSources := append([]string(nil), category.Sources...)
		category.Sources = copyOfSources
		out = append(out, category)
	}
	return out
}

// GetCategory returns one catalog entry by ID.
func GetCategory(id string) (Category, bool) {
	for _, category := range builtInCategories {
		if category.ID == id {
			return category, true
		}
	}
	return Category{}, false
}

// NormalizeCategories trims, lowercases, de-duplicates, and validates a
// category selection. Unknown IDs are rejected so a typo cannot be stored as a
// rule that silently never matches.
func NormalizeCategories(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if _, ok := GetCategory(id); !ok {
			return nil, fmt.Errorf("%w %q", ErrUnknownCategory, raw)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// LoadEnabledCategories reads the gateway-wide category selection. A missing,
// empty, or unreadable value means no category is enabled gateway-wide: the
// caller must not treat that as an error, because category blocking is
// optional and the global blocklist is configured separately.
func LoadEnabledCategories(settings *gatesentry2storage.MapStore) []string {
	if settings == nil {
		return nil
	}
	raw, err := settings.GetE(EnabledCategoriesKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil
	}
	normalized, err := NormalizeCategories(ids)
	if err != nil {
		return nil
	}
	return normalized
}

// CategoryStatus is one catalog entry with its current coverage and its
// gateway-wide state, for the API and the UI.
type CategoryStatus struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DomainCount int    `json:"domain_count"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	Enabled     bool   `json:"enabled"`
}

// CategoryIndex holds the downloaded domain set for each category. The DNS
// blocklist refresh is the only writer; policy evaluation only reads. A nil
// index is valid and means "no category data loaded yet": category rules then
// match nothing rather than blocking or allowing by accident.
type CategoryIndex struct {
	mu      sync.RWMutex
	sets    map[string]map[string]bool
	updated map[string]time.Time
}

// NewCategoryIndex creates an empty index.
func NewCategoryIndex() *CategoryIndex {
	return &CategoryIndex{
		sets:    map[string]map[string]bool{},
		updated: map[string]time.Time{},
	}
}

// Replace swaps one category's domain set atomically. Readers of a previously
// returned set keep the set they already have, so a refresh never exposes a
// half-built map.
func (i *CategoryIndex) Replace(categoryID string, domains []string, at time.Time) {
	if i == nil {
		return
	}
	set := make(map[string]bool, len(domains))
	for _, domain := range domains {
		if normalized := normalizeCategoryDomain(domain); normalized != "" {
			set[normalized] = true
		}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sets == nil {
		i.sets = map[string]map[string]bool{}
	}
	if i.updated == nil {
		i.updated = map[string]time.Time{}
	}
	i.sets[categoryID] = set
	i.updated[categoryID] = at
}

// Contains reports whether a category covers a domain or one of its parent
// domains. Upstream feeds list registrable domains without their subdomains,
// so "facebook.com" in a category has to cover "www.facebook.com" for the
// category to mean anything on real traffic.
func (i *CategoryIndex) Contains(categoryID, domain string) bool {
	if i == nil {
		return false
	}
	i.mu.RLock()
	set := i.sets[categoryID]
	i.mu.RUnlock()
	if len(set) == 0 {
		return false
	}
	// A set is replaced, never mutated, so it is safe to read without the lock.
	for candidate := normalizeCategoryDomain(domain); candidate != ""; {
		if set[candidate] {
			return true
		}
		idx := strings.IndexByte(candidate, '.')
		if idx < 0 {
			return false
		}
		candidate = candidate[idx+1:]
	}
	return false
}

// Count reports how many domains a category currently covers.
func (i *CategoryIndex) Count(categoryID string) int {
	if i == nil {
		return 0
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.sets[categoryID])
}

// Drop releases one category's domain set. A category that is neither enabled
// gateway-wide nor selected by a policy group is dropped on the next refresh so
// an unused feed does not stay resident.
func (i *CategoryIndex) Drop(categoryID string) {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.sets, categoryID)
	delete(i.updated, categoryID)
}

// UpdatedAt reports when a category was last refreshed, or the zero time when
// it has never been downloaded.
func (i *CategoryIndex) UpdatedAt(categoryID string) time.Time {
	if i == nil {
		return time.Time{}
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.updated[categoryID]
}

// normalizeCategoryDomain lowercases a feed or query name and strips the
// decorations a hosts-format line or a wildcard feed entry can carry.
func normalizeCategoryDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "*.")
	domain = strings.TrimSuffix(domain, ".")
	return domain
}

// EnabledCategories returns the gateway-wide category selection.
func (s *Service) EnabledCategories() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.enabledCategories...)
}

// EnabledCategoryMatch reports whether a gateway-wide enabled category covers
// the domain, and which category it is. Gateway-wide categories apply to every
// client the gateway resolves for, which is what makes them the default policy:
// the DNS adapter consults this for queries the exact-match blocklist map does
// not cover, so a feed entry for "facebook.com" also blocks
// "www.facebook.com". An empty result means no enabled category covers the
// domain and the caller keeps its existing decision.
func (s *Service) EnabledCategoryMatch(domain string) (string, bool) {
	if s == nil || domain == "" {
		return "", false
	}
	s.mu.RLock()
	ids := s.enabledCategories
	index := s.categories
	s.mu.RUnlock()
	for _, id := range ids {
		if index.Contains(id, domain) {
			return id, true
		}
	}
	return "", false
}

// SetEnabledCategories validates and persists the gateway-wide category
// selection. It does not download anything: the DNS blocklist refresh is the
// only writer of domain data, and the caller requests a refresh after saving.
func (s *Service) SetEnabledCategories(ids []string) error {
	if s.storage == nil {
		return fmt.Errorf("policy service has no storage")
	}
	normalized, err := NormalizeCategories(ids)
	if err != nil {
		return err
	}
	if normalized == nil {
		normalized = []string{}
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Errorf("encode enabled categories: %w", err)
	}
	if err := s.storage.Update(EnabledCategoriesKey, string(encoded)); err != nil {
		return err
	}
	s.mu.Lock()
	s.enabledCategories = normalized
	s.mu.Unlock()
	return nil
}

// SetCategoryIndex attaches the downloaded category index. Startup calls it
// once before the DNS listener accepts queries; the blocklist refresh is the
// only writer of the index contents afterwards.
func (s *Service) SetCategoryIndex(index *CategoryIndex) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.categories = index
}

// CategoryStatuses reports the catalog with its current coverage and its
// gateway-wide state, so the UI never has to guess what a category contains.
func (s *Service) CategoryStatuses() []CategoryStatus {
	enabled := make(map[string]bool)
	for _, id := range s.EnabledCategories() {
		enabled[id] = true
	}
	index := s.categories
	catalog := CategoryCatalog()
	statuses := make([]CategoryStatus, 0, len(catalog))
	for _, category := range catalog {
		status := CategoryStatus{
			ID:          category.ID,
			Name:        category.Name,
			Description: category.Description,
			DomainCount: index.Count(category.ID),
			Enabled:     enabled[category.ID],
		}
		if updated := index.UpdatedAt(category.ID); !updated.IsZero() {
			status.UpdatedAt = updated.UTC().Format(time.RFC3339)
		}
		statuses = append(statuses, status)
	}
	return statuses
}
