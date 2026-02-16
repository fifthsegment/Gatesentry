package domainlist

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
)

const storageKey = "domain_lists"

// DomainListManager handles CRUD operations for domain lists, persists them
// to MapStore, and maintains the shared in-memory DomainListIndex.
type DomainListManager struct {
	storage *gatesentry2storage.MapStore
	Index   *DomainListIndex
	mu      sync.RWMutex // Protects the lists slice during CRUD operations
}

// NewDomainListManager creates a new manager backed by the given MapStore.
func NewDomainListManager(storage *gatesentry2storage.MapStore) *DomainListManager {
	return &DomainListManager{
		storage: storage,
		Index:   NewDomainListIndex(),
	}
}

// ---------- CRUD Operations ----------

// GetLists returns all domain lists (full objects).
func (m *DomainListManager) GetLists() ([]DomainList, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data := m.storage.Get(storageKey)
	if data == "" {
		return []DomainList{}, nil
	}

	var collection DomainListCollection
	err := json.Unmarshal([]byte(data), &collection)
	if err != nil {
		// Try plain array fallback (like RuleManager does)
		var lists []DomainList
		err2 := json.Unmarshal([]byte(data), &lists)
		if err2 != nil {
			log.Printf("[DomainList] Error unmarshaling domain lists: %v", err)
			return []DomainList{}, err
		}
		return lists, nil
	}

	return collection.Lists, nil
}

// GetListSummaries returns all domain lists as lightweight summaries
// (excludes the Domains array to avoid sending 300K entries over the wire).
func (m *DomainListManager) GetListSummaries() ([]DomainListSummary, error) {
	lists, err := m.GetLists()
	if err != nil {
		return nil, err
	}

	summaries := make([]DomainListSummary, len(lists))
	for i, dl := range lists {
		summaries[i] = dl.ToSummary()
	}
	return summaries, nil
}

// GetList retrieves a single domain list by ID. Returns nil if not found.
func (m *DomainListManager) GetList(id string) (*DomainList, error) {
	lists, err := m.GetLists()
	if err != nil {
		return nil, err
	}
	for i := range lists {
		if lists[i].ID == id {
			return &lists[i], nil
		}
	}
	return nil, nil
}

// AddList creates a new domain list and returns it with generated ID.
func (m *DomainListManager) AddList(dl DomainList) (DomainList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	lists, err := m.getListsUnsafe()
	if err != nil {
		return dl, err
	}

	// Generate ID and timestamps
	now := time.Now().Format(time.RFC3339)
	if dl.ID == "" {
		dl.ID = gatesentryUtils.RandomString(16)
	}
	dl.CreatedAt = now
	dl.LastUpdated = now

	// For local lists, compute entry count from domains
	if dl.Source == "local" && dl.Domains != nil {
		dl.EntryCount = len(ParseDomainsFromLines(dl.Domains))
	}

	lists = append(lists, dl)
	if err := m.saveListsUnsafe(lists); err != nil {
		return dl, err
	}

	// Index local list domains immediately
	if dl.Source == "local" && len(dl.Domains) > 0 {
		parsed := ParseDomainsFromLines(dl.Domains)
		m.Index.AddDomains(dl.ID, parsed)
	}

	return dl, nil
}

// UpdateList updates an existing domain list's metadata. For local lists,
// the domains array can also be updated. Preserves CreatedAt.
func (m *DomainListManager) UpdateList(id string, updated DomainList) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lists, err := m.getListsUnsafe()
	if err != nil {
		return err
	}

	for i, dl := range lists {
		if dl.ID == id {
			updated.ID = id
			updated.CreatedAt = dl.CreatedAt
			updated.LastUpdated = time.Now().Format(time.RFC3339)

			// For local lists, update domains and entry count
			if updated.Source == "local" {
				if updated.Domains != nil {
					parsed := ParseDomainsFromLines(updated.Domains)
					updated.EntryCount = len(parsed)
					// Re-index
					m.Index.ReplaceList(id, parsed)
				} else {
					// Preserve existing domains if none were provided
					updated.Domains = dl.Domains
					updated.EntryCount = dl.EntryCount
				}
			}

			lists[i] = updated
			return m.saveListsUnsafe(lists)
		}
	}

	return nil // Not found — no-op, consistent with RuleManager
}

// DeleteList removes a domain list by ID and removes it from the index.
func (m *DomainListManager) DeleteList(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lists, err := m.getListsUnsafe()
	if err != nil {
		return err
	}

	filtered := make([]DomainList, 0, len(lists))
	for _, dl := range lists {
		if dl.ID != id {
			filtered = append(filtered, dl)
		}
	}

	// Remove from index
	m.Index.RemoveList(id)

	return m.saveListsUnsafe(filtered)
}

// ---------- Domain CRUD (local lists only) ----------

// AddDomainsToList adds domains to a local list. No-op for URL-sourced lists.
func (m *DomainListManager) AddDomainsToList(id string, newDomains []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lists, err := m.getListsUnsafe()
	if err != nil {
		return err
	}

	for i, dl := range lists {
		if dl.ID == id {
			if dl.Source != "local" {
				log.Printf("[DomainList] Cannot add domains to URL-sourced list %s", id)
				return nil
			}

			// Build a set of existing domains for dedup
			existing := make(map[string]bool, len(dl.Domains))
			for _, d := range dl.Domains {
				existing[d] = true
			}

			added := 0
			for _, d := range newDomains {
				d = normalizeDomain(d)
				if d != "" && !existing[d] {
					dl.Domains = append(dl.Domains, d)
					existing[d] = true
					added++
				}
			}

			dl.EntryCount = len(dl.Domains)
			dl.LastUpdated = time.Now().Format(time.RFC3339)
			lists[i] = dl

			if err := m.saveListsUnsafe(lists); err != nil {
				return err
			}

			// Update index
			if added > 0 {
				parsed := ParseDomainsFromLines(dl.Domains)
				m.Index.ReplaceList(id, parsed)
			}

			return nil
		}
	}

	return nil // Not found
}

// RemoveDomainFromList removes a single domain from a local list.
func (m *DomainListManager) RemoveDomainFromList(id string, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	lists, err := m.getListsUnsafe()
	if err != nil {
		return err
	}

	for i, dl := range lists {
		if dl.ID == id {
			if dl.Source != "local" {
				log.Printf("[DomainList] Cannot remove domains from URL-sourced list %s", id)
				return nil
			}

			domain = normalizeDomain(domain)
			filtered := make([]string, 0, len(dl.Domains))
			for _, d := range dl.Domains {
				if normalizeDomain(d) != domain {
					filtered = append(filtered, d)
				}
			}

			dl.Domains = filtered
			dl.EntryCount = len(filtered)
			dl.LastUpdated = time.Now().Format(time.RFC3339)
			lists[i] = dl

			if err := m.saveListsUnsafe(lists); err != nil {
				return err
			}

			// Re-index
			parsed := ParseDomainsFromLines(dl.Domains)
			m.Index.ReplaceList(id, parsed)

			return nil
		}
	}

	return nil
}

// GetDomainsForList returns the domains for a list. For URL-sourced lists,
// this returns the indexed domains (from memory). For local lists, returns
// the stored domain array.
func (m *DomainListManager) GetDomainsForList(id string) ([]string, error) {
	dl, err := m.GetList(id)
	if err != nil {
		return nil, err
	}
	if dl == nil {
		return nil, nil
	}

	if dl.Source == "local" {
		return dl.Domains, nil
	}

	// For URL-sourced lists, we don't store domains in the JSON —
	// they're only in the index. Return nil (the UI can show entry count).
	return nil, nil
}

// IsDomainInList checks whether a domain is present in a specific list
// using the in-memory index (works for both local and URL-sourced lists).
func (m *DomainListManager) IsDomainInList(domain string, listID string) bool {
	if m.Index == nil {
		return false
	}
	return m.Index.IsDomainInList(domain, listID)
}

// IsDomainInAnyList checks whether a domain is present in any of the given lists
// using the in-memory index. Returns the first matching list ID, or empty string.
func (m *DomainListManager) IsDomainInAnyList(domain string, listIDs []string) (bool, string) {
	if m.Index == nil {
		return false, ""
	}
	for _, id := range listIDs {
		if m.Index.IsDomainInList(domain, id) {
			return true, id
		}
	}
	return false, ""
}

// ---------- Loading and Refreshing ----------

// LoadAllLists downloads URL-sourced lists and indexes local lists.
// Should be called at boot and by the scheduler for periodic refresh.
func (m *DomainListManager) LoadAllLists() {
	lists, err := m.GetLists()
	if err != nil {
		log.Printf("[DomainList] Error loading lists: %v", err)
		return
	}

	if len(lists) == 0 {
		log.Println("[DomainList] No domain lists configured")
		return
	}

	log.Printf("[DomainList] Loading %d domain lists...", len(lists))

	// Separate URL-sourced and local lists
	var urlLists []DomainList
	var localLists []DomainList
	for _, dl := range lists {
		if dl.Source == "url" && dl.URL != "" {
			urlLists = append(urlLists, dl)
		} else if dl.Source == "local" {
			localLists = append(localLists, dl)
		}
	}

	// Index local lists synchronously (they're small)
	for _, dl := range localLists {
		if len(dl.Domains) > 0 {
			parsed := ParseDomainsFromLines(dl.Domains)
			m.Index.ReplaceList(dl.ID, parsed)
			log.Printf("[DomainList] Indexed local list %q (%s): %d domains", dl.Name, dl.ID, len(parsed))
		}
	}

	// Download and index URL-sourced lists concurrently
	if len(urlLists) > 0 {
		m.downloadAndIndex(urlLists)
	}

	log.Printf("[DomainList] All lists loaded. Total unique domains in index: %d", m.Index.TotalDomains())
}

// RefreshList forces a re-download of a single URL-sourced list.
func (m *DomainListManager) RefreshList(id string) error {
	dl, err := m.GetList(id)
	if err != nil {
		return err
	}
	if dl == nil {
		return nil
	}
	if dl.Source != "url" {
		log.Printf("[DomainList] Cannot refresh local list %s", id)
		return nil
	}

	domains, err := FetchDomainsFromURL(dl.URL)
	if err != nil {
		return err
	}

	m.Index.ReplaceList(id, domains)

	// Update the entry count and timestamp in storage
	m.mu.Lock()
	defer m.mu.Unlock()

	lists, err := m.getListsUnsafe()
	if err != nil {
		return err
	}
	for i, l := range lists {
		if l.ID == id {
			lists[i].EntryCount = len(domains)
			lists[i].LastUpdated = time.Now().Format(time.RFC3339)
			break
		}
	}
	return m.saveListsUnsafe(lists)
}

// downloadAndIndex downloads URL-sourced lists concurrently and indexes them.
func (m *DomainListManager) downloadAndIndex(urlLists []DomainList) {
	type downloadResult struct {
		listID  string
		domains []string
		err     error
	}

	var wg sync.WaitGroup
	results := make(chan downloadResult, len(urlLists))

	for _, dl := range urlLists {
		wg.Add(1)
		go func(dl DomainList) {
			defer wg.Done()
			domains, err := FetchDomainsFromURL(dl.URL)
			results <- downloadResult{
				listID:  dl.ID,
				domains: domains,
				err:     err,
			}
		}(dl)
	}

	// Close channel when all downloads finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and update index + storage
	updatedCounts := make(map[string]int) // listID → count
	for result := range results {
		if result.err != nil {
			log.Printf("[DomainList] Error downloading list %s: %v", result.listID, result.err)
			continue
		}
		m.Index.ReplaceList(result.listID, result.domains)
		updatedCounts[result.listID] = len(result.domains)
		log.Printf("[DomainList] Indexed URL list %s: %d domains", result.listID, len(result.domains))
	}

	// Batch-update entry counts in storage
	if len(updatedCounts) > 0 {
		m.mu.Lock()
		lists, err := m.getListsUnsafe()
		if err == nil {
			now := time.Now().Format(time.RFC3339)
			for i, dl := range lists {
				if count, ok := updatedCounts[dl.ID]; ok {
					lists[i].EntryCount = count
					lists[i].LastUpdated = now
				}
			}
			m.saveListsUnsafe(lists)
		}
		m.mu.Unlock()
	}
}

// ---------- Internal helpers ----------

// getListsUnsafe reads lists from storage WITHOUT acquiring mu.
// Caller must hold mu.
func (m *DomainListManager) getListsUnsafe() ([]DomainList, error) {
	data := m.storage.Get(storageKey)
	if data == "" {
		return []DomainList{}, nil
	}

	var collection DomainListCollection
	err := json.Unmarshal([]byte(data), &collection)
	if err != nil {
		var lists []DomainList
		err2 := json.Unmarshal([]byte(data), &lists)
		if err2 != nil {
			return []DomainList{}, err
		}
		return lists, nil
	}

	return collection.Lists, nil
}

// saveListsUnsafe persists lists to storage WITHOUT acquiring mu.
// Caller must hold mu.
func (m *DomainListManager) saveListsUnsafe(lists []DomainList) error {
	collection := DomainListCollection{Lists: lists}
	data, err := json.Marshal(collection)
	if err != nil {
		log.Printf("[DomainList] Error marshaling domain lists: %v", err)
		return err
	}
	m.storage.Update(storageKey, string(data))
	return nil
}

// normalizeDomain lowercases a domain and strips trailing dots.
func normalizeDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	domain = strings.ToLower(domain)
	domain = strings.TrimSuffix(domain, ".")
	return domain
}
