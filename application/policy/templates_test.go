package policy

import (
	"errors"
	"os"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func templateTestService(t *testing.T) (*Service, *gatesentry2storage.MapStore) {
	t.Helper()
	oldBase := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(t.TempDir() + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(oldBase) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, store
}

func TestPolicyTemplatesDescribeSupportedBehavior(t *testing.T) {
	templates := PolicyTemplates()
	wantIDs := []string{"child", "teen", "guest", "work", "iot"}
	if len(templates) != len(wantIDs) {
		t.Fatalf("templates = %d, want %d", len(templates), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if templates[i].ID != wantID || !templates[i].Available {
			t.Fatalf("template %d = %+v, want available %s", i, templates[i], wantID)
		}
		if len(templates[i].Limitations) == 0 {
			t.Fatalf("template %s lacks a limitation", templates[i].ID)
		}
	}
	child := templates[0].Group("Europe/Oslo")
	if child.ID != "template-child" || child.Name != "Young child" || !child.SafeSearch {
		t.Fatalf("child group = %+v", child)
	}
	if len(child.Rules) != 1 || child.Rules[0].Schedule == nil || child.Rules[0].Schedule.Timezone != "Europe/Oslo" {
		t.Fatalf("child bedtime rule = %+v, want a schedule in the gateway's zone", child.Rules)
	}
	if _, err := NormalizeGroup(child); err != nil {
		t.Fatalf("child starter is not a valid policy: %v", err)
	}
	for _, template := range templates {
		if _, err := NormalizeGroup(template.Group("UTC")); err != nil {
			t.Fatalf("template %s is not a valid policy: %v", template.ID, err)
		}
	}
	if templates[0].Description == "" || templates[1].Description == "" {
		t.Fatal("template is missing a description")
	}

	// The caveats that hold for every starter are stated once for the catalog,
	// so no template repeats one of them.
	shared := TemplateSharedLimitations()
	if len(shared) == 0 {
		t.Fatal("no shared template limitations")
	}
	for _, template := range templates {
		for _, limitation := range template.Limitations {
			for _, sharedLimitation := range shared {
				if limitation == sharedLimitation {
					t.Fatalf("template %s repeats the shared caveat %q", template.ID, limitation)
				}
			}
		}
	}
	shared[0] = "mutated"
	if TemplateSharedLimitations()[0] == "mutated" {
		t.Fatal("shared limitations returned the package slice")
	}

	templates[0].Limitations[0] = "mutated"
	again := PolicyTemplates()
	if again[0].Limitations[0] == "mutated" {
		t.Fatal("catalog returned shared limitation slice")
	}
}

func TestTemplateGroupLifecyclePreservesEditsAndAssignments(t *testing.T) {
	service, _ := templateTestService(t)
	template, ok := GetPolicyTemplate("child")
	if !ok {
		t.Fatal("child template not found")
	}
	group := template.Group("UTC")
	if err := service.CreateGroup(group); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateGroup(group); !errors.Is(err, ErrGroupExists) {
		t.Fatalf("reapply error = %v, want ErrGroupExists", err)
	}

	group.Description = "Customized child policy"
	group.BlockedDomains = []string{"games.example"}
	if err := service.UpdateGroup(group.ID, group); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	stored := service.Snapshot().Groups[group.ID]
	if stored.Description != group.Description || len(stored.BlockedDomains) != 1 {
		t.Fatalf("stored customized group = %+v", stored)
	}

	if err := service.SaveAssignments([]DeviceAssignment{{DeviceID: "device-1", GroupID: group.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	snapshot := service.Snapshot()
	if _, exists := snapshot.Groups[group.ID]; exists {
		t.Fatal("deleted group remained in snapshot")
	}
	if _, exists := snapshot.Assignments["device-1"]; exists {
		t.Fatal("deleting group left an assignment to the deleted group")
	}
}

func TestPolicyGroupUpdateDoesNotOverwriteOtherGroups(t *testing.T) {
	service, _ := templateTestService(t)
	if err := service.CreateGroup(PolicyGroup{ID: "one", Name: "One"}); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateGroup(PolicyGroup{ID: "two", Name: "Two"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateGroup("one", PolicyGroup{Name: "Edited One", AllowedDomains: []string{"x.example"}}); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	snapshot := service.Snapshot()
	if snapshot.Groups["one"].Name != "Edited One" || snapshot.Groups["two"].Name != "Two" {
		t.Fatalf("groups after one-group edit = %+v", snapshot.Groups)
	}
}
