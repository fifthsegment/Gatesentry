package gatesentryWebserverEndpoints

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

// setupUserTestStore creates a temp MapStore and returns it plus a cleanup func.
func setupUserTestStore(t *testing.T) (*gatesentry2storage.MapStore, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "usertest-*")
	if err != nil {
		t.Fatal(err)
	}

	origBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(tmpDir + "/")

	store := gatesentry2storage.NewMapStore("test_users_settings", false)

	return store, func() {
		gatesentry2storage.SetBaseDir(origBaseDir)
		os.RemoveAll(tmpDir)
	}
}

// decodeUsersFromStore reads the "authusers" key and returns the user slice.
func decodeUsersFromStore(t *testing.T, store *gatesentry2storage.MapStore) []GatesentryTypes.GSUser {
	t.Helper()
	raw := store.Get("authusers")
	if raw == "" {
		return nil
	}
	var users []GatesentryTypes.GSUser
	if err := json.Unmarshal([]byte(raw), &users); err != nil {
		t.Fatalf("unmarshal authusers: %v", err)
	}
	return users
}

// assertOk checks that the response is UserEndpointJsonOk{Ok: true}.
func assertOk(t *testing.T, result interface{}, label string) {
	t.Helper()
	resp, ok := result.(UserEndpointJsonOk)
	if !ok {
		t.Fatalf("[%s] expected UserEndpointJsonOk, got %T: %+v", label, result, result)
	}
	if !resp.Ok {
		t.Fatalf("[%s] expected Ok=true, got false", label)
	}
}

// assertError checks the response is UserEndpointJsonError with expected substring.
func assertError(t *testing.T, result interface{}, substr string, label string) {
	t.Helper()
	resp, ok := result.(UserEndpointJsonError)
	if !ok {
		t.Fatalf("[%s] expected UserEndpointJsonError, got %T: %+v", label, result, result)
	}
	if resp.Ok {
		t.Fatalf("[%s] expected Ok=false, got true", label)
	}
	if substr != "" && resp.Error == "" {
		t.Fatalf("[%s] expected error containing %q, got empty", label, substr)
	}
	if substr != "" && !containsSubstring(resp.Error, substr) {
		t.Fatalf("[%s] expected error containing %q, got %q", label, substr, resp.Error)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (sub == "" || findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Validation tests ---

func TestValidateUserInputJsonSingle(t *testing.T) {
	tests := []struct {
		name string
		user UserInputJsonSingle
		want bool
	}{
		{"valid", UserInputJsonSingle{Username: "testuser", Password: "longenough1"}, true},
		{"short_username", UserInputJsonSingle{Username: "ab", Password: "longenough1"}, false},
		{"short_password", UserInputJsonSingle{Username: "testuser", Password: "short"}, false},
		{"both_short", UserInputJsonSingle{Username: "ab", Password: "short"}, false},
		{"exact_min_username", UserInputJsonSingle{Username: "abc", Password: "1234567890"}, true},
		{"exact_min_password", UserInputJsonSingle{Username: "abc", Password: "1234567890"}, true},
		{"empty_both", UserInputJsonSingle{Username: "", Password: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateUserInputJsonSingle(tt.user)
			if got != tt.want {
				t.Errorf("ValidateUserInputJsonSingle(%+v) = %v, want %v", tt.user, got, tt.want)
			}
		})
	}
}

// --- Create user tests ---

func TestGSApiUserCreate_Success(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	result := GSApiUserCreate(UserInputJsonSingle{
		Username:    "testuser1",
		Password:    "mypassword123",
		AllowAccess: true,
	}, store)

	assertOk(t, result, "create user")

	// Verify the user was persisted
	users := decodeUsersFromStore(t, store)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].User != "testuser1" {
		t.Errorf("expected username 'testuser1', got %q", users[0].User)
	}
	if !users[0].AllowAccess {
		t.Error("expected AllowAccess=true")
	}
	// Verify base64 encoding is correct (lowercase username)
	expected := base64.StdEncoding.EncodeToString([]byte("testuser1:mypassword123"))
	if users[0].Base64String != expected {
		t.Errorf("base64 mismatch: got %q, want %q", users[0].Base64String, expected)
	}
}

func TestGSApiUserCreate_UsernameNormalized(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	result := GSApiUserCreate(UserInputJsonSingle{
		Username:    "TestUser",
		Password:    "mypassword123",
		AllowAccess: true,
	}, store)

	assertOk(t, result, "create mixed-case")

	users := decodeUsersFromStore(t, store)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	// Username should be stored lowercase
	if users[0].User != "testuser" {
		t.Errorf("expected lowercase 'testuser', got %q", users[0].User)
	}
	// Base64 should use the lowercase username
	expected := base64.StdEncoding.EncodeToString([]byte("testuser:mypassword123"))
	if users[0].Base64String != expected {
		t.Errorf("base64 should use lowercase username: got %q, want %q", users[0].Base64String, expected)
	}
}

func TestGSApiUserCreate_DuplicateRejected(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// Create first user
	GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "mypassword123", AllowAccess: true,
	}, store)

	// Try to create same user again
	result := GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "mypassword123", AllowAccess: true,
	}, store)

	assertError(t, result, "already exists", "duplicate user")
}

func TestGSApiUserCreate_DuplicateCaseInsensitive(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// Create lowercase
	GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "mypassword123", AllowAccess: true,
	}, store)

	// Try mixed case — should still be rejected
	result := GSApiUserCreate(UserInputJsonSingle{
		Username: "TestUser1", Password: "mypassword123", AllowAccess: true,
	}, store)

	assertError(t, result, "already exists", "case-insensitive duplicate")
}

func TestGSApiUserCreate_ValidationRejectsShortPassword(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	result := GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "short", AllowAccess: true,
	}, store)

	assertError(t, result, "too short", "short password")
}

func TestGSApiUserCreate_ValidationRejectsShortUsername(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	result := GSApiUserCreate(UserInputJsonSingle{
		Username: "ab", Password: "mypassword123", AllowAccess: true,
	}, store)

	assertError(t, result, "too short", "short username")
}

// --- Update user (PUT) tests ---

func TestGSApiUserPUT_ChangePassword(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// Create user
	GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "oldpassword1", AllowAccess: true,
	}, store)

	// Get the original base64
	usersBefore := decodeUsersFromStore(t, store)
	origBase64 := usersBefore[0].Base64String

	// Update password
	result := GSApiUserPUT(store, UserInputJsonSingle{
		Username: "testuser1", Password: "newpassword1", AllowAccess: true,
	})

	assertOk(t, result, "update password")

	// Verify the base64 changed
	usersAfter := decodeUsersFromStore(t, store)
	if usersAfter[0].Base64String == origBase64 {
		t.Error("base64 did not change after password update")
	}
	expected := base64.StdEncoding.EncodeToString([]byte("testuser1:newpassword1"))
	if usersAfter[0].Base64String != expected {
		t.Errorf("base64 mismatch: got %q, want %q", usersAfter[0].Base64String, expected)
	}
}

func TestGSApiUserPUT_ChangePasswordCaseInsensitive(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// Create user (stored as lowercase)
	GSApiUserCreate(UserInputJsonSingle{
		Username: "TestUser1", Password: "oldpassword1", AllowAccess: true,
	}, store)

	// Update with mixed case username — should still find the user
	result := GSApiUserPUT(store, UserInputJsonSingle{
		Username: "TESTUSER1", Password: "newpassword1", AllowAccess: true,
	})

	assertOk(t, result, "case-insensitive update")

	// Verify the base64 uses lowercase username
	users := decodeUsersFromStore(t, store)
	expected := base64.StdEncoding.EncodeToString([]byte("testuser1:newpassword1"))
	if users[0].Base64String != expected {
		t.Errorf("base64 should use lowercase: got %q, want %q", users[0].Base64String, expected)
	}
}

func TestGSApiUserPUT_ToggleAccessNoPasswordChange(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// Create user with access
	GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "oldpassword1", AllowAccess: true,
	}, store)

	origBase64 := decodeUsersFromStore(t, store)[0].Base64String

	// Toggle access, empty password → should not change base64
	result := GSApiUserPUT(store, UserInputJsonSingle{
		Username: "testuser1", Password: "", AllowAccess: false,
	})

	assertOk(t, result, "toggle access")

	users := decodeUsersFromStore(t, store)
	if users[0].AllowAccess {
		t.Error("expected AllowAccess=false after toggle")
	}
	if users[0].Base64String != origBase64 {
		t.Error("base64 should not change when password is empty")
	}
}

func TestGSApiUserPUT_RejectsShortPassword(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "oldpassword1", AllowAccess: true,
	}, store)

	result := GSApiUserPUT(store, UserInputJsonSingle{
		Username: "testuser1", Password: "short", AllowAccess: true,
	})

	assertError(t, result, "too short", "short password on update")

	// Verify original password was NOT changed
	users := decodeUsersFromStore(t, store)
	expected := base64.StdEncoding.EncodeToString([]byte("testuser1:oldpassword1"))
	if users[0].Base64String != expected {
		t.Error("password should not have changed after validation failure")
	}
}

func TestGSApiUserPUT_UserNotFound(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// Create one user
	GSApiUserCreate(UserInputJsonSingle{
		Username: "testuser1", Password: "mypassword123", AllowAccess: true,
	}, store)

	// Try to update a non-existent user
	result := GSApiUserPUT(store, UserInputJsonSingle{
		Username: "nonexistent", Password: "newpassword1", AllowAccess: true,
	})

	assertError(t, result, "not found", "user not found")
}

// --- Delete user tests ---

func TestGSApiUserDELETE_Success(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// Create two users
	GSApiUserCreate(UserInputJsonSingle{
		Username: "user1", Password: "mypassword123", AllowAccess: true,
	}, store)
	GSApiUserCreate(UserInputJsonSingle{
		Username: "user2", Password: "mypassword456", AllowAccess: true,
	}, store)

	// Delete user1
	result := GSApiUserDELETE("user1", store)
	assertOk(t, result, "delete user")

	// Verify only user2 remains
	users := decodeUsersFromStore(t, store)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].User != "user2" {
		t.Errorf("expected remaining user 'user2', got %q", users[0].User)
	}
}

func TestGSApiUserDELETE_CaseInsensitive(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	GSApiUserCreate(UserInputJsonSingle{
		Username: "TestUser", Password: "mypassword123", AllowAccess: true,
	}, store)

	// Delete with different case — should still work
	result := GSApiUserDELETE("TESTUSER", store)
	assertOk(t, result, "case-insensitive delete")

	users := decodeUsersFromStore(t, store)
	if len(users) != 0 {
		t.Errorf("expected 0 users after delete, got %d", len(users))
	}
}

func TestGSApiUserDELETE_NotFound(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	result := GSApiUserDELETE("nonexistent", store)
	assertError(t, result, "not found", "delete nonexistent user")
}

// --- JSON response format tests ---
// These verify that the error/success responses serialize with correct JSON keys
// so the frontend can read response.ok and response.error (lowercase).

func TestErrorResponseJsonFormat(t *testing.T) {
	errResp := UserEndpointJsonError{Ok: false, Error: "something broke"}
	data, err := json.Marshal(errResp)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	// Verify lowercase keys (matching json tags)
	if _, ok := m["ok"]; !ok {
		t.Error("expected lowercase 'ok' key in JSON")
	}
	if _, ok := m["error"]; !ok {
		t.Error("expected lowercase 'error' key in JSON")
	}
	// Verify no uppercase keys
	if _, ok := m["Ok"]; ok {
		t.Error("unexpected uppercase 'Ok' key — missing json tag?")
	}
	if _, ok := m["Error"]; ok {
		t.Error("unexpected uppercase 'Error' key — missing json tag?")
	}
}

func TestOkResponseJsonFormat(t *testing.T) {
	okResp := UserEndpointJsonOk{Ok: true}
	data, err := json.Marshal(okResp)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	if _, ok := m["ok"]; !ok {
		t.Error("expected lowercase 'ok' key in JSON")
	}
	if val, ok := m["ok"].(bool); !ok || !val {
		t.Errorf("expected ok=true, got %v", m["ok"])
	}
}

// --- Full CRUD lifecycle test ---

func TestUserCRUD_FullLifecycle(t *testing.T) {
	store, cleanup := setupUserTestStore(t)
	defer cleanup()

	// 1. Create user
	createResult := GSApiUserCreate(UserInputJsonSingle{
		Username: "lifecycle", Password: "initialpass1", AllowAccess: true,
	}, store)
	assertOk(t, createResult, "lifecycle create")

	// 2. Verify created
	users := decodeUsersFromStore(t, store)
	if len(users) != 1 || users[0].User != "lifecycle" {
		t.Fatalf("user not created correctly: %+v", users)
	}
	initialBase64 := users[0].Base64String

	// 3. Change password
	putResult := GSApiUserPUT(store, UserInputJsonSingle{
		Username: "lifecycle", Password: "changedpass1", AllowAccess: true,
	})
	assertOk(t, putResult, "lifecycle password change")

	// 4. Verify password changed (base64 is different)
	users = decodeUsersFromStore(t, store)
	if users[0].Base64String == initialBase64 {
		t.Fatal("password change did not update base64")
	}
	expectedBase64 := base64.StdEncoding.EncodeToString([]byte("lifecycle:changedpass1"))
	if users[0].Base64String != expectedBase64 {
		t.Fatalf("base64 mismatch after password change: got %q want %q",
			users[0].Base64String, expectedBase64)
	}

	// 5. Toggle access (no password change)
	toggleResult := GSApiUserPUT(store, UserInputJsonSingle{
		Username: "lifecycle", Password: "", AllowAccess: false,
	})
	assertOk(t, toggleResult, "lifecycle toggle access")

	users = decodeUsersFromStore(t, store)
	if users[0].AllowAccess {
		t.Error("AllowAccess should be false after toggle")
	}
	if users[0].Base64String != expectedBase64 {
		t.Error("base64 should not change when toggling access without password")
	}

	// 6. Delete user
	deleteResult := GSApiUserDELETE("lifecycle", store)
	assertOk(t, deleteResult, "lifecycle delete")

	users = decodeUsersFromStore(t, store)
	if len(users) != 0 {
		t.Errorf("expected 0 users after delete, got %d", len(users))
	}
}
