package policy

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestCategoryCatalogIsWellFormed(t *testing.T) {
	catalog := CategoryCatalog()
	if len(catalog) == 0 {
		t.Fatal("category catalog is empty")
	}
	seen := make(map[string]bool, len(catalog))
	for _, category := range catalog {
		if category.ID == "" || category.Name == "" || category.Description == "" {
			t.Fatalf("category lacks id, name, or description: %+v", category)
		}
		if seen[category.ID] {
			t.Fatalf("duplicate category id %q", category.ID)
		}
		seen[category.ID] = true
		if len(category.Sources) == 0 {
			t.Fatalf("category %s has no upstream source", category.ID)
		}
		for _, source := range category.Sources {
			if !strings.HasPrefix(source, "https://") {
				t.Fatalf("category %s source %q is not https", category.ID, source)
			}
		}
	}

	// The catalog is process-wide release data, so callers must not be able to
	// mutate it through a returned slice.
	catalog[0].Sources[0] = "https://mutated.example/list.txt"
	if again := CategoryCatalog(); again[0].Sources[0] == "https://mutated.example/list.txt" {
		t.Fatal("CategoryCatalog returned a shared source slice")
	}

	if _, ok := GetCategory("social"); !ok {
		t.Fatal("social category missing from catalog")
	}
	if _, ok := GetCategory("no-such-category"); ok {
		t.Fatal("unknown category resolved")
	}
}

func TestNormalizeCategoriesTrimsDedupesAndRejectsUnknown(t *testing.T) {
	got, err := NormalizeCategories([]string{" Social ", "", "social", "ADS"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "social" || got[1] != "ads" {
		t.Fatalf("normalized = %v, want [social ads]", got)
	}

	if got, err := NormalizeCategories(nil); err != nil || got != nil {
		t.Fatalf("empty selection = %v, %v; want nil, nil", got, err)
	}

	if _, err := NormalizeCategories([]string{"social", "typo-category"}); !errors.Is(err, ErrUnknownCategory) {
		t.Fatalf("unknown id error = %v, want ErrUnknownCategory", err)
	}
}

func TestCategoryIndexCoversParentDomains(t *testing.T) {
	// A nil index is the state before the first refresh: nothing may match.
	var unloaded *CategoryIndex
	if unloaded.Contains("social", "facebook.com") {
		t.Fatal("nil index matched a domain")
	}
	if unloaded.Count("social") != 0 {
		t.Fatal("nil index reported coverage")
	}
	if !unloaded.UpdatedAt("social").IsZero() {
		t.Fatal("nil index reported a refresh time")
	}

	index := NewCategoryIndex()
	refreshed := time.Unix(1700000000, 0).UTC()
	index.Replace("social", []string{"*.Facebook.com", "facebook.com.", "instagram.com", ""}, refreshed)

	for _, domain := range []string{"facebook.com", "www.facebook.com", "m.facebook.com", "instagram.com"} {
		if !index.Contains("social", domain) {
			t.Fatalf("category did not cover %s", domain)
		}
	}
	for _, domain := range []string{"notfacebook.com", "facebook.com.evil.example", "example.com"} {
		if index.Contains("social", domain) {
			t.Fatalf("category wrongly covered %s", domain)
		}
	}
	if got := index.Count("social"); got != 2 {
		t.Fatalf("coverage count = %d, want 2", got)
	}
	if got := index.UpdatedAt("social"); !got.Equal(refreshed) {
		t.Fatalf("updated at = %s, want %s", got, refreshed)
	}

	// A refresh replaces the set; a domain dropped upstream must stop matching.
	index.Replace("social", []string{"instagram.com"}, time.Now())
	if index.Contains("social", "facebook.com") {
		t.Fatal("stale domain survived a refresh")
	}
	if got := index.Count("social"); got != 1 {
		t.Fatalf("coverage count after refresh = %d, want 1", got)
	}
}

// categoryService wires a service whose group assignment is the only thing
// deciding the outcome, so category matching is what the assertions measure.
func categoryService(t testing.TB, group PolicyGroup, categories map[string][]string) (*Service, Identity) {
	t.Helper()
	svc := newTestService(t)
	index := NewCategoryIndex()
	for id, domains := range categories {
		index.Replace(id, domains, time.Now())
	}
	svc.SetCategoryIndex(index)
	if err := svc.SaveGroups([]PolicyGroup{group}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: group.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}
	return svc, svc.ResolveIdentity("192.0.2.10", "")
}

func TestGroupCategoryRuleBlocksAssignedDevice(t *testing.T) {
	svc, identity := categoryService(t,
		PolicyGroup{ID: "kids", Name: "Kids", BlockedCategories: []string{"social"}},
		map[string][]string{"social": {"facebook.com", "tiktok.com"}},
	)

	decision := svc.EvaluateDNS(identity, "www.facebook.com")
	if decision.Action != ActionBlock {
		t.Fatalf("action = %s, want block", decision.Action)
	}
	if decision.GroupID != "kids" {
		t.Fatalf("group = %s, want kids", decision.GroupID)
	}
	if decision.MatchedDomain != "category:social" {
		t.Fatalf("matched = %q, want category:social", decision.MatchedDomain)
	}
	if !strings.Contains(decision.Reason, "social") {
		t.Fatalf("reason = %q, want the category named", decision.Reason)
	}

	// A category rule only covers its own domains; everything else keeps the
	// gateway default.
	if got := svc.EvaluateDNS(identity, "example.com").Action; got != ActionNone {
		t.Fatalf("unrelated domain action = %s, want none", got)
	}
}

func TestGroupCategoryAllowExemptsAssignedDevice(t *testing.T) {
	svc, identity := categoryService(t,
		PolicyGroup{ID: "work", Name: "Work", Rules: []GroupRule{{
			ID: "social", Enabled: true, Action: ActionAllow, Target: RuleTarget{Categories: []string{"social"}},
		}}},
		map[string][]string{"social": {"facebook.com"}},
	)

	if got := svc.EvaluateDNS(identity, "facebook.com").Action; got != ActionAllow {
		t.Fatalf("action = %s, want allow", got)
	}
	if got := svc.EvaluateDNS(identity, "example.com").Action; got != ActionNone {
		t.Fatalf("unrelated domain action = %s, want none", got)
	}
}

func TestGroupCategoryRuleNeedsAnAssignment(t *testing.T) {
	svc := newTestService(t)
	index := NewCategoryIndex()
	index.Replace("social", []string{"facebook.com"}, time.Now())
	svc.SetCategoryIndex(index)
	if err := svc.SaveGroups([]PolicyGroup{{
		ID: "kids", Name: "Kids", BlockedCategories: []string{"social"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	svc.devices = &mapResolver{devices: map[string]string{"192.0.2.10": "device-1"}}

	// An unassigned device is on the default policy, not the kids policy.
	identity := svc.ResolveIdentity("192.0.2.10", "")
	if identity.GroupID != DefaultGroupID {
		t.Fatalf("unassigned identity group = %q, want the default policy", identity.GroupID)
	}
	if got := svc.EvaluateDNS(identity, "www.facebook.com").Action; got != ActionNone {
		t.Fatalf("action = %s, want none without an assignment", got)
	}
}

func TestExplicitDomainPatternIsNamedBeforeCategory(t *testing.T) {
	svc, identity := categoryService(t,
		PolicyGroup{
			ID: "kids", Name: "Kids", BlockedDomains: []string{"facebook.com"}, BlockedCategories: []string{"social"},
		},
		map[string][]string{"social": {"facebook.com", "tiktok.com"}},
	)

	if got := svc.EvaluateDNS(identity, "facebook.com").MatchedDomain; got != "facebook.com" {
		t.Fatalf("matched = %q, want the explicit pattern", got)
	}
	// The category still covers domains the operator never listed.
	if got := svc.EvaluateDNS(identity, "www.tiktok.com").MatchedDomain; got != "category:social" {
		t.Fatalf("matched = %q, want category:social", got)
	}
}

func TestCategoryRuleMatchesNothingWithoutDownloadedFeeds(t *testing.T) {
	// The index is attached but never filled, which is what a failed first
	// refresh looks like. A category rule must then express no opinion instead
	// of blocking or allowing by accident.
	svc, identity := categoryService(t,
		PolicyGroup{ID: "kids", Name: "Kids", BlockedCategories: []string{"social"}},
		nil,
	)

	if got := svc.EvaluateDNS(identity, "www.facebook.com").Action; got != ActionNone {
		t.Fatalf("action = %s, want none with no category data", got)
	}
}

func TestEnabledCategoriesPersistAndReportCoverage(t *testing.T) {
	svc, _ := templateTestService(t)
	if got := svc.EnabledCategories(); len(got) != 0 {
		t.Fatalf("initial enabled categories = %v, want none", got)
	}

	if err := svc.SetEnabledCategories([]string{" Ads ", "social"}); err != nil {
		t.Fatal(err)
	}
	got := svc.EnabledCategories()
	if len(got) != 2 || got[0] != "ads" || got[1] != "social" {
		t.Fatalf("enabled categories = %v, want [ads social]", got)
	}

	if err := svc.SetEnabledCategories([]string{"no-such-category"}); !errors.Is(err, ErrUnknownCategory) {
		t.Fatalf("unknown category error = %v, want ErrUnknownCategory", err)
	}
	if got := svc.EnabledCategories(); len(got) != 2 {
		t.Fatalf("a rejected write changed the stored selection: %v", got)
	}

	index := NewCategoryIndex()
	index.Replace("ads", []string{"ads.example"}, time.Now())
	svc.SetCategoryIndex(index)
	statuses := svc.CategoryStatuses()
	if len(statuses) != len(CategoryCatalog()) {
		t.Fatalf("statuses = %d, want the full catalog", len(statuses))
	}
	for _, status := range statuses {
		switch status.ID {
		case "ads":
			if status.DomainCount != 1 || !status.Enabled || status.UpdatedAt == "" {
				t.Fatalf("ads status = %+v", status)
			}
		case "social":
			if !status.Enabled {
				t.Fatalf("social should be enabled: %+v", status)
			}
		default:
			if status.Enabled {
				t.Fatalf("category %s reported enabled without being selected", status.ID)
			}
		}
	}

	if err := svc.SetEnabledCategories(nil); err != nil {
		t.Fatal(err)
	}
	if got := svc.EnabledCategories(); len(got) != 0 {
		t.Fatalf("cleared enabled categories = %v, want none", got)
	}
}

func TestTemplatesOnlyReferenceCatalogCategories(t *testing.T) {
	for _, template := range PolicyTemplates() {
		if _, err := NormalizeCategories(template.Categories); err != nil {
			t.Fatalf("template %s references %v: %v", template.ID, template.Categories, err)
		}
		group := template.Group("UTC")
		if len(group.BlockedCategories) != len(template.Categories) {
			t.Fatalf("template %s group categories = %v, want %v", template.ID, group.BlockedCategories, template.Categories)
		}
		for _, rule := range group.Rules {
			if _, err := NormalizeCategories(rule.Target.Categories); err != nil {
				t.Fatalf("template %s rule references %v: %v", template.ID, rule.Target.Categories, err)
			}
		}
	}

	// The two profiles that are meant to protect a child or teen must actually
	// block something, and must name the categories they block.
	for _, id := range []string{"child", "teen"} {
		template, ok := GetPolicyTemplate(id)
		if !ok {
			t.Fatalf("template %s missing", id)
		}
		if len(template.Categories) == 0 {
			t.Fatalf("template %s blocks no category", id)
		}
		if !template.SafeSearch {
			t.Fatalf("template %s does not force safe search", id)
		}
	}

	child, _ := GetPolicyTemplate("child")
	for _, want := range []string{"adult", "gambling", "malware", "piracy"} {
		found := false
		for _, id := range child.Categories {
			if id == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("child template missing category %s: %v", want, child.Categories)
		}
	}

	// Templates are catalog data; a caller editing the returned copy must not
	// change what the next caller sees.
	child.Categories[0] = "mutated"
	if again, _ := GetPolicyTemplate("child"); again.Categories[0] == "mutated" {
		t.Fatal("GetPolicyTemplate returned a shared category slice")
	}
}

func TestCategoryIndexDropReleasesUnusedFeeds(t *testing.T) {
	index := NewCategoryIndex()
	index.Replace("malware", []string{"malware.example"}, time.Now())
	if !index.Contains("malware", "malware.example") {
		t.Fatal("a replaced category did not match its own domain")
	}

	index.Drop("malware")
	if index.Contains("malware", "malware.example") {
		t.Fatal("a dropped category still matched")
	}
	if index.Count("malware") != 0 || !index.UpdatedAt("malware").IsZero() {
		t.Fatal("a dropped category still reported coverage")
	}

	// The refresh runs before startup too, so dropping has to tolerate the
	// unloaded index a failed start leaves behind.
	var unloaded *CategoryIndex
	unloaded.Drop("malware")
}

func TestReferencedCategoriesReportsEveryGroupSelection(t *testing.T) {
	svc, _ := templateTestService(t)
	if got := svc.ReferencedCategories(); len(got) != 0 {
		t.Fatalf("referenced categories = %v, want none without groups", got)
	}

	groups := []PolicyGroup{
		{ID: "kids", Name: "Kids", BlockedCategories: []string{"social", "adult"}},
		{ID: "work", Name: "Work", Rules: []GroupRule{{ID: "r", Enabled: true, Action: ActionAllow, Target: RuleTarget{Categories: []string{"social"}}}}},
		{ID: "plain", Name: "Plain"},
	}
	if err := svc.SaveGroups(groups); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	// The refresh downloads exactly these feeds, so duplicates across groups
	// and groups without categories must not appear in the answer.
	got := svc.ReferencedCategories()
	if len(got) != 2 || got[0] != "adult" || got[1] != "social" {
		t.Fatalf("referenced categories = %v, want [adult social]", got)
	}
}

func TestEnabledCategoryMatchCoversSubdomains(t *testing.T) {
	svc, _ := templateTestService(t)
	index := NewCategoryIndex()
	// Feeds list registrable domains, so the subdomain coverage has to come
	// from the match, not from the feed contents.
	index.Replace("social", []string{"facebook.com"}, time.Now())
	svc.SetCategoryIndex(index)
	if err := svc.SetEnabledCategories([]string{"social"}); err != nil {
		t.Fatal(err)
	}

	for _, domain := range []string{"facebook.com", "www.facebook.com", "m.facebook.com"} {
		id, matched := svc.EnabledCategoryMatch(domain)
		if !matched || id != "social" {
			t.Fatalf("EnabledCategoryMatch(%q) = %q, %v; want social, true", domain, id, matched)
		}
	}
	if id, matched := svc.EnabledCategoryMatch("example.com"); matched {
		t.Fatalf("EnabledCategoryMatch(example.com) = %q, want no match", id)
	}
}

func TestEnabledCategoryMatchFollowsTheStoredSelection(t *testing.T) {
	svc, _ := templateTestService(t)
	index := NewCategoryIndex()
	index.Replace("social", []string{"facebook.com"}, time.Now())
	svc.SetCategoryIndex(index)

	// A category that is not selected gateway-wide must not block anything, no
	// matter how much data the index holds for it.
	if id, matched := svc.EnabledCategoryMatch("facebook.com"); matched {
		t.Fatalf("unselected category matched: %q", id)
	}
	if err := svc.SetEnabledCategories([]string{"social"}); err != nil {
		t.Fatal(err)
	}
	if _, matched := svc.EnabledCategoryMatch("facebook.com"); !matched {
		t.Fatal("selected category stopped matching")
	}
	// Clearing the selection takes effect without a restart, because the DNS
	// adapter reads the cached selection this write refreshes.
	if err := svc.SetEnabledCategories(nil); err != nil {
		t.Fatal(err)
	}
	if id, matched := svc.EnabledCategoryMatch("facebook.com"); matched {
		t.Fatalf("cleared category still matched: %q", id)
	}
}

func TestGroupCategoryBlocksOnTheProxy(t *testing.T) {
	svc, identity := categoryService(t,
		PolicyGroup{ID: "kids", Name: "Kids", BlockedCategories: []string{"social"}},
		map[string][]string{"social": {"facebook.com", "tiktok.com"}},
	)

	for _, host := range []string{"facebook.com", "www.facebook.com", "m.tiktok.com"} {
		match := svc.EvaluateProxy(identity, host)
		if !match.Matched || !match.ShouldBlock || match.MatchedDomain != "category:social" {
			t.Fatalf("%s: match = %+v, want a block by category:social", host, match)
		}
		// A plain domain block needs no inspection: the proxy refuses the
		// CONNECT before any TLS, so clients without the CA are covered too.
		if match.ShouldMITM {
			t.Fatalf("%s: match = %+v, want no inspection for a domain block", host, match)
		}
	}
	if match := svc.EvaluateProxy(identity, "example.com"); match.Matched {
		t.Fatalf("unrelated domain match = %+v, want no decision", match)
	}
	// Other devices stay on the default policy.
	other := svc.ResolveIdentity("192.0.2.99", "")
	if match := svc.EvaluateProxy(other, "www.facebook.com"); match.Matched {
		t.Fatalf("unassigned device match = %+v, want no decision", match)
	}
}

// BenchmarkProxyCategoryLookup measures a proxy decision against category
// feeds the size of the real ones. The lookup is one hash probe per domain
// label, so its cost must not grow with the number of domains in a feed.
func BenchmarkProxyCategoryLookup(b *testing.B) {
	for _, size := range []int{1_000, 1_000_000} {
		b.Run(fmt.Sprintf("domains=%d", size), func(b *testing.B) {
			domains := make([]string, size)
			for i := range domains {
				domains[i] = fmt.Sprintf("site%d.example", i)
			}
			svc, identity := categoryService(b,
				PolicyGroup{ID: "kids", Name: "Kids", BlockedCategories: []string{"ads", "social", "adult"}},
				map[string][]string{"ads": domains[:size/2], "social": {"facebook.com"}, "adult": domains[size/2:]},
			)
			hosts := []string{"cdn.www.site7.example", "www.facebook.com", "unrelated.host.example.org"}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				svc.EvaluateProxy(identity, hosts[i%len(hosts)])
			}
		})
	}
}
