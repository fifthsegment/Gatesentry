package gatesentryf

import (
	"encoding/json"
	"os"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

// setupUsersTestRuntime creates a minimal GSRuntime with a temp MapStore for testing.
func setupUsersTestRuntime(t *testing.T) (*GSRuntime, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "userstest-*")
	if err != nil {
		t.Fatal(err)
	}

	origBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(tmpDir + "/")

	store := gatesentry2storage.NewMapStore("test_settings", false)

	runtime := &GSRuntime{
		GSSettings: store,
	}

	cleanup := func() {
		gatesentry2storage.SetBaseDir(origBaseDir)
		os.RemoveAll(tmpDir)
	}
	return runtime, cleanup
}

// seedUsers persists users to the store so LoadUsers can find them.
func seedUsers(t *testing.T, store *gatesentry2storage.MapStore, users []GatesentryTypes.GSUser) {
	t.Helper()
	b, err := json.Marshal(users)
	if err != nil {
		t.Fatalf("seedUsers marshal: %v", err)
	}
	store.Update("authusers", string(b))
}

// --- UpdateUserData tests ---

func TestUpdateUserData_IncrementsBytes(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	rt.AuthUsers = []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 0},
		{User: "bob", DataConsumed: 100},
	}

	rt.UpdateUserData("alice", 500)
	rt.UpdateUserData("alice", 300)
	rt.UpdateUserData("bob", 200)

	if rt.AuthUsers[0].DataConsumed != 800 {
		t.Errorf("alice: expected 800, got %d", rt.AuthUsers[0].DataConsumed)
	}
	if rt.AuthUsers[1].DataConsumed != 300 {
		t.Errorf("bob: expected 300, got %d", rt.AuthUsers[1].DataConsumed)
	}
}

func TestUpdateUserData_UnknownUserNoOp(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	rt.AuthUsers = []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 100},
	}

	// Updating a nonexistent user should not panic or change anything
	rt.UpdateUserData("ghost", 9999)
	rt.UpdateUserData("", 9999)

	if rt.AuthUsers[0].DataConsumed != 100 {
		t.Errorf("alice should be unchanged at 100, got %d", rt.AuthUsers[0].DataConsumed)
	}
}

func TestUpdateUserData_EmptyUsersList(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	rt.AuthUsers = nil

	// Should not panic
	rt.UpdateUserData("alice", 100)
}

// --- LoadUsers tests ---

func TestLoadUsers_Basic(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	seedUsers(t, rt.GSSettings, []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 50, AllowAccess: true},
		{User: "bob", DataConsumed: 0, AllowAccess: false},
	})

	rt.LoadUsers()

	if len(rt.AuthUsers) != 2 {
		t.Fatalf("expected 2 users, got %d", len(rt.AuthUsers))
	}
	if rt.AuthUsers[0].User != "alice" || rt.AuthUsers[0].DataConsumed != 50 {
		t.Errorf("alice: unexpected data %+v", rt.AuthUsers[0])
	}
}

func TestLoadUsers_PreservesInMemoryDataConsumed(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	// Seed store with DataConsumed=100 for alice
	seedUsers(t, rt.GSSettings, []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 100, AllowAccess: true},
		{User: "bob", DataConsumed: 50, AllowAccess: true},
	})

	// Load initial state
	rt.LoadUsers()

	// Simulate proxy traffic accumulating bytes in memory
	rt.UpdateUserData("alice", 900) // alice now 1000 in memory
	rt.UpdateUserData("bob", 450)   // bob now 500 in memory

	// Simulate a user CRUD that writes to the store (store still has 100/50)
	// Then reload happens — this is the bug scenario
	rt.LoadUsers()

	// After reload, in-memory values (1000/500) should be preserved since
	// they're higher than the stale store values (100/50)
	if rt.AuthUsers[0].DataConsumed != 1000 {
		t.Errorf("alice: expected 1000 (preserved from memory), got %d", rt.AuthUsers[0].DataConsumed)
	}
	if rt.AuthUsers[1].DataConsumed != 500 {
		t.Errorf("bob: expected 500 (preserved from memory), got %d", rt.AuthUsers[1].DataConsumed)
	}
}

func TestLoadUsers_NewUserFromStoreGetsZeroBytes(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	// Start with alice only in memory
	rt.AuthUsers = []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 500},
	}

	// Store now has both alice (stale bytes) and a new user bob
	seedUsers(t, rt.GSSettings, []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 100},
		{User: "bob", DataConsumed: 0, AllowAccess: true},
	})

	rt.LoadUsers()

	if len(rt.AuthUsers) != 2 {
		t.Fatalf("expected 2 users, got %d", len(rt.AuthUsers))
	}
	// alice's in-memory bytes (500) should be preserved
	if rt.AuthUsers[0].DataConsumed != 500 {
		t.Errorf("alice: expected 500, got %d", rt.AuthUsers[0].DataConsumed)
	}
	// bob is new — should have 0 bytes
	if rt.AuthUsers[1].DataConsumed != 0 {
		t.Errorf("bob: expected 0, got %d", rt.AuthUsers[1].DataConsumed)
	}
}

func TestLoadUsers_DeletedUserBytesNotCarriedOver(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	// Memory has alice with 1000 bytes
	rt.AuthUsers = []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 1000},
		{User: "bob", DataConsumed: 500},
	}

	// Store only has bob (alice was deleted via API)
	seedUsers(t, rt.GSSettings, []GatesentryTypes.GSUser{
		{User: "bob", DataConsumed: 200},
	})

	rt.LoadUsers()

	if len(rt.AuthUsers) != 1 {
		t.Fatalf("expected 1 user, got %d", len(rt.AuthUsers))
	}
	if rt.AuthUsers[0].User != "bob" {
		t.Errorf("expected bob, got %s", rt.AuthUsers[0].User)
	}
	// bob's in-memory bytes (500) > store bytes (200), should be preserved
	if rt.AuthUsers[0].DataConsumed != 500 {
		t.Errorf("bob: expected 500, got %d", rt.AuthUsers[0].DataConsumed)
	}
}

// --- GSUserDataSaver / round-trip tests ---

func TestDataSaver_PersistsDataConsumed(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	rt.AuthUsers = []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 12345},
		{User: "bob", DataConsumed: 67890},
	}

	rt.GSUserDataSaver()

	// Reload from store
	rt.AuthUsers = nil
	rt.LoadUsers()

	if len(rt.AuthUsers) != 2 {
		t.Fatalf("expected 2 users after reload, got %d", len(rt.AuthUsers))
	}
	if rt.AuthUsers[0].DataConsumed != 12345 {
		t.Errorf("alice: expected 12345, got %d", rt.AuthUsers[0].DataConsumed)
	}
	if rt.AuthUsers[1].DataConsumed != 67890 {
		t.Errorf("bob: expected 67890, got %d", rt.AuthUsers[1].DataConsumed)
	}
}

func TestDataSaver_FullPipeline(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	// Seed initial users
	seedUsers(t, rt.GSSettings, []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 0, AllowAccess: true},
	})
	rt.LoadUsers()

	// Simulate proxy traffic
	rt.UpdateUserData("alice", 1024)
	rt.UpdateUserData("alice", 2048)

	if rt.AuthUsers[0].DataConsumed != 3072 {
		t.Fatalf("in-memory: expected 3072, got %d", rt.AuthUsers[0].DataConsumed)
	}

	// Simulate a settings change triggering reload (without data saver running)
	rt.LoadUsers()

	// In-memory bytes should survive the reload
	if rt.AuthUsers[0].DataConsumed != 3072 {
		t.Errorf("after reload: expected 3072, got %d (bytes were clobbered!)", rt.AuthUsers[0].DataConsumed)
	}

	// Now persist
	rt.GSUserDataSaver()

	// Clear memory and reload — should get persisted value
	rt.AuthUsers = nil
	rt.LoadUsers()

	if rt.AuthUsers[0].DataConsumed != 3072 {
		t.Errorf("after save+reload: expected 3072, got %d", rt.AuthUsers[0].DataConsumed)
	}
}

// --- GSApiUsersGET (via handler) ---

func TestGSUserGetDataJSON_ReturnsLiveData(t *testing.T) {
	rt, cleanup := setupUsersTestRuntime(t)
	defer cleanup()

	rt.AuthUsers = []GatesentryTypes.GSUser{
		{User: "alice", DataConsumed: 999},
	}

	data := rt.GSUserGetDataJSON()
	if data == nil {
		t.Fatal("GSUserGetDataJSON returned nil")
	}

	var users []GatesentryTypes.GSUserPublic
	if err := json.Unmarshal(data, &users); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(users) != 1 || users[0].DataConsumed != 999 {
		t.Errorf("expected DataConsumed=999, got %+v", users)
	}
}
