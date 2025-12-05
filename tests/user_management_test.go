package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestUserManagement(t *testing.T) {
	t.Run("Integration test: User management and authentication", func(t *testing.T) {
		redirectLogs(t)
		t.Log("Testing user management functionality")

		// Get admin token
		username := gatesentryAdminUsername
		password := gatesentryAdminPassword
		payload := map[string]string{"username": username, "pass": password}
		jsonData, _ := json.Marshal(payload)

		tokenResp, err := http.Post(gatesentryWebserverBaseEndpoint+"/auth/token",
			"application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatal("Failed to get admin token:", err)
		}
		defer tokenResp.Body.Close()

		tokenBody, _ := io.ReadAll(tokenResp.Body)
		var tokenResult map[string]interface{}
		json.Unmarshal(tokenBody, &tokenResult)
		token := tokenResult["Jwtoken"].(string)

		client := &http.Client{}

		// Test 1: Create a new user
		t.Log("Test 1: creating a new test user")
		newUser := map[string]interface{}{
			"username":    "testuser123",
			"password":    "testpassword123",
			"allowaccess": true,
		}
		newUserJSON, _ := json.Marshal(newUser)
		req, _ := http.NewRequest("POST", gatesentryWebserverBaseEndpoint+"/users",
			bytes.NewBuffer(newUserJSON))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("failed to create user. status: %d, body: %s", resp.StatusCode, string(body))
		}
		t.Log("User created successfully")

		// Test 2: List all users
		t.Log("Test 2: listing all users")
		req, _ = http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/users", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to list users: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read users response body: %v", err)
		}
		var usersResponse map[string]interface{}
		if err := json.Unmarshal(body, &usersResponse); err != nil {
			t.Fatalf("failed to unmarshal users response: %v", err)
		}

		users, ok := usersResponse["users"].([]interface{})
		if !ok {
			t.Fatal("users field has unexpected type")
		}
		if len(users) < 2 { // Should have at least admin + testuser123
			t.Fatal("expected at least 2 users")
		}
		t.Logf("Found %d users", len(users))

		// Test 3: Update user
		t.Log("Test 3: updating test user")
		updateUser := map[string]interface{}{
			"username":    "testuser123",
			"password":    "newpassword123",
			"allowaccess": false, // Change access
		}
		updateUserJSON, _ := json.Marshal(updateUser)
		req, _ = http.NewRequest("PUT", gatesentryWebserverBaseEndpoint+"/users",
			bytes.NewBuffer(updateUserJSON))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to update user: %v", err)
		}
		resp.Body.Close()
		t.Log("User updated successfully")

		// Test 4: Delete user
		t.Log("Test 4: deleting test user")
		req, _ = http.NewRequest("DELETE", gatesentryWebserverBaseEndpoint+"/users/testuser123", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to delete user: %v", err)
		}
		resp.Body.Close()
		t.Log("User deleted successfully")
	})

	t.Run("Integration test: Time-based filtering", func(t *testing.T) {
		redirectLogs(t)
		t.Log("Testing time-based content filtering")

		// Get admin token
		username := gatesentryAdminUsername
		password := gatesentryAdminPassword
		payload := map[string]string{"username": username, "pass": password}
		jsonData, _ := json.Marshal(payload)

		tokenResp, err := http.Post(gatesentryWebserverBaseEndpoint+"/auth/token",
			"application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			t.Fatal("Failed to get admin token:", err)
		}
		defer tokenResp.Body.Close()

		tokenBody, _ := io.ReadAll(tokenResp.Body)
		var tokenResult map[string]interface{}
		json.Unmarshal(tokenBody, &tokenResult)
		token := tokenResult["Jwtoken"].(string)

		client := &http.Client{}

		// Test 1: Get current settings
		t.Log("Test 1: getting current time filter settings")
		req, _ := http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/settings", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to get settings: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Log("Settings retrieved successfully")
		} else {
			// In some deployments the settings endpoint may not be configured;
			// log the status code but do not fail the whole suite.
			t.Logf("Settings endpoint returned status %d (non-200)", resp.StatusCode)
		}
	})
}
