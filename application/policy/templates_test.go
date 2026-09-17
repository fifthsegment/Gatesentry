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
	wantIDs := []string{"child", "teen", "adult-default", "guest", "work", "iot", "unrestricted"}
	if len(templates) != len(wantIDs) {
		t.Fatalf("templates = %d, want %d", len(templates), len(wantIDs))
	}
	for i, wantID := range wantIDs {
		if templates[i].ID != wantID || !templates[i].Available {
			t.Fatalf("template %d = %+v, want available %s", i, templates[i], wantID)
		}
		if len(templates[i].Protections) == 0 || len(templates[i].Limitations) == 0 {
			t.Fatalf("template %s lacks preview protections or limitations", templates[i].ID)
		}
	}
	if templates[0].Group().ID != "template-child" || templates[0].Group().Name != "Child" {
		t.Fatalf("child group = %+v", templates[0].Group())
	}

	templates[0].Protections[0] = "mutated"
	again := PolicyTemplates()
	if again[0].Protections[0] == "mutated" {
		t.Fatal("catalog returned shared protection slice")
	}
}

func TestTemplateGroupLifecyclePreservesEditsAndAssignments(t *testing.T) {
	service, _ := templateTestService(t)
	template, ok := GetPolicyTemplate("child")
	if !ok {
		t.Fatal("child template not found")
	}
	group := template.Group()
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
	group.Domains = []string{"*.games.example"}
	group.Action = ActionBlock
	if err := service.UpdateGroup(group.ID, group); err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	stored := service.Snapshot().Groups[group.ID]
	if stored.Description != group.Description || stored.Action != ActionBlock || len(stored.Domains) != 1 {
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
	if err := service.UpdateGroup("one", PolicyGroup{Name: "Edited One", Action: ActionAllow}); err != nil {
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
