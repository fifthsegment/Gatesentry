package tests

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func TestProxyFiltering(t *testing.T) {
	t.Run("Integration test: Proxy filtering functionality", func(t *testing.T) {
		redirectLogs(t)
		t.Log("Testing proxy filtering functionality")

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

		// Test 1: Verify URL blocking filters exist
		t.Log("Test 1: checking URL blocking filters")
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

		var blockedURLsFilterId string
		var exceptionURLsFilterId string

		for _, filter := range filters {
			if name, ok := filter["Name"].(string); ok {
				if name == "Blocked URLs" {
					if id, ok := filter["Id"].(string); ok {
						blockedURLsFilterId = id
						t.Logf("Found Blocked URLs filter with ID %s", id)
					}
				} else if name == "Exception URLs" {
					if id, ok := filter["Id"].(string); ok {
						exceptionURLsFilterId = id
						t.Logf("Found Exception URLs filter with ID %s", id)
					}
				}
			}
		}

		if blockedURLsFilterId == "" {
			t.Log("Warning: Blocked URLs filter not found")
		}
		if exceptionURLsFilterId == "" {
			t.Log("Warning: Exception URLs filter not found")
		}

		// Test 2: Get blocked URLs filter details
		if blockedURLsFilterId != "" {
			t.Log("Test 2: retrieving blocked URLs filter details")
			req, _ = http.NewRequest("GET", gatesentryWebserverBaseEndpoint+"/filters/"+blockedURLsFilterId, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err = client.Do(req)
			if err != nil {
				t.Fatalf("failed to get blocked URLs filter details: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("unexpected status code for blocked URLs filter details: %d", resp.StatusCode)
			}

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read blocked URLs filter details body: %v", err)
			}
			var filterDetails []map[string]interface{}
			if err := json.Unmarshal(body, &filterDetails); err != nil {
				t.Fatalf("failed to unmarshal blocked URLs filter details: %v", err)
			}

			if len(filterDetails) == 0 {
				t.Fatal("blocked URLs filter details response is empty")
			}

			entries, ok := filterDetails[0]["Entries"].([]interface{})
			if !ok {
				t.Fatal("blocked URLs filter entries field has unexpected type")
			}

			t.Logf("Blocked URLs filter has %d entries configured", len(entries))
		}

		// Test 3: Test proxy basic functionality
		t.Log("Test 3: testing proxy basic functionality")
		proxyURLParsed, err := url.Parse(proxyURL)
		if err != nil {
			t.Fatalf("failed to parse proxy URL %q: %v", proxyURL, err)
		}
		proxyClient := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyURL(proxyURLParsed),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // Required for testing
			},
			Timeout: defaultTimeout,
		}

		// Test HTTP request through proxy
		resp, err = proxyClient.Get("http://www.example.com")
		if err != nil {
			t.Logf("Warning: could not access HTTP site through proxy: %v", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusForbidden {
				t.Log("HTTP proxy is functional")
			}
		}

		// Test HTTPS request through proxy
		resp, err = proxyClient.Get("https://www.example.com")
		if err != nil {
			t.Logf("Warning: could not access HTTPS site through proxy: %v", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusForbidden {
				t.Log("HTTPS proxy is functional")
			}
		}

		// Test 4: Test blocked site (if configured)
		if httpBlockedSite != "" {
			t.Log("Test 4: testing blocked site filtering")
			resp, err = proxyClient.Get(httpBlockedSite)
			if err != nil {
				t.Logf("Note: blocked site test returned error (may be expected): %v", err)
			} else {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusForbidden {
					t.Log("Blocked site is properly filtered")
				} else {
					t.Logf("Note: blocked site returned status %d (filtering may not be configured)", resp.StatusCode)
				}
			}
		}
	})
}
