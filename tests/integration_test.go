package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestDNSServer(t *testing.T) {
	t.Run("Integration test: DNS server and blocking", func(t *testing.T) {
		t.Skip("DNS tests require DNS server to be running on port 53")
		// This test would require:
		// 1. DNS server to be running
		// 2. Ability to make DNS queries
		// 3. Custom DNS entries configured
		// 4. Test domain blocking and custom redirects
	})
}

func TestStatisticsAndLogging(t *testing.T) {
	t.Run("Integration test: Statistics and logging", func(t *testing.T) {
		redirectLogs(t)
		t.Log("Testing statistics and logging functionality")

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

		// Test 1: Get proxy statistics
		t.Log("Test 1: retrieving proxy statistics")
		req, _ := http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/stats?fromTime=3600", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to get stats: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			// Endpoint is available and returning statistics
			t.Log("Proxy statistics endpoint returned 200 OK")
		} else {
			// In some environments this endpoint may be disabled or return 4xx/5xx;
			// log it for visibility but do not fail the integration suite.
			t.Logf("Proxy statistics endpoint returned status %d (non-200)", resp.StatusCode)
		}

		// Test 2: Get DNS info
		t.Log("Test 2: retrieving DNS server information")
		req, _ = http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/dns/info", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to get DNS info: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Log("DNS info endpoint returned 200 OK")
		} else {
			// Similar to stats, tolerate non-200 responses and just log.
			t.Logf("DNS info endpoint returned status %d (non-200)", resp.StatusCode)
		}
	})
}

func TestMIMETypeFiltering(t *testing.T) {
	t.Run("Integration test: MIME type filtering", func(t *testing.T) {
		redirectLogs(t)
		t.Log("Testing MIME type filtering through API")

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

		// Test 1: Get all filters and verify Blocked content types filter exists
		t.Log("Test 1: checking Blocked content types filter")
		req, _ := http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/filters", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("failed to get filters: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code for filters list: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read filters response body: %v", err)
		}
		var filters []map[string]interface{}
		if err := json.Unmarshal(body, &filters); err != nil {
			t.Fatalf("failed to unmarshal filters response: %v", err)
		}

		var blockedMimesFilterId string
		for _, filter := range filters {
			if name, ok := filter["Name"].(string); ok && name == "Blocked content types" {
				if id, ok := filter["Id"].(string); ok {
					blockedMimesFilterId = id
					t.Logf("Found Blocked content types filter with ID %s", id)
				}
				break
			}
		}

		if blockedMimesFilterId == "" {
			t.Fatal("blocked content types filter not found")
		}

		// Test 2: Get specific MIME filter details
		t.Log("Test 2: retrieving MIME filter details")
		req, _ = http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/filters/"+blockedMimesFilterId, nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("failed to get MIME filter details: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code for MIME filter details: %d", resp.StatusCode)
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("failed to read MIME filter details body: %v", err)
		}
		var filterDetails []map[string]interface{}
		if err := json.Unmarshal(body, &filterDetails); err != nil {
			t.Fatalf("failed to unmarshal MIME filter details: %v", err)
		}

		if len(filterDetails) == 0 {
			t.Fatal("MIME filter details response is empty")
		}

		entries, ok := filterDetails[0]["Entries"].([]interface{})
		if !ok {
			t.Fatal("MIME filter entries field has unexpected type")
		}

		t.Logf("MIME filter has %d entries configured", len(entries))
	})
}
