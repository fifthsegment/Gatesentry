package domainlist

import (
	"os"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// setupTestManager creates a DomainListManager backed by a temp directory.
// Returns the manager and a cleanup function.
func setupTestManager(t *testing.T) (*DomainListManager, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "domainlist-test-*")
	if err != nil {
		t.Fatal(err)
	}

	// Set the storage base dir to our temp directory
	origBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(tmpDir + "/")

	store := gatesentry2storage.NewMapStore("test_domainlists", false)
	manager := NewDomainListManager(store)

	cleanup := func() {
		gatesentry2storage.SetBaseDir(origBaseDir)
		os.RemoveAll(tmpDir)
	}

	return manager, cleanup
}

// ---------- Index Tests ----------

func TestIndexAddAndLookup(t *testing.T) {
	idx := NewDomainListIndex()

	domains := []string{"example.com", "ads.tracker.com", "malware.bad.org"}
	idx.AddDomains("list1", domains)

	if !idx.IsDomainInList("example.com", "list1") {
		t.Error("expected example.com to be in list1")
	}
	if idx.IsDomainInList("example.com", "list2") {
		t.Error("expected example.com NOT to be in list2")
	}
	if !idx.IsDomainInList("malware.bad.org", "list1") {
		t.Error("expected malware.bad.org to be in list1")
	}
	if idx.IsDomainInList("clean.example.com", "list1") {
		t.Error("expected clean.example.com NOT to be in list1")
	}
}

func TestIndexIsDomainInAnyList(t *testing.T) {
	idx := NewDomainListIndex()

	idx.AddDomains("ads", []string{"tracker.com", "ads.example.com"})
	idx.AddDomains("malware", []string{"evil.org", "malware.net"})
	idx.AddDomains("social", []string{"facebook.com", "twitter.com"})

	tests := []struct {
		domain  string
		listIDs []string
		want    bool
	}{
		{"tracker.com", []string{"ads", "malware"}, true},
		{"evil.org", []string{"ads", "malware"}, true},
		{"facebook.com", []string{"ads", "malware"}, false},
		{"facebook.com", []string{"social"}, true},
		{"facebook.com", []string{"ads", "social"}, true},
		{"unknown.com", []string{"ads", "malware", "social"}, false},
	}

	for _, tt := range tests {
		got := idx.IsDomainInAnyList(tt.domain, tt.listIDs)
		if got != tt.want {
			t.Errorf("IsDomainInAnyList(%q, %v) = %v, want %v", tt.domain, tt.listIDs, got, tt.want)
		}
	}
}

func TestIndexRemoveList(t *testing.T) {
	idx := NewDomainListIndex()

	idx.AddDomains("list1", []string{"shared.com", "only-in-1.com"})
	idx.AddDomains("list2", []string{"shared.com", "only-in-2.com"})

	idx.RemoveList("list1")

	if idx.IsDomainInList("shared.com", "list1") {
		t.Error("shared.com should not be in list1 after removal")
	}
	if !idx.IsDomainInList("shared.com", "list2") {
		t.Error("shared.com should still be in list2")
	}
	if idx.IsDomainInList("only-in-1.com", "list1") {
		t.Error("only-in-1.com should not be in index after list1 removal")
	}
	if _, ok := idx.domains["only-in-1.com"]; ok {
		t.Error("only-in-1.com should be cleaned up from domains map entirely")
	}
}

func TestIndexReplaceList(t *testing.T) {
	idx := NewDomainListIndex()

	idx.AddDomains("list1", []string{"old1.com", "old2.com"})

	idx.ReplaceList("list1", []string{"new1.com", "new2.com"})

	if idx.IsDomainInList("old1.com", "list1") {
		t.Error("old1.com should not be in list1 after replace")
	}
	if !idx.IsDomainInList("new1.com", "list1") {
		t.Error("new1.com should be in list1 after replace")
	}
	if !idx.IsDomainInList("new2.com", "list1") {
		t.Error("new2.com should be in list1 after replace")
	}
}

func TestIndexTotalDomains(t *testing.T) {
	idx := NewDomainListIndex()

	idx.AddDomains("list1", []string{"a.com", "b.com", "c.com"})
	idx.AddDomains("list2", []string{"b.com", "d.com"}) // b.com is shared

	// Total unique domains: a.com, b.com, c.com, d.com = 4
	if got := idx.TotalDomains(); got != 4 {
		t.Errorf("TotalDomains() = %d, want 4", got)
	}
}

func TestIndexGetListsForDomain(t *testing.T) {
	idx := NewDomainListIndex()

	idx.AddDomains("list1", []string{"shared.com"})
	idx.AddDomains("list2", []string{"shared.com"})
	idx.AddDomains("list3", []string{"unique.com"})

	lists := idx.GetListsForDomain("shared.com")
	if len(lists) != 2 {
		t.Errorf("expected 2 lists for shared.com, got %d", len(lists))
	}
	if !lists["list1"] || !lists["list2"] {
		t.Error("expected list1 and list2 for shared.com")
	}

	lists = idx.GetListsForDomain("notfound.com")
	if lists != nil {
		t.Error("expected nil for non-existent domain")
	}
}

func TestIndexClear(t *testing.T) {
	idx := NewDomainListIndex()
	idx.AddDomains("list1", []string{"a.com", "b.com"})
	idx.Clear()

	if got := idx.TotalDomains(); got != 0 {
		t.Errorf("TotalDomains() after Clear() = %d, want 0", got)
	}
}

// ---------- Loader Tests ----------

func TestParseDomainsFromLines(t *testing.T) {
	lines := []string{
		"example.com",
		"  UPPER.COM  ",
		"trailing.dot.com.",
		"",
		"# comment line",
		"  # another comment",
		"normal.org",
	}

	got := ParseDomainsFromLines(lines)
	want := []string{"example.com", "upper.com", "trailing.dot.com", "normal.org"}

	if len(got) != len(want) {
		t.Fatalf("ParseDomainsFromLines returned %d domains, want %d", len(got), len(want))
	}
	for i, d := range got {
		if d != want[i] {
			t.Errorf("domain[%d] = %q, want %q", i, d, want[i])
		}
	}
}

func TestParseDomainsFromLinesEmpty(t *testing.T) {
	got := ParseDomainsFromLines(nil)
	if got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}

	got = ParseDomainsFromLines([]string{})
	if got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}

	got = ParseDomainsFromLines([]string{"", "  ", "# only comments"})
	if got != nil {
		t.Errorf("expected nil for comments-only input, got %v", got)
	}
}

// ---------- Manager CRUD Tests ----------

func TestManagerAddAndGetList(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	dl := DomainList{
		Name:     "Test Ads",
		Source:   "local",
		Category: "Ads",
		Domains:  []string{"ads.example.com", "tracker.net"},
	}

	created, err := manager.AddList(dl)
	if err != nil {
		t.Fatalf("AddList failed: %v", err)
	}
	if created.ID == "" {
		t.Error("expected generated ID")
	}
	if created.CreatedAt == "" {
		t.Error("expected CreatedAt timestamp")
	}
	if created.EntryCount != 2 {
		t.Errorf("EntryCount = %d, want 2", created.EntryCount)
	}

	// Retrieve it
	fetched, err := manager.GetList(created.ID)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("GetList returned nil")
	}
	if fetched.Name != "Test Ads" {
		t.Errorf("Name = %q, want %q", fetched.Name, "Test Ads")
	}
	if len(fetched.Domains) != 2 {
		t.Errorf("Domains count = %d, want 2", len(fetched.Domains))
	}

	// Should be indexed
	if !manager.Index.IsDomainInList("ads.example.com", created.ID) {
		t.Error("expected ads.example.com to be indexed")
	}
}

func TestManagerGetLists(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	// Empty initially
	lists, err := manager.GetLists()
	if err != nil {
		t.Fatalf("GetLists failed: %v", err)
	}
	if len(lists) != 0 {
		t.Errorf("expected 0 lists, got %d", len(lists))
	}

	// Add two lists
	manager.AddList(DomainList{Name: "List 1", Source: "local", Category: "Ads"})
	manager.AddList(DomainList{Name: "List 2", Source: "url", URL: "https://example.com/hosts.txt", Category: "Malware"})

	lists, err = manager.GetLists()
	if err != nil {
		t.Fatalf("GetLists failed: %v", err)
	}
	if len(lists) != 2 {
		t.Errorf("expected 2 lists, got %d", len(lists))
	}
}

func TestManagerGetListSummaries(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	manager.AddList(DomainList{
		Name:     "Big List",
		Source:   "local",
		Category: "Ads",
		Domains:  []string{"a.com", "b.com", "c.com"},
	})

	summaries, err := manager.GetListSummaries()
	if err != nil {
		t.Fatalf("GetListSummaries failed: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].EntryCount != 3 {
		t.Errorf("summary EntryCount = %d, want 3", summaries[0].EntryCount)
	}
	// Summaries should not expose the Domains slice (it's not in the struct)
}

func TestManagerUpdateList(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	created, _ := manager.AddList(DomainList{
		Name:     "Original",
		Source:   "local",
		Category: "Ads",
		Domains:  []string{"old.com"},
	})

	err := manager.UpdateList(created.ID, DomainList{
		Name:        "Updated",
		Source:      "local",
		Category:    "Malware",
		Description: "Updated description",
		Domains:     []string{"new.com", "new2.com"},
	})
	if err != nil {
		t.Fatalf("UpdateList failed: %v", err)
	}

	fetched, _ := manager.GetList(created.ID)
	if fetched.Name != "Updated" {
		t.Errorf("Name = %q, want %q", fetched.Name, "Updated")
	}
	if fetched.Category != "Malware" {
		t.Errorf("Category = %q, want %q", fetched.Category, "Malware")
	}
	if fetched.CreatedAt != created.CreatedAt {
		t.Error("CreatedAt should be preserved")
	}
	if fetched.EntryCount != 2 {
		t.Errorf("EntryCount = %d, want 2", fetched.EntryCount)
	}

	// Index should be updated
	if manager.Index.IsDomainInList("old.com", created.ID) {
		t.Error("old.com should not be indexed after update")
	}
	if !manager.Index.IsDomainInList("new.com", created.ID) {
		t.Error("new.com should be indexed after update")
	}
}

func TestManagerDeleteList(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	created, _ := manager.AddList(DomainList{
		Name:    "To Delete",
		Source:  "local",
		Domains: []string{"gone.com"},
	})

	err := manager.DeleteList(created.ID)
	if err != nil {
		t.Fatalf("DeleteList failed: %v", err)
	}

	fetched, _ := manager.GetList(created.ID)
	if fetched != nil {
		t.Error("expected nil after deletion")
	}

	lists, _ := manager.GetLists()
	if len(lists) != 0 {
		t.Errorf("expected 0 lists after deletion, got %d", len(lists))
	}

	// Index should be cleaned up
	if manager.Index.IsDomainInList("gone.com", created.ID) {
		t.Error("gone.com should not be indexed after deletion")
	}
}

// ---------- Domain CRUD Tests ----------

func TestManagerAddDomainsToList(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	created, _ := manager.AddList(DomainList{
		Name:    "My List",
		Source:  "local",
		Domains: []string{"existing.com"},
	})

	err := manager.AddDomainsToList(created.ID, []string{"new1.com", "new2.com", "existing.com"}) // existing.com should be deduped
	if err != nil {
		t.Fatalf("AddDomainsToList failed: %v", err)
	}

	fetched, _ := manager.GetList(created.ID)
	if fetched.EntryCount != 3 {
		t.Errorf("EntryCount = %d, want 3 (dedup existing.com)", fetched.EntryCount)
	}

	if !manager.Index.IsDomainInList("new1.com", created.ID) {
		t.Error("new1.com should be indexed")
	}
}

func TestManagerRemoveDomainFromList(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	created, _ := manager.AddList(DomainList{
		Name:    "My List",
		Source:  "local",
		Domains: []string{"keep.com", "remove.com", "also-keep.com"},
	})

	err := manager.RemoveDomainFromList(created.ID, "remove.com")
	if err != nil {
		t.Fatalf("RemoveDomainFromList failed: %v", err)
	}

	fetched, _ := manager.GetList(created.ID)
	if fetched.EntryCount != 2 {
		t.Errorf("EntryCount = %d, want 2", fetched.EntryCount)
	}

	if manager.Index.IsDomainInList("remove.com", created.ID) {
		t.Error("remove.com should not be indexed after removal")
	}
	if !manager.Index.IsDomainInList("keep.com", created.ID) {
		t.Error("keep.com should still be indexed")
	}
}

func TestManagerCannotModifyURLSourcedList(t *testing.T) {
	manager, cleanup := setupTestManager(t)
	defer cleanup()

	created, _ := manager.AddList(DomainList{
		Name:   "URL List",
		Source: "url",
		URL:    "https://example.com/hosts.txt",
	})

	// Should be a no-op (not an error)
	err := manager.AddDomainsToList(created.ID, []string{"cant-add.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = manager.RemoveDomainFromList(created.ID, "cant-remove.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------- Type Tests ----------

func TestToSummary(t *testing.T) {
	dl := DomainList{
		ID:          "abc123",
		Name:        "Test",
		Description: "Desc",
		Category:    "Ads",
		Source:      "url",
		URL:         "https://example.com",
		Domains:     []string{"huge", "list", "of", "domains"},
		EntryCount:  4,
		LastUpdated: "2025-01-01T00:00:00Z",
		CreatedAt:   "2025-01-01T00:00:00Z",
	}

	summary := dl.ToSummary()
	if summary.ID != dl.ID || summary.Name != dl.Name || summary.EntryCount != dl.EntryCount {
		t.Error("summary fields don't match source")
	}
	// Summary type has no Domains field — that's the whole point
}

// ---------- Migration Name Inference Tests ----------

func TestInferNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts", "StevenBlack"},
		{"https://v.firebog.net/hosts/AdguardDNS.txt", "AdguardDNS"},
		{"https://example.com/path/to/blocklist.txt", "blocklist"},
		{"https://example.com/", "example.com"},
	}

	for _, tt := range tests {
		got := inferNameFromURL(tt.url)
		if got != tt.want {
			t.Errorf("inferNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
