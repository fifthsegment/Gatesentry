package gatesentryf_test

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	application "bitbucket.org/abdullah_irfan/gatesentryf"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	endpoints "bitbucket.org/abdullah_irfan/gatesentryf/webserver/endpoints"
)

func TestUserDataSaverDoesNotOverwriteConcurrentDurableMutations(t *testing.T) {
	dir := t.TempDir()
	oldBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(oldBaseDir) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	seedJSON, _ := json.Marshal([]GatesentryTypes.GSUser{{User: "seed", DataConsumed: 1}})
	if err := store.Update("authusers", string(seedJSON)); err != nil {
		t.Fatal(err)
	}
	runtime := &application.GSRuntime{GSSettings: store, AuthUsers: []GatesentryTypes.GSUser{{User: "seed", DataConsumed: 99}}}

	const additions = 30
	var wg sync.WaitGroup
	for i := 0; i < additions; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("api-%02d", i)
			if _, err := endpoints.GSApiUserCreate(endpoints.UserInputJsonSingle{Username: name, Password: "long-password", AllowAccess: true}, store); err != nil {
				t.Errorf("API mutation: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			runtime.GSUserDataSaver()
		}()
	}
	wg.Wait()

	current, err := store.GetE("authusers")
	if err != nil {
		t.Fatal(err)
	}
	var users []GatesentryTypes.GSUser
	if err := json.Unmarshal([]byte(current), &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != additions+1 {
		t.Fatalf("durable users = %d, want %d", len(users), additions+1)
	}
	if users[0].DataConsumed != 99 {
		t.Fatalf("merged consumption = %d, want 99", users[0].DataConsumed)
	}
}
