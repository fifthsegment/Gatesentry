package gatesentryWebserverEndpoints

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"bitbucket.org/abdullah_irfan/gatesentryf/domainlist"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

// --- Mock DomainListManager for testing domain list lookups ---

type mockDomainListManager struct {
	index *domainlist.DomainListIndex
}

func newMockDomainListManager(domains map[string][]string) *mockDomainListManager {
	idx := domainlist.NewDomainListIndex()
	for listID, doms := range domains {
		idx.AddDomains(listID, doms)
	}
	return &mockDomainListManager{index: idx}
}

func (m *mockDomainListManager) GetLists() ([]domainlist.DomainList, error) {
	return nil, nil
}
func (m *mockDomainListManager) GetListSummaries() ([]domainlist.DomainListSummary, error) {
	return nil, nil
}
func (m *mockDomainListManager) GetList(id string) (*domainlist.DomainList, error) {
	return nil, nil
}
func (m *mockDomainListManager) AddList(dl domainlist.DomainList) (domainlist.DomainList, error) {
	return dl, nil
}
func (m *mockDomainListManager) UpdateList(id string, updated domainlist.DomainList) error {
	return nil
}
func (m *mockDomainListManager) DeleteList(id string) error  { return nil }
func (m *mockDomainListManager) RefreshList(id string) error { return nil }
func (m *mockDomainListManager) AddDomainsToList(id string, domains []string) error {
	return nil
}
func (m *mockDomainListManager) RemoveDomainFromList(id string, domain string) error {
	return nil
}
func (m *mockDomainListManager) GetDomainsForList(id string) ([]string, error) {
	return nil, nil
}
func (m *mockDomainListManager) IsDomainInList(domain string, listID string) bool {
	return m.index.IsDomainInList(domain, listID)
}
func (m *mockDomainListManager) IsDomainInAnyList(domain string, listIDs []string) (bool, string) {
	for _, id := range listIDs {
		if m.index.IsDomainInList(domain, id) {
			return true, id
		}
	}
	return false, ""
}

// setupTestRuleEndpoint creates a temporary MapStore for test settings and
// initializes the test endpoints. Returns a cleanup function.
func setupTestRuleEndpoint(t *testing.T) func() {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "testrule-*")
	if err != nil {
		t.Fatal(err)
	}

	origBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(tmpDir + "/")

	store := gatesentry2storage.NewMapStore("test_settings", false)
	store.Update("enable_https_filtering", "true")

	// Initialize test endpoints with the store but no filters (keyword tests
	// that need live filters are handled in TestLiveFetch).
	InitTestEndpoints(store, nil)

	return func() {
		gatesentry2storage.SetBaseDir(origBaseDir)
		os.RemoveAll(tmpDir)
	}
}

// callRuleMatch is a helper that POSTs a TestRuleRequest to the handler and
// returns the decoded TestRuleResponse.
func callRuleMatch(t *testing.T, req TestRuleRequest) TestRuleResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/api/test/rule-match", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	GSApiTestRuleMatch(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp TestRuleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// findStep returns the TestStep with the given step number, or nil.
func findStep(steps []TestStep, stepNum int) *TestStep {
	for i := range steps {
		if steps[i].Step == stepNum {
			return &steps[i]
		}
	}
	return nil
}

// ---------- Test 1: Basic block rule match ----------
// Rule with *.example.com domain pattern, action=block
// URL: https://www.example.com/path/page
// Expected: all 8 steps evaluated, outcome="block"

func TestRuleMatch_BasicBlock(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:        true,
			Action:         GatesentryTypes.RuleActionBlock,
			MITMAction:     GatesentryTypes.MITMActionEnable,
			DomainPatterns: []string{"*.example.com"},
		},
		URL: "https://www.example.com/path/page",
	})

	if resp.Outcome != "block" {
		t.Errorf("expected outcome 'block', got %q", resp.Outcome)
	}
	if len(resp.Steps) != 8 {
		t.Errorf("expected 8 steps, got %d", len(resp.Steps))
	}

	// Steps 1-7 should pass, step 8 should be block
	for _, stepNum := range []int{1, 2, 3, 4, 5, 6, 7} {
		s := findStep(resp.Steps, stepNum)
		if s == nil {
			t.Errorf("step %d missing", stepNum)
			continue
		}
		if s.Result != "pass" {
			t.Errorf("step %d: expected result 'pass', got %q (%s)", stepNum, s.Result, s.Detail)
		}
	}

	step8 := findStep(resp.Steps, 8)
	if step8 == nil {
		t.Fatal("step 8 missing")
	}
	if step8.Result != "block" {
		t.Errorf("step 8: expected result 'block', got %q", step8.Result)
	}
}

// ---------- Test 2: Domain mismatch ----------
// Rule with *.google.com domain pattern
// URL: https://www.example.com
// Expected: outcome="skip", fails at step 3

func TestRuleMatch_DomainMismatch(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:        true,
			Action:         GatesentryTypes.RuleActionBlock,
			MITMAction:     GatesentryTypes.MITMActionEnable,
			DomainPatterns: []string{"*.google.com"},
		},
		URL: "https://www.example.com",
	})

	if resp.Outcome != "skip" {
		t.Errorf("expected outcome 'skip', got %q", resp.Outcome)
	}

	// Step 3 should be "fail" (domain mismatch)
	step3 := findStep(resp.Steps, 3)
	if step3 == nil {
		t.Fatal("step 3 missing")
	}
	if step3.Result != "fail" {
		t.Errorf("step 3: expected 'fail', got %q (%s)", step3.Result, step3.Detail)
	}

	// Steps beyond 3 should not be present (early exit)
	if findStep(resp.Steps, 4) != nil {
		t.Error("step 4 should not be present after domain mismatch")
	}
}

// ---------- Test 3: Disabled rule ----------
// Rule with enabled=false
// Expected: outcome="skip", step 1 result="skip"

func TestRuleMatch_DisabledRule(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:        false,
			Action:         GatesentryTypes.RuleActionBlock,
			DomainPatterns: []string{"*.example.com"},
		},
		URL: "https://www.example.com",
	})

	if resp.Outcome != "skip" {
		t.Errorf("expected outcome 'skip', got %q", resp.Outcome)
	}

	step1 := findStep(resp.Steps, 1)
	if step1 == nil {
		t.Fatal("step 1 missing")
	}
	if step1.Result != "skip" {
		t.Errorf("step 1: expected 'skip', got %q (%s)", step1.Result, step1.Detail)
	}

	// Only step 1 should be present (immediate exit)
	if len(resp.Steps) != 1 {
		t.Errorf("expected 1 step (early exit), got %d", len(resp.Steps))
	}
}

// ---------- Test 4: URL regex mismatch ----------
// Rule with domain *, url_regex_patterns=[".*\.pdf$"], mitm_action="enable"
// URL: https://example.com/page.html
// Expected: outcome="skip", step 5 result="fail"

func TestRuleMatch_URLRegexMismatch(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:          true,
			Action:           GatesentryTypes.RuleActionBlock,
			MITMAction:       GatesentryTypes.MITMActionEnable,
			DomainPatterns:   []string{"*"},
			URLRegexPatterns: []string{`.*\.pdf$`},
		},
		URL: "https://example.com/page.html",
	})

	if resp.Outcome != "skip" {
		t.Errorf("expected outcome 'skip', got %q", resp.Outcome)
	}

	// Steps 1-4 should pass
	for _, stepNum := range []int{1, 2, 3, 4} {
		s := findStep(resp.Steps, stepNum)
		if s == nil {
			t.Errorf("step %d missing", stepNum)
			continue
		}
		if s.Result != "pass" {
			t.Errorf("step %d: expected 'pass', got %q (%s)", stepNum, s.Result, s.Detail)
		}
	}

	// Step 5 should fail (URL regex mismatch)
	step5 := findStep(resp.Steps, 5)
	if step5 == nil {
		t.Fatal("step 5 missing")
	}
	if step5.Result != "fail" {
		t.Errorf("step 5: expected 'fail', got %q (%s)", step5.Result, step5.Detail)
	}

	// Steps beyond 5 should not be present (early exit)
	if findStep(resp.Steps, 6) != nil {
		t.Error("step 6 should not be present after URL regex mismatch")
	}
}

// ---------- Test 5: Live fetch ----------
// Rule with domain *, keyword_filter_enabled=true, mitm_action="enable"
// URL: http://example.com, live=true
// Expected: outcome="allow", response_status=200, response_content_type contains "text/html"

func TestRuleMatch_LiveFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live HTTP fetch in short mode")
	}

	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:              true,
			Action:               GatesentryTypes.RuleActionAllow,
			MITMAction:           GatesentryTypes.MITMActionEnable,
			DomainPatterns:       []string{"*"},
			KeywordFilterEnabled: true,
		},
		URL:  "http://example.com",
		Live: true,
	})

	if resp.Outcome != "allow" {
		t.Errorf("expected outcome 'allow', got %q", resp.Outcome)
	}

	if resp.ResponseStatus != 200 {
		t.Errorf("expected response_status 200, got %d", resp.ResponseStatus)
	}

	if resp.ResponseContentType == "" {
		t.Error("expected non-empty response_content_type")
	} else if !strings.Contains(strings.ToLower(resp.ResponseContentType), "text/html") {
		t.Errorf("expected content-type containing 'text/html', got %q", resp.ResponseContentType)
	}

	// All 8 steps should be present
	if len(resp.Steps) != 8 {
		t.Errorf("expected 8 steps, got %d", len(resp.Steps))
	}
}

// ==========================================================================
// Domain Lookup endpoint tests
// ==========================================================================

// callDomainLookup is a helper that POSTs a TestDomainLookupRequest to the
// handler and returns the decoded TestDomainLookupResponse.
func callDomainLookup(t *testing.T, req TestDomainLookupRequest) (*httptest.ResponseRecorder, TestDomainLookupResponse) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/api/test/domain-lookup", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	GSApiTestDomainLookup(rec, httpReq)

	var resp TestDomainLookupResponse
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return rec, resp
}

// setupMockDomainListManager installs a mock with test domains and returns a
// cleanup function that restores the original.
func setupMockDomainListManager(domains map[string][]string) func() {
	orig := domainListManager
	mock := newMockDomainListManager(domains)
	InitDomainListManager(mock)
	return func() {
		domainListManager = orig
	}
}

// ---------- Test 6: Domain lookup — found in list ----------

func TestDomainLookup_Found(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	dlCleanup := setupMockDomainListManager(map[string][]string{
		"list-ads": {"doubleclick.net", "ads.example.com", "tracker.test.com"},
	})
	defer dlCleanup()

	rec, resp := callDomainLookup(t, TestDomainLookupRequest{
		Domain:  "doubleclick.net",
		ListIDs: []string{"list-ads"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !resp.Found {
		t.Error("expected found=true for doubleclick.net")
	}
	if resp.InList != "list-ads" {
		t.Errorf("expected in_list='list-ads', got %q", resp.InList)
	}
}

// ---------- Test 7: Domain lookup — not found ----------

func TestDomainLookup_NotFound(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	dlCleanup := setupMockDomainListManager(map[string][]string{
		"list-ads": {"doubleclick.net", "ads.example.com"},
	})
	defer dlCleanup()

	rec, resp := callDomainLookup(t, TestDomainLookupRequest{
		Domain:  "google.com",
		ListIDs: []string{"list-ads"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if resp.Found {
		t.Error("expected found=false for google.com")
	}
}

// ---------- Test 8: Domain lookup — empty domain (400) ----------

func TestDomainLookup_EmptyDomain(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	body, _ := json.Marshal(TestDomainLookupRequest{Domain: "", ListIDs: []string{"list-1"}})
	httpReq := httptest.NewRequest(http.MethodPost, "/api/test/domain-lookup", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	GSApiTestDomainLookup(rec, httpReq)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty domain, got %d", rec.Code)
	}
}

// ---------- Test 9: Domain lookup — no domain list manager ----------

func TestDomainLookup_NoManager(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	// Ensure no DLM is set
	orig := domainListManager
	domainListManager = nil
	defer func() { domainListManager = orig }()

	rec, resp := callDomainLookup(t, TestDomainLookupRequest{
		Domain:  "example.com",
		ListIDs: []string{"list-1"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if resp.Found {
		t.Error("expected found=false when no manager is available")
	}
	if !strings.Contains(resp.Message, "not available") {
		t.Errorf("expected 'not available' message, got %q", resp.Message)
	}
}

// ==========================================================================
// Additional rule-match pipeline tests
// ==========================================================================

// ---------- Test 10: Rule-match with domain_lists (index-based) ----------

func TestRuleMatch_DomainListMatch(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	dlCleanup := setupMockDomainListManager(map[string][]string{
		"blocklist-1": {"doubleclick.net", "ads.example.com", "tracker.test.com"},
	})
	defer dlCleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:     true,
			Action:      GatesentryTypes.RuleActionBlock,
			MITMAction:  GatesentryTypes.MITMActionEnable,
			DomainLists: []string{"blocklist-1"},
		},
		URL: "https://doubleclick.net/ad.js",
	})

	if resp.Outcome != "block" {
		t.Errorf("expected outcome 'block', got %q", resp.Outcome)
	}
	if len(resp.Steps) != 8 {
		t.Errorf("expected 8 steps, got %d", len(resp.Steps))
	}

	// Step 3 should pass with domain list match
	step3 := findStep(resp.Steps, 3)
	if step3 == nil {
		t.Fatal("step 3 missing")
	}
	if step3.Result != "pass" {
		t.Errorf("step 3: expected 'pass', got %q (%s)", step3.Result, step3.Detail)
	}
	if !strings.Contains(step3.Detail, "domain list") {
		t.Errorf("step 3 detail should mention 'domain list', got %q", step3.Detail)
	}
}

// ---------- Test 11: Rule-match domain list — domain NOT in list ----------

func TestRuleMatch_DomainListMiss(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	dlCleanup := setupMockDomainListManager(map[string][]string{
		"blocklist-1": {"doubleclick.net", "ads.example.com"},
	})
	defer dlCleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:     true,
			Action:      GatesentryTypes.RuleActionBlock,
			MITMAction:  GatesentryTypes.MITMActionEnable,
			DomainLists: []string{"blocklist-1"},
		},
		URL: "https://google.com/search",
	})

	if resp.Outcome != "skip" {
		t.Errorf("expected outcome 'skip', got %q", resp.Outcome)
	}

	step3 := findStep(resp.Steps, 3)
	if step3 == nil {
		t.Fatal("step 3 missing")
	}
	if step3.Result != "fail" {
		t.Errorf("step 3: expected 'fail', got %q (%s)", step3.Result, step3.Detail)
	}
}

// ---------- Test 12: User mismatch — step 2 fail ----------

func TestRuleMatch_UserMismatch(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:        true,
			Action:         GatesentryTypes.RuleActionBlock,
			MITMAction:     GatesentryTypes.MITMActionEnable,
			DomainPatterns: []string{"*"},
			Users:          []string{"alice", "bob"},
		},
		URL:  "https://example.com",
		User: "charlie",
	})

	if resp.Outcome != "skip" {
		t.Errorf("expected outcome 'skip', got %q", resp.Outcome)
	}

	step2 := findStep(resp.Steps, 2)
	if step2 == nil {
		t.Fatal("step 2 missing")
	}
	if step2.Result != "fail" {
		t.Errorf("step 2: expected 'fail', got %q (%s)", step2.Result, step2.Detail)
	}
	if findStep(resp.Steps, 3) != nil {
		t.Error("step 3 should not be present after user mismatch")
	}
}

// ---------- Test 13: User match — step 2 pass ----------

func TestRuleMatch_UserMatch(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:        true,
			Action:         GatesentryTypes.RuleActionBlock,
			MITMAction:     GatesentryTypes.MITMActionEnable,
			DomainPatterns: []string{"*"},
			Users:          []string{"alice", "bob"},
		},
		URL:  "https://example.com",
		User: "alice",
	})

	if resp.Outcome != "block" {
		t.Errorf("expected outcome 'block', got %q", resp.Outcome)
	}

	step2 := findStep(resp.Steps, 2)
	if step2 == nil {
		t.Fatal("step 2 missing")
	}
	if step2.Result != "pass" {
		t.Errorf("step 2: expected 'pass', got %q (%s)", step2.Result, step2.Detail)
	}
}

// ---------- Test 14: HTTPS without MITM — steps 5-7 skipped ----------

func TestRuleMatch_HTTPSNoMITM(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:              true,
			Action:               GatesentryTypes.RuleActionAllow,
			MITMAction:           GatesentryTypes.MITMActionDisable,
			DomainPatterns:       []string{"*"},
			URLRegexPatterns:     []string{`.*\.pdf$`},
			KeywordFilterEnabled: true,
		},
		URL: "https://example.com/file.pdf",
	})

	// Even though URL pattern and keyword filter are set, MITM is off for HTTPS
	// so steps 5-7 are info/skipped and the rule goes to action
	if resp.Outcome != "allow" {
		t.Errorf("expected outcome 'allow', got %q", resp.Outcome)
	}

	// Step 4 should note MITM is disabled
	step4 := findStep(resp.Steps, 4)
	if step4 == nil {
		t.Fatal("step 4 missing")
	}
	if step4.Result != "info" {
		t.Errorf("step 4: expected 'info' (MITM disabled note), got %q (%s)", step4.Result, step4.Detail)
	}

	// Step 5 should be info (URL patterns not evaluated without MITM)
	step5 := findStep(resp.Steps, 5)
	if step5 == nil {
		t.Fatal("step 5 missing")
	}
	if step5.Result != "info" {
		t.Errorf("step 5: expected 'info', got %q (%s)", step5.Result, step5.Detail)
	}

	// Step 7 should be info (keyword filter not evaluated without MITM)
	step7 := findStep(resp.Steps, 7)
	if step7 == nil {
		t.Fatal("step 7 missing")
	}
	if step7.Result != "info" {
		t.Errorf("step 7: expected 'info', got %q (%s)", step7.Result, step7.Detail)
	}

	// Step 8 should still be present with the allow action
	step8 := findStep(resp.Steps, 8)
	if step8 == nil {
		t.Fatal("step 8 missing")
	}
	if step8.Result != "allow" {
		t.Errorf("step 8: expected 'allow', got %q", step8.Result)
	}
}

// ---------- Test 15: Catch-all rule (no domain criteria) ----------

func TestRuleMatch_CatchAll(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:    true,
			Action:     GatesentryTypes.RuleActionAllow,
			MITMAction: GatesentryTypes.MITMActionEnable,
			// No DomainPatterns, no DomainLists — catch-all
		},
		URL: "https://anything.example.com/page",
	})

	if resp.Outcome != "allow" {
		t.Errorf("expected outcome 'allow', got %q", resp.Outcome)
	}

	// Step 3 should pass as catch-all
	step3 := findStep(resp.Steps, 3)
	if step3 == nil {
		t.Fatal("step 3 missing")
	}
	if step3.Result != "pass" {
		t.Errorf("step 3: expected 'pass', got %q (%s)", step3.Result, step3.Detail)
	}
	if !strings.Contains(step3.Detail, "catch-all") {
		t.Errorf("step 3 detail should mention 'catch-all', got %q", step3.Detail)
	}

	if len(resp.Steps) != 8 {
		t.Errorf("expected 8 steps, got %d", len(resp.Steps))
	}
}

// ---------- Test 16: Allow action — full pipeline, step 8 = allow ----------

func TestRuleMatch_AllowAction(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:        true,
			Action:         GatesentryTypes.RuleActionAllow,
			MITMAction:     GatesentryTypes.MITMActionEnable,
			DomainPatterns: []string{"*.example.com"},
		},
		URL: "https://www.example.com/page",
	})

	if resp.Outcome != "allow" {
		t.Errorf("expected outcome 'allow', got %q", resp.Outcome)
	}

	step8 := findStep(resp.Steps, 8)
	if step8 == nil {
		t.Fatal("step 8 missing")
	}
	if step8.Result != "allow" {
		t.Errorf("step 8: expected 'allow', got %q", step8.Result)
	}
}

// ---------- Test 17: Empty URL returns 400 ----------

func TestRuleMatch_EmptyURL(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	body, _ := json.Marshal(TestRuleRequest{
		Rule: GatesentryTypes.Rule{Enabled: true},
		URL:  "",
	})
	httpReq := httptest.NewRequest(http.MethodPost, "/api/test/rule-match", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	GSApiTestRuleMatch(rec, httpReq)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty URL, got %d", rec.Code)
	}
}

// ---------- Test 18: URL regex match — step 5 pass ----------

func TestRuleMatch_URLRegexMatch(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:          true,
			Action:           GatesentryTypes.RuleActionBlock,
			MITMAction:       GatesentryTypes.MITMActionEnable,
			DomainPatterns:   []string{"*"},
			URLRegexPatterns: []string{`.*\.pdf$`},
		},
		URL: "https://example.com/document.pdf",
	})

	if resp.Outcome != "block" {
		t.Errorf("expected outcome 'block', got %q", resp.Outcome)
	}

	step5 := findStep(resp.Steps, 5)
	if step5 == nil {
		t.Fatal("step 5 missing")
	}
	if step5.Result != "pass" {
		t.Errorf("step 5: expected 'pass', got %q (%s)", step5.Result, step5.Detail)
	}
}

// ---------- Test 19: HTTP always inspectable (MITM irrelevant) ----------

func TestRuleMatch_HTTPAlwaysInspectable(t *testing.T) {
	cleanup := setupTestRuleEndpoint(t)
	defer cleanup()

	// Even with MITM disabled, HTTP URLs can still be inspected
	resp := callRuleMatch(t, TestRuleRequest{
		Rule: GatesentryTypes.Rule{
			Enabled:          true,
			Action:           GatesentryTypes.RuleActionBlock,
			MITMAction:       GatesentryTypes.MITMActionDisable,
			DomainPatterns:   []string{"*"},
			URLRegexPatterns: []string{`.*\.pdf$`},
		},
		URL: "http://example.com/document.pdf",
	})

	if resp.Outcome != "block" {
		t.Errorf("expected outcome 'block', got %q", resp.Outcome)
	}

	// Step 5 should pass (HTTP is always inspectable)
	step5 := findStep(resp.Steps, 5)
	if step5 == nil {
		t.Fatal("step 5 missing")
	}
	if step5.Result != "pass" {
		t.Errorf("step 5: expected 'pass' (HTTP inspectable), got %q (%s)", step5.Result, step5.Detail)
	}
}
