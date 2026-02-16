package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsBlockedAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"blocked_url", true},
		{"blocked_text_content", true},
		{"blocked_media_content", true},
		{"blocked_file_type", true},
		{"blocked_time", true},
		{"blocked_internet_for_user", true},
		{"ssldirect", false},
		{"ssl-bump", false},
		{"filternone", false},
		{"filtererror", false},
		{"", false},
	}
	for _, tc := range tests {
		got := isBlockedAction(tc.action)
		if got != tc.want {
			t.Errorf("isBlockedAction(%q) = %v, want %v", tc.action, got, tc.want)
		}
	}
}

func TestIsAllowedAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"ssldirect", true},
		{"ssl-bump", true},
		{"filternone", true},
		{"blocked_url", false},
		{"filtererror", false},
		{"", false},
	}
	for _, tc := range tests {
		got := isAllowedAction(tc.action)
		if got != tc.want {
			t.Errorf("isAllowedAction(%q) = %v, want %v", tc.action, got, tc.want)
		}
	}
}

func TestActionLabel(t *testing.T) {
	// All known actions should return a human-friendly string (not the raw constant)
	known := []string{
		"blocked_text_content", "blocked_media_content", "blocked_file_type",
		"blocked_time", "blocked_internet_for_user", "blocked_url",
		"ssldirect", "ssl-bump", "filternone", "filtererror",
	}
	for _, action := range known {
		label := actionLabel(action)
		if label == "" {
			t.Errorf("actionLabel(%q) returned empty string", action)
		}
		// The label should NOT be the raw action string for known types
		// (except filtererror which falls through to default)
		if action != "filtererror" && label == action {
			t.Errorf("actionLabel(%q) returned raw action string instead of a label", action)
		}
	}

	// Unknown actions fall through to the raw string
	if got := actionLabel("something_unknown"); got != "something_unknown" {
		t.Errorf("actionLabel(unknown) = %q, want %q", got, "something_unknown")
	}
}

func TestBuildTopSites(t *testing.T) {
	counts := map[string]int{
		"example.com": 10,
		"test.com":    50,
		"foo.com":     30,
		"bar.com":     5,
		"baz.com":     20,
	}

	// Top 3 should be test.com(50), foo.com(30), baz.com(20)
	top3 := buildTopSites(counts, 3)
	if len(top3) != 3 {
		t.Fatalf("expected 3 sites, got %d", len(top3))
	}
	if top3[0].Host != "test.com" || top3[0].Count != 50 {
		t.Errorf("top[0] = %v, want test.com:50", top3[0])
	}
	if top3[1].Host != "foo.com" || top3[1].Count != 30 {
		t.Errorf("top[1] = %v, want foo.com:30", top3[1])
	}
	if top3[2].Host != "baz.com" || top3[2].Count != 20 {
		t.Errorf("top[2] = %v, want baz.com:20", top3[2])
	}

	// Empty map returns empty slice
	empty := buildTopSites(map[string]int{}, 5)
	if len(empty) != 0 {
		t.Errorf("expected empty slice, got %d items", len(empty))
	}

	// N larger than available
	allSites := buildTopSites(counts, 100)
	if len(allSites) != 5 {
		t.Errorf("expected 5 sites (all), got %d", len(allSites))
	}
}

func TestApiGetProxyStats_EmptyLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logger := setupTestLogger(tmpDir)

	req := httptest.NewRequest(http.MethodGet, "/api/stats/proxy?seconds=3600&group=minute", nil)
	w := httptest.NewRecorder()

	ApiGetProxyStats(w, req, logger)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp ProxyStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Summary.TotalRequests != 0 {
		t.Errorf("expected 0 total requests, got %d", resp.Summary.TotalRequests)
	}
	if resp.Summary.Allowed != 0 {
		t.Errorf("expected 0 allowed, got %d", resp.Summary.Allowed)
	}
	if resp.Summary.Blocked != 0 {
		t.Errorf("expected 0 blocked, got %d", resp.Summary.Blocked)
	}
}

func TestApiGetProxyStats_WithProxyEntries(t *testing.T) {
	tmpDir := t.TempDir()
	logger := setupTestLogger(tmpDir)

	// Insert some proxy log entries directly into BuntDB
	insertProxyLogEntry(t, logger, "example.com:443", "user1", "ssl-bump")
	insertProxyLogEntry(t, logger, "ads.tracker.com:443", "user1", "blocked_url")
	insertProxyLogEntry(t, logger, "google.com:443", "user2", "ssldirect")
	insertProxyLogEntry(t, logger, "malware.com:443", "user2", "blocked_text_content")
	insertProxyLogEntry(t, logger, "ads.tracker.com:443", "user1", "blocked_url")

	req := httptest.NewRequest(http.MethodGet, "/api/stats/proxy?seconds=3600&group=minute", nil)
	w := httptest.NewRecorder()

	ApiGetProxyStats(w, req, logger)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp ProxyStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Summary.TotalRequests != 5 {
		t.Errorf("expected 5 total requests, got %d", resp.Summary.TotalRequests)
	}
	if resp.Summary.Allowed != 2 {
		t.Errorf("expected 2 allowed, got %d", resp.Summary.Allowed)
	}
	if resp.Summary.Blocked != 3 {
		t.Errorf("expected 3 blocked, got %d", resp.Summary.Blocked)
	}
	if resp.Summary.SSLBumped != 1 {
		t.Errorf("expected 1 ssl_bumped, got %d", resp.Summary.SSLBumped)
	}
	if resp.Summary.SSLDirect != 1 {
		t.Errorf("expected 1 ssl_direct, got %d", resp.Summary.SSLDirect)
	}

	// Top blocked should have ads.tracker.com at position 0 (count=2)
	// Note: the logger strips ":443" from proxy URLs in GetLastXSecondsDNSLogs
	if len(resp.TopBlocked) < 1 {
		t.Fatal("expected at least 1 top blocked site")
	}
	if resp.TopBlocked[0].Host != "ads.tracker.com" {
		t.Errorf("top blocked = %q, want ads.tracker.com", resp.TopBlocked[0].Host)
	}
	if resp.TopBlocked[0].Count != 2 {
		t.Errorf("top blocked count = %d, want 2", resp.TopBlocked[0].Count)
	}

	// Users list should have user1 and user2
	if len(resp.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(resp.Users))
	}
}

func TestApiGetProxyStats_UserFilter(t *testing.T) {
	tmpDir := t.TempDir()
	logger := setupTestLogger(tmpDir)

	insertProxyLogEntry(t, logger, "example.com:443", "user1", "ssl-bump")
	insertProxyLogEntry(t, logger, "ads.com:443", "user1", "blocked_url")
	insertProxyLogEntry(t, logger, "google.com:443", "user2", "ssldirect")

	// Filter by user1 only
	req := httptest.NewRequest(http.MethodGet, "/api/stats/proxy?seconds=3600&group=minute&user=user1", nil)
	w := httptest.NewRecorder()

	ApiGetProxyStats(w, req, logger)

	var resp ProxyStatsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Summary.TotalRequests != 2 {
		t.Errorf("expected 2 total requests for user1, got %d", resp.Summary.TotalRequests)
	}
	if len(resp.Users) != 1 {
		t.Errorf("expected 1 user in filtered results, got %d", len(resp.Users))
	}
}
