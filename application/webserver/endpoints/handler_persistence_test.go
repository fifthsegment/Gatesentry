package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
)

func failingSettingsStore(t *testing.T) *gatesentry2storage.MapStore {
	t.Helper()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(filepath.Join(t.TempDir(), "missing") + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSettingsGetReturnsStorageReadFailure(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("timezone", "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings"), []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := GSApiSettingsGET("timezone", store); err == nil {
		t.Fatal("expected settings GET to return storage read failure")
	}
}

func TestConcurrentUserMutationsDoNotLoseUnrelatedChanges(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}

	seed := make([]gatesentryTypes.GSUser, 20)
	for i := range seed {
		seed[i] = gatesentryTypes.GSUser{User: fmt.Sprintf("seed-%02d", i), AllowAccess: false}
	}
	seedJSON, err := json.Marshal(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("authusers", string(seedJSON)); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := GSApiUserCreate(UserInputJsonSingle{Username: fmt.Sprintf("created-%02d", i), Password: "long-password", AllowAccess: true}, store); err != nil {
				t.Errorf("create: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if i < 10 {
				if _, err := GSApiUserDELETE(fmt.Sprintf("seed-%02d", i), store); err != nil {
					t.Errorf("delete: %v", err)
				}
				return
			}
			if _, err := GSApiUserPUT(store, UserInputJsonSingle{Username: fmt.Sprintf("seed-%02d", i), AllowAccess: true}); err != nil {
				t.Errorf("update: %v", err)
			}
		}()
	}
	wg.Wait()

	var users []gatesentryTypes.GSUser
	if err := json.Unmarshal([]byte(store.GetOrDefault("authusers", "")), &users); err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]gatesentryTypes.GSUser, len(users))
	for _, user := range users {
		byName[user.User] = user
	}
	if len(users) != 30 {
		t.Fatalf("users = %d, want 30", len(users))
	}
	for i := 0; i < 20; i++ {
		if _, ok := byName[fmt.Sprintf("created-%02d", i)]; !ok {
			t.Errorf("created-%02d is missing", i)
		}
	}
	for i := 0; i < 10; i++ {
		if _, ok := byName[fmt.Sprintf("seed-%02d", i)]; ok {
			t.Errorf("seed-%02d was not deleted", i)
		}
		if user, ok := byName[fmt.Sprintf("seed-%02d", i+10)]; !ok || !user.AllowAccess {
			t.Errorf("seed-%02d was not updated", i+10)
		}
	}
}

func TestConcurrentGeneralSettingsUpdatesPreservePasswords(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	initialJSON, err := json.Marshal(gatesentryWebserverTypes.GSGeneral_Settings{
		LogLocation:   "initial.db",
		AdminPassword: "initial-password",
		AdminUser:     "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update("general_settings", string(initialJSON)); err != nil {
		t.Fatal(err)
	}

	const updates = 40
	var wg sync.WaitGroup
	for i := 0; i < updates; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			value, err := json.Marshal(gatesentryWebserverTypes.GSGeneral_Settings{
				LogLocation:   fmt.Sprintf("explicit-%02d.db", i),
				AdminPassword: fmt.Sprintf("password-%02d", i),
				AdminUser:     "admin",
			})
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := GSApiSettingsPOST("general_settings", store, gatesentryWebserverTypes.Datareceiver{Value: string(value)}); err != nil {
				t.Errorf("explicit password update: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			value, err := json.Marshal(gatesentryWebserverTypes.GSGeneral_Settings{
				LogLocation: fmt.Sprintf("preserve-%02d.db", i),
				AdminUser:   "admin",
			})
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := GSApiSettingsPOST("general_settings", store, gatesentryWebserverTypes.Datareceiver{Value: string(value)}); err != nil {
				t.Errorf("password-preserving update: %v", err)
			}
		}()
	}
	wg.Wait()

	var got gatesentryWebserverTypes.GSGeneral_Settings
	if err := json.Unmarshal([]byte(store.GetOrDefault("general_settings", "")), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.AdminPassword, "password-") {
		t.Fatalf("password-preserving update restored stale password %q", got.AdminPassword)
	}
}

func TestMutationHelpersReturnPersistenceFailures(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		store := failingSettingsStore(t)
		output, err := GSApiUserCreate(UserInputJsonSingle{Username: "user", Password: "long-password", AllowAccess: true}, store)
		if err == nil {
			t.Fatalf("output = %#v; expected persistence error", output)
		}
	})

	t.Run("consumption", func(t *testing.T) {
		store := failingSettingsStore(t)
		mutated := false
		runtime := gatesentryWebserverTypes.NewTemporaryRuntime(gatesentryWebserverTypes.InputArgs{
			RemoveUser: func(gatesentryTypes.GSUser) { mutated = true },
			UpdateUser: func(string, gatesentryTypes.GSUserPublic) { mutated = true },
		})
		output, err := GSApiConsumptionPOST(Datareceiver{EnableUsers: true}, store, runtime)
		if err == nil {
			t.Fatalf("output = %#v; expected persistence error", output)
		}
		if mutated {
			t.Fatal("runtime users changed after persistence failure")
		}
	})

	t.Run("dns", func(t *testing.T) {
		store := failingSettingsStore(t)
		output, err := GSApiDNSSaveEntriesCustom([]gatesentryTypes.DNSCustomEntry{{IP: "192.0.2.1", Domain: "example.test"}}, store, nil)
		if err == nil {
			t.Fatalf("output = %#v; expected persistence error", output)
		}
	})
}
