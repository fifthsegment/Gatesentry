package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"strings"

	"bitbucket.org/abdullah_irfan/gatesentryf/domainlist"
	"github.com/gorilla/mux"
)

// DomainListManagerInterface defines the interface for domain list management.
// This allows the webserver endpoints to be decoupled from the concrete implementation.
type DomainListManagerInterface interface {
	GetLists() ([]domainlist.DomainList, error)
	GetListSummaries() ([]domainlist.DomainListSummary, error)
	GetList(id string) (*domainlist.DomainList, error)
	AddList(dl domainlist.DomainList) (domainlist.DomainList, error)
	UpdateList(id string, updated domainlist.DomainList) error
	DeleteList(id string) error
	RefreshList(id string) error
	AddDomainsToList(id string, domains []string) error
	RemoveDomainFromList(id string, domain string) error
	GetDomainsForList(id string) ([]string, error)
}

var domainListManager DomainListManagerInterface

// InitDomainListManager initializes the domain list manager with an implementation.
func InitDomainListManager(dlm DomainListManagerInterface) {
	domainListManager = dlm
}

// GetDomainListManager returns the global domain list manager instance.
func GetDomainListManager() DomainListManagerInterface {
	return domainListManager
}

// ---------- List CRUD Handlers ----------

// GSApiDomainListsGetAll returns all domain lists as summaries (no domains array).
func GSApiDomainListsGetAll(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	summaries, err := domainListManager.GetListSummaries()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"lists": summaries,
	})
}

// GSApiDomainListGet returns a single domain list by ID (full object, minus domains for URL lists).
func GSApiDomainListGet(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	dl, err := domainListManager.GetList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if dl == nil {
		http.Error(w, "Domain list not found", http.StatusNotFound)
		return
	}

	// For URL-sourced lists, don't send the domains array (it's huge and not stored)
	if dl.Source == "url" {
		dl.Domains = nil
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dl)
}

// GSApiDomainListCreate creates a new domain list.
func GSApiDomainListCreate(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	var dl domainlist.DomainList
	err := json.NewDecoder(r.Body).Decode(&dl)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if dl.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if dl.Source != "url" && dl.Source != "local" {
		http.Error(w, "Source must be 'url' or 'local'", http.StatusBadRequest)
		return
	}
	if dl.Source == "url" && dl.URL == "" {
		http.Error(w, "URL is required for URL-sourced lists", http.StatusBadRequest)
		return
	}

	created, err := domainListManager.AddList(dl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Domain list created successfully",
		"list":    created.ToSummary(),
	})
}

// GSApiDomainListUpdate updates an existing domain list.
func GSApiDomainListUpdate(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	var dl domainlist.DomainList
	err := json.NewDecoder(r.Body).Decode(&dl)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if dl.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}

	err = domainListManager.UpdateList(id, dl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Domain list updated successfully",
	})
}

// GSApiDomainListDelete deletes a domain list.
func GSApiDomainListDelete(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	err := domainListManager.DeleteList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Domain list deleted successfully",
	})
}

// GSApiDomainListRefresh forces a re-download of a URL-sourced list.
func GSApiDomainListRefresh(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	err := domainListManager.RefreshList(id)
	if err != nil {
		http.Error(w, "Refresh failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated list summary
	dl, err := domainListManager.GetList(id)
	if err != nil || dl == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Domain list refreshed successfully",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Domain list refreshed successfully",
		"list":    dl.ToSummary(),
	})
}

// ---------- Domain CRUD Handlers (local lists only) ----------

// GSApiDomainListDomainsGet returns the domains in a list.
func GSApiDomainListDomainsGet(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	domains, err := domainListManager.GetDomainsForList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"domains": domains,
	})
}

// GSApiDomainListDomainsAdd adds domains to a local list.
func GSApiDomainListDomainsAdd(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	var body struct {
		Domains []string `json:"domains"`
	}
	err := json.NewDecoder(r.Body).Decode(&body)
	if err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if len(body.Domains) == 0 {
		http.Error(w, "At least one domain is required", http.StatusBadRequest)
		return
	}

	err = domainListManager.AddDomainsToList(id, body.Domains)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Domains added successfully",
	})
}

// GSApiDomainListDomainRemove removes a single domain from a local list.
func GSApiDomainListDomainRemove(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	domain := vars["domain"]

	err := domainListManager.RemoveDomainFromList(id, domain)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Domain removed successfully",
	})
}

// ---------- Lookup Handler ----------

// GSApiDomainListCheck tests if a domain is in a specific list.
func GSApiDomainListCheck(w http.ResponseWriter, r *http.Request) {
	if domainListManager == nil {
		http.Error(w, "Domain list manager not initialized", http.StatusInternalServerError)
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	domain := vars["domain"]

	// We need access to the index — get it from the concrete manager.
	// The interface doesn't expose the index directly, but we can check
	// by trying to get the list and checking membership.
	dl, err := domainListManager.GetList(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if dl == nil {
		http.Error(w, "Domain list not found", http.StatusNotFound)
		return
	}

	// For local lists, check the domains array directly
	found := false
	if dl.Source == "local" {
		domain = normalizeForCheck(domain)
		for _, d := range dl.Domains {
			if normalizeForCheck(d) == domain {
				found = true
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"domain": domain,
		"list":   id,
		"found":  found,
	})
}

func normalizeForCheck(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.TrimSuffix(s, ".")
	return s
}
