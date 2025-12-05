package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestAuthAndFilters(t *testing.T) {
	t.Run("Test if webserver login works with the default user", func(t *testing.T) {
		username := gatesentryAdminUsername
		password := gatesentryAdminPassword

		payload := map[string]string{"username": username, "pass": password}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			t.Fatal("Failed to marshal JSON for sending:", err)
		}

		resp, err := http.Post(gatesentryWebserverBaseEndpoint+"/auth/token", "application/json", bytes.NewBuffer(jsonData))

		if err != nil {
			t.Fatal("Failed to get token:", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal("Failed to read body:", err)
		}

		// Extract token from response
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal("Failed to unmarshal response:", err)
		}
		token, ok := result["Jwtoken"].(string)
		if !ok {
			t.Fatal("Token not found in response")
		}

		// Make GET request to /filters using the token
		req, err := http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/filters", nil)
		if err != nil {
			t.Fatal("Failed to create request:", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err = client.Do(req)
		time.Sleep(2 * time.Second)
		if err != nil {
			t.Fatal("Failed to get filters:", err)
		}
		defer resp.Body.Close()

		// Check for 200 status code
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		jsonDataString := `
		[{"Content":"google","Score":10000}]
		`

		req, err = http.NewRequest("POST", gatesentryWebserverBaseEndpoint+"/filters/bVxTPTOXiqGRbhF", bytes.NewBuffer([]byte(jsonDataString)))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if err != nil {
			t.Fatal("Failed to create request:", err)
		}

		// get response body
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal("Failed to post filters:", err)
		}

		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal("Failed to read body:", err)
		}

		t.Logf("Response body after post = %s", string(body))
		t.Log("Waiting for the server to reload")

		// time.Sleep(4 * time.Second)

		for _, filter := range R.Filters {
			t.Logf("Filter name = %s", filter.FilterName)
			for _, line := range filter.FileContents {
				t.Logf("Line = %s", line.Content)
			}
		}

		req, err = http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/filters/bVxTPTOXiqGRbhF", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		if err != nil {
			t.Fatal("Failed to create request:", err)
		}

		// get response body
		resp, err = client.Do(req)
		time.Sleep(2 * time.Second)
		if err != nil {
			t.Fatal("Failed to get filters:", err)
		}

		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal("Failed to read body:", err)
		}

		t.Logf("Response body = %s", string(body))

	})
}
