package domainlist

import (
	"sync"
)

// DomainListIndex provides O(1) domain lookup across all loaded domain lists.
// It maps each domain to the set of list IDs that contain it.
//
// Thread-safe via sync.RWMutex — concurrent reads do not block each other,
// writes (rebuild/swap) acquire an exclusive lock.
type DomainListIndex struct {
	// domains maps lowercase domain → set of list IDs that contain it.
	// Inner map: listID → true.
	domains map[string]map[string]bool
	mu      sync.RWMutex
}

// NewDomainListIndex creates an empty index.
func NewDomainListIndex() *DomainListIndex {
	return &DomainListIndex{
		domains: make(map[string]map[string]bool),
	}
}

// IsDomainInList checks whether a domain is present in a specific list.
// O(1) lookup. Thread-safe for concurrent reads.
func (idx *DomainListIndex) IsDomainInList(domain string, listID string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	lists, ok := idx.domains[domain]
	if !ok {
		return false
	}
	return lists[listID]
}

// IsDomainInAnyList checks whether a domain is present in ANY of the given lists.
// Returns true on first match. Thread-safe for concurrent reads.
func (idx *DomainListIndex) IsDomainInAnyList(domain string, listIDs []string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	lists, ok := idx.domains[domain]
	if !ok {
		return false
	}
	for _, id := range listIDs {
		if lists[id] {
			return true
		}
	}
	return false
}

// GetListsForDomain returns the set of list IDs that contain the given domain.
// Returns nil if the domain is not indexed. Thread-safe.
func (idx *DomainListIndex) GetListsForDomain(domain string) map[string]bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	lists, ok := idx.domains[domain]
	if !ok {
		return nil
	}

	// Return a copy to avoid race conditions
	result := make(map[string]bool, len(lists))
	for k, v := range lists {
		result[k] = v
	}
	return result
}

// AddDomains adds a slice of domains to the index under the given list ID.
// Acquires a write lock. Used during initial load and refresh.
func (idx *DomainListIndex) AddDomains(listID string, domains []string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, domain := range domains {
		if _, ok := idx.domains[domain]; !ok {
			idx.domains[domain] = make(map[string]bool)
		}
		idx.domains[domain][listID] = true
	}
}

// RemoveList removes all entries for a specific list ID from the index.
// This is used when a domain list is deleted or before it is reloaded.
func (idx *DomainListIndex) RemoveList(listID string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for domain, lists := range idx.domains {
		delete(lists, listID)
		// Clean up empty domain entries to avoid memory leaks
		if len(lists) == 0 {
			delete(idx.domains, domain)
		}
	}
}

// ReplaceList atomically removes all domains for a list and adds new ones.
// Used during refresh — avoids a window where the list is empty.
func (idx *DomainListIndex) ReplaceList(listID string, domains []string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// Remove old entries for this list
	for domain, lists := range idx.domains {
		delete(lists, listID)
		if len(lists) == 0 {
			delete(idx.domains, domain)
		}
	}

	// Add new entries
	for _, domain := range domains {
		if _, ok := idx.domains[domain]; !ok {
			idx.domains[domain] = make(map[string]bool)
		}
		idx.domains[domain][listID] = true
	}
}

// TotalDomains returns the total number of unique domains in the index.
func (idx *DomainListIndex) TotalDomains() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.domains)
}

// CountForList returns the number of domains associated with a specific list ID.
func (idx *DomainListIndex) CountForList(listID string) int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	count := 0
	for _, lists := range idx.domains {
		if lists[listID] {
			count++
		}
	}
	return count
}

// Clear removes all entries from the index.
func (idx *DomainListIndex) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.domains = make(map[string]map[string]bool)
}
