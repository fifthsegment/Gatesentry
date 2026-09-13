package gatesentryf

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

func TestConcurrentRuleMutationsDoNotLoseUpdates(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("rules", false)
	if err != nil {
		t.Fatal(err)
	}
	rm := NewRuleManager(store)

	const count = 30
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := rm.AddRule(GatesentryTypes.Rule{ID: fmt.Sprintf("rule-%d", i), Domain: fmt.Sprintf("%d.example", i)})
			if err != nil {
				t.Errorf("add: %v", err)
			}
		}(i)
	}
	wg.Wait()
	rules, err := rm.GetRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != count {
		t.Fatalf("rules = %d, want %d", len(rules), count)
	}

	wg = sync.WaitGroup{}
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("rule-%d", i)
			if i%2 == 0 {
				if err := rm.DeleteRule(id); err != nil {
					t.Errorf("delete: %v", err)
				}
				return
			}
			if err := rm.UpdateRule(id, GatesentryTypes.Rule{Domain: fmt.Sprintf("updated-%d.example", i)}); err != nil {
				t.Errorf("update: %v", err)
			}
		}(i)
	}
	wg.Wait()
	rules, err = rm.GetRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != count/2 {
		t.Fatalf("rules after mutations = %d, want %d", len(rules), count/2)
	}
	for _, rule := range rules {
		if !strings.HasPrefix(rule.Domain, "updated-") {
			t.Errorf("rule %s was not updated", rule.ID)
		}
	}
}
