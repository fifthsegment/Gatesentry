package gatesentryWebserverEndpoints

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	gatesentry2filters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

// --- Module-level state ---

var testSettings *gatesentry2storage.MapStore
var testFilters *[]gatesentry2filters.GSFilter

// InitTestEndpoints initializes shared state for test endpoints.
func InitTestEndpoints(settings *gatesentry2storage.MapStore, filters *[]gatesentry2filters.GSFilter) {
	testSettings = settings
	testFilters = filters
}

// --- API types ---

// TestStep represents one evaluation step in the pipeline.
type TestStep struct {
	Step   int    `json:"step"`
	Name   string `json:"name"`
	Result string `json:"result"` // pass, fail, skip, info, block, allow
	Detail string `json:"detail"`
}

// TestRuleRequest is the request body for the rule test endpoint.
type TestRuleRequest struct {
	Rule GatesentryTypes.Rule `json:"rule"`
	URL  string               `json:"url"`
	User string               `json:"user"`
	Live bool                 `json:"live"` // if true, make real HTTP request for steps 6-7
}

// TestRuleResponse is the response for the rule test endpoint.
type TestRuleResponse struct {
	Steps   []TestStep `json:"steps"`
	Outcome string     `json:"outcome"` // allow, block, skip, error
	Reason  string     `json:"reason"`

	// Live response data (only populated when Live=true)
	ResponseStatus      int    `json:"response_status,omitempty"`
	ResponseContentType string `json:"response_content_type,omitempty"`
	KeywordScore        int    `json:"keyword_score,omitempty"`
	KeywordWatermark    int    `json:"keyword_watermark,omitempty"`
}

// --- Domain matching (mirrors application/rules.go logic) ---

func testMatchDomain(pattern, domain string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	domain = strings.ToLower(strings.TrimSpace(domain))
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if pattern == domain {
		return true
	}
	if strings.Contains(pattern, "*") {
		return testGlobMatch(pattern, domain)
	}
	return false
}

func testGlobMatch(pattern, str string) bool {
	// Fast path: *.example.com
	if strings.HasPrefix(pattern, "*.") && !strings.Contains(pattern[2:], "*") {
		suffix := pattern[2:]
		return strings.HasSuffix(str, "."+suffix) || str == suffix
	}
	// General glob: split on * and check parts appear in order
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(str, parts[0]) {
		return false
	}
	str = str[len(parts[0]):]
	for i := 1; i < len(parts)-1; i++ {
		idx := strings.Index(str, parts[i])
		if idx < 0 {
			return false
		}
		str = str[idx+len(parts[i]):]
	}
	return strings.HasSuffix(str, parts[len(parts)-1])
}

// --- Content-type matching (mirrors rules.go CheckContentTypeBlocked) ---

func testCheckContentType(contentType string, blockedTypes []string) (bool, string) {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	for _, blocked := range blockedTypes {
		b := strings.ToLower(strings.TrimSpace(blocked))
		if b != "" && strings.Contains(ct, b) {
			return true, b
		}
	}
	return false, ""
}

// --- GSApiTestRuleMatch: full 8-step pipeline evaluation against an unsaved rule ---

func GSApiTestRuleMatch(w http.ResponseWriter, r *http.Request) {
	var req TestRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Normalize URL
	inputURL := req.URL
	if !strings.HasPrefix(strings.ToLower(inputURL), "http://") && !strings.HasPrefix(strings.ToLower(inputURL), "https://") {
		inputURL = "http://" + inputURL
	}
	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		sendTestResponse(w, nil, "error", "Invalid URL: "+err.Error())
		return
	}

	hostname := parsedURL.Hostname()
	fullURL := parsedURL.String()
	isHTTPS := parsedURL.Scheme == "https"
	rule := &req.Rule

	var steps []TestStep

	// ── Step 1: Rule status + time restriction ──

	if !rule.Enabled {
		steps = append(steps, TestStep{1, "Rule Status", "skip", "Rule is disabled — skipped"})
		sendTestResponse(w, steps, "skip", "Rule is disabled — would be skipped")
		return
	}

	if rule.TimeRestriction != nil && (rule.TimeRestriction.From != "" || rule.TimeRestriction.To != "") {
		now := time.Now()
		currentTime := now.Format("15:04")
		from := rule.TimeRestriction.From
		to := rule.TimeRestriction.To
		if from == "" {
			from = "00:00"
		}
		if to == "" {
			to = "23:59"
		}

		inWindow := false
		if from <= to {
			inWindow = currentTime >= from && currentTime <= to
		} else {
			// Overnight window (e.g. 22:00-06:00)
			inWindow = currentTime >= from || currentTime <= to
		}

		if !inWindow {
			steps = append(steps, TestStep{1, "Active Hours", "skip", fmt.Sprintf("Current time %s is outside %s – %s", currentTime, from, to)})
			sendTestResponse(w, steps, "skip", "Outside active hours — rule skipped")
			return
		}
		steps = append(steps, TestStep{1, "Active Hours", "pass", fmt.Sprintf("Time %s is within %s – %s", currentTime, from, to)})
	} else {
		steps = append(steps, TestStep{1, "Rule Status", "pass", "Rule is enabled, no time restriction"})
	}

	// ── Step 2: User match ──

	if len(rule.Users) > 0 {
		if req.User == "" {
			steps = append(steps, TestStep{2, "User Match", "info", fmt.Sprintf("Restricted to: %s (no test user specified)", strings.Join(rule.Users, ", "))})
		} else {
			userFound := false
			for _, u := range rule.Users {
				if u == req.User {
					userFound = true
					break
				}
			}
			if userFound {
				steps = append(steps, TestStep{2, "User Match", "pass", fmt.Sprintf("User \"%s\" is in the allowed list", req.User)})
			} else {
				steps = append(steps, TestStep{2, "User Match", "fail", fmt.Sprintf("User \"%s\" is not in the allowed list — rule skipped", req.User)})
				sendTestResponse(w, steps, "skip", fmt.Sprintf("User \"%s\" doesn't match — rule skipped", req.User))
				return
			}
		}
	} else {
		steps = append(steps, TestStep{2, "User Match", "pass", "No user restriction — applies to all users"})
	}

	// ── Step 3: Domain match ──

	hasDomainPatterns := len(rule.DomainPatterns) > 0 || rule.Domain != ""
	hasDomainLists := len(rule.DomainLists) > 0

	if !hasDomainPatterns && !hasDomainLists {
		steps = append(steps, TestStep{3, "Domain Match", "pass", "No domain criteria — catch-all (matches all domains)"})
	} else {
		domainMatched := false
		matchedBy := ""

		// Legacy single domain field
		if rule.Domain != "" && testMatchDomain(rule.Domain, hostname) {
			domainMatched = true
			matchedBy = fmt.Sprintf("legacy pattern \"%s\"", rule.Domain)
		}

		// Domain patterns (wildcards)
		if !domainMatched {
			for _, p := range rule.DomainPatterns {
				if testMatchDomain(p, hostname) {
					domainMatched = true
					matchedBy = fmt.Sprintf("pattern \"%s\"", p)
					break
				}
			}
		}

		// Domain lists — server-side lookup via DomainListManager index (O(1))
		if !domainMatched && hasDomainLists {
			dlm := GetDomainListManager()
			if dlm != nil {
				lowerHost := strings.ToLower(hostname)
				found, matchedListID := dlm.IsDomainInAnyList(lowerHost, rule.DomainLists)
				if found {
					domainMatched = true
					matchedBy = fmt.Sprintf("domain list \"%s\"", matchedListID)
				}
			} else {
				steps = append(steps, TestStep{3, "Domain Match", "info", "Domain list manager not available — cannot check domain lists"})
			}
		}

		if domainMatched {
			steps = append(steps, TestStep{3, "Domain Match", "pass", fmt.Sprintf("\"%s\" matched via %s", hostname, matchedBy)})
		} else {
			steps = append(steps, TestStep{3, "Domain Match", "fail", fmt.Sprintf("\"%s\" does not match any domain pattern or list — rule skipped", hostname)})
			sendTestResponse(w, steps, "skip", fmt.Sprintf("Domain \"%s\" doesn't match — rule skipped", hostname))
			return
		}
	}

	// ── Step 4: MITM resolution ──

	globalMITM := false
	if testSettings != nil {
		globalMITM = testSettings.Get("enable_https_filtering") == "true"
	}

	mitmOn := false
	mitmLabel := ""
	switch rule.MITMAction {
	case GatesentryTypes.MITMActionEnable:
		mitmOn = true
		mitmLabel = "enable"
	case GatesentryTypes.MITMActionDisable:
		mitmOn = false
		mitmLabel = "disable"
	default: // "default" or empty
		mitmOn = globalMITM
		if globalMITM {
			mitmLabel = "default → enabled (global setting)"
		} else {
			mitmLabel = "default → disabled (global setting)"
		}
	}

	canInspect := !isHTTPS || mitmOn

	if isHTTPS && !mitmOn {
		steps = append(steps, TestStep{4, "MITM Resolution", "info",
			fmt.Sprintf("MITM: %s — HTTPS without decryption, steps 5–7 skipped", mitmLabel)})
	} else if isHTTPS {
		steps = append(steps, TestStep{4, "MITM Resolution", "pass",
			fmt.Sprintf("MITM: %s — HTTPS will be decrypted for full inspection", mitmLabel)})
	} else {
		steps = append(steps, TestStep{4, "MITM Resolution", "pass",
			fmt.Sprintf("HTTP — full inspection always available (MITM: %s)", mitmLabel)})
	}

	// ── Step 5: URL patterns ──

	hasURLPatterns := len(rule.URLRegexPatterns) > 0
	if hasURLPatterns && canInspect {
		urlMatched := false
		matchedPat := ""
		var regexErr string
		for _, p := range rule.URLRegexPatterns {
			re, err := regexp.Compile("(?i)" + p)
			if err != nil {
				regexErr = fmt.Sprintf("Invalid regex \"%s\": %v", p, err)
				continue
			}
			if re.MatchString(fullURL) {
				urlMatched = true
				matchedPat = p
				break
			}
		}
		if urlMatched {
			steps = append(steps, TestStep{5, "URL Pattern", "pass", fmt.Sprintf("URL matches regex \"%s\"", matchedPat)})
		} else if regexErr != "" {
			steps = append(steps, TestStep{5, "URL Pattern", "fail", regexErr})
			sendTestResponse(w, steps, "error", regexErr)
			return
		} else {
			steps = append(steps, TestStep{5, "URL Pattern", "fail",
				fmt.Sprintf("URL doesn't match any of %d pattern(s) — rule skipped", len(rule.URLRegexPatterns))})
			sendTestResponse(w, steps, "skip", "URL doesn't match — rule skipped")
			return
		}
	} else if hasURLPatterns && !canInspect {
		steps = append(steps, TestStep{5, "URL Pattern", "info",
			"URL patterns defined but HTTPS without MITM — not evaluated"})
	} else {
		steps = append(steps, TestStep{5, "URL Pattern", "pass",
			"No URL patterns — not evaluated (effective match)"})
	}

	// ── Steps 6-7: Live fetch for content-type and keywords ──

	var liveContentType string
	var liveBody string
	var liveStatus int
	var liveFetched bool
	var liveError string

	if req.Live && canInspect {
		liveStatus, liveContentType, liveBody, liveFetched, liveError = doLiveFetch(fullURL)
		if !liveFetched && liveError != "" {
			log.Printf("[TestRule] Live fetch failed: %s", liveError)
		}
	}

	// ── Step 6: Content-type ──

	hasContentTypes := len(rule.BlockedContentTypes) > 0
	if hasContentTypes && canInspect {
		if liveFetched {
			matched, matchedType := testCheckContentType(liveContentType, rule.BlockedContentTypes)
			if matched {
				steps = append(steps, TestStep{6, "Content-Type", "pass",
					fmt.Sprintf("Response \"%s\" matches blocked type \"%s\"", liveContentType, matchedType)})
			} else {
				steps = append(steps, TestStep{6, "Content-Type", "fail",
					fmt.Sprintf("Response \"%s\" doesn't match any of %d blocked type(s) — rule skipped",
						liveContentType, len(rule.BlockedContentTypes))})
				sendTestResponse(w, steps, "skip", "Content-Type doesn't match — rule skipped")
				return
			}
		} else if liveError != "" {
			steps = append(steps, TestStep{6, "Content-Type", "info",
				fmt.Sprintf("Could not fetch URL: %s. %d type(s) would be evaluated at request time.", liveError, len(rule.BlockedContentTypes))})
		} else {
			steps = append(steps, TestStep{6, "Content-Type", "info",
				fmt.Sprintf("%d type(s) defined — evaluated at request time against actual response", len(rule.BlockedContentTypes))})
		}
	} else if hasContentTypes && !canInspect {
		steps = append(steps, TestStep{6, "Content-Type", "info",
			"Content-type filters defined but HTTPS without MITM — not evaluated"})
	} else {
		steps = append(steps, TestStep{6, "Content-Type", "pass",
			"No content-type filters — not evaluated (effective match)"})
	}

	// ── Step 7: Keyword filter ──

	keywordScore := 0
	keywordWatermark := 0

	if rule.KeywordFilterEnabled && canInspect {
		if liveFetched && isHTMLContent(liveContentType) {
			keywordScore, keywordWatermark = testScanKeywords(liveBody)
			if keywordScore > 0 && keywordScore > keywordWatermark {
				steps = append(steps, TestStep{7, "Keyword Filter", "block",
					fmt.Sprintf("Keyword score %d > watermark %d — FORCE BLOCK", keywordScore, keywordWatermark)})
				resp := TestRuleResponse{
					Steps:               steps,
					Outcome:             "block",
					Reason:              fmt.Sprintf("Keyword filter triggered (score %d > watermark %d)", keywordScore, keywordWatermark),
					ResponseStatus:      liveStatus,
					ResponseContentType: liveContentType,
					KeywordScore:        keywordScore,
					KeywordWatermark:    keywordWatermark,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			} else if keywordScore > 0 {
				steps = append(steps, TestStep{7, "Keyword Filter", "pass",
					fmt.Sprintf("Keyword score %d ≤ watermark %d — below threshold", keywordScore, keywordWatermark)})
			} else {
				steps = append(steps, TestStep{7, "Keyword Filter", "pass",
					"No blocked keywords found in page content"})
			}
		} else if liveFetched && !isHTMLContent(liveContentType) {
			steps = append(steps, TestStep{7, "Keyword Filter", "info",
				fmt.Sprintf("Response is \"%s\" — keyword scanning only applies to HTML", liveContentType)})
		} else if liveError != "" {
			steps = append(steps, TestStep{7, "Keyword Filter", "info",
				fmt.Sprintf("Could not fetch URL: %s. Keywords would be scanned at request time.", liveError)})
		} else {
			steps = append(steps, TestStep{7, "Keyword Filter", "info",
				"Enabled — response body will be scanned at request time"})
		}
	} else if rule.KeywordFilterEnabled && !canInspect {
		steps = append(steps, TestStep{7, "Keyword Filter", "info",
			"Enabled but HTTPS without MITM — not evaluated"})
	} else {
		steps = append(steps, TestStep{7, "Keyword Filter", "pass",
			"Disabled — not evaluated"})
	}

	// ── Step 8: Action ──

	actionLabel := "ALLOW"
	outcomeResult := "allow"
	if rule.Action == GatesentryTypes.RuleActionBlock {
		actionLabel = "BLOCK"
		outcomeResult = "block"
	}
	steps = append(steps, TestStep{8, "Action", outcomeResult,
		fmt.Sprintf("Rule action: %s", actionLabel)})

	resp := TestRuleResponse{
		Steps:               steps,
		Outcome:             outcomeResult,
		Reason:              fmt.Sprintf("This rule would %s the request", actionLabel),
		ResponseStatus:      liveStatus,
		ResponseContentType: liveContentType,
		KeywordScore:        keywordScore,
		KeywordWatermark:    keywordWatermark,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Domain Lookup endpoint ---

// TestDomainLookupRequest is the request for domain list membership check.
type TestDomainLookupRequest struct {
	Domain  string   `json:"domain"`
	ListIDs []string `json:"list_ids"`
}

// TestDomainLookupResponse is the response for domain list membership check.
type TestDomainLookupResponse struct {
	Domain  string `json:"domain"`
	Found   bool   `json:"found"`
	InList  string `json:"in_list,omitempty"`
	Message string `json:"message"`
}

// GSApiTestDomainLookup checks if a domain is in any of the specified domain lists.
func GSApiTestDomainLookup(w http.ResponseWriter, r *http.Request) {
	var req TestDomainLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Domain == "" {
		http.Error(w, "Domain is required", http.StatusBadRequest)
		return
	}

	domain := strings.ToLower(strings.TrimSpace(req.Domain))

	dlm := GetDomainListManager()
	if dlm == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestDomainLookupResponse{
			Domain:  domain,
			Found:   false,
			Message: "Domain list manager not available",
		})
		return
	}

	found, matchedList := dlm.IsDomainInAnyList(domain, req.ListIDs)
	if found {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(TestDomainLookupResponse{
			Domain:  domain,
			Found:   true,
			InList:  matchedList,
			Message: fmt.Sprintf("Domain \"%s\" found in list \"%s\"", domain, matchedList),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TestDomainLookupResponse{
		Domain:  domain,
		Found:   false,
		Message: fmt.Sprintf("Domain \"%s\" not found in any of %d list(s)", domain, len(req.ListIDs)),
	})
}

// --- Helper functions ---

func sendTestResponse(w http.ResponseWriter, steps []TestStep, outcome, reason string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TestRuleResponse{
		Steps:   steps,
		Outcome: outcome,
		Reason:  reason,
	})
}

func isHTMLContent(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml")
}

// doLiveFetch makes a real HTTP request to get response headers and body.
func doLiveFetch(targetURL string) (statusCode int, contentType string, body string, ok bool, errMsg string) {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(targetURL)
	if err != nil {
		return 0, "", "", false, err.Error()
	}
	defer resp.Body.Close()

	contentType = resp.Header.Get("Content-Type")
	statusCode = resp.StatusCode

	// Read up to 2MB for keyword scanning
	maxBytes := int64(2 * 1024 * 1024)
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return statusCode, contentType, "", true, ""
	}

	return statusCode, contentType, string(data), true, ""
}

// testScanKeywords runs keyword scanning against page content using the loaded Filters.
func testScanKeywords(body string) (score int, watermark int) {
	// Default watermark
	watermark = 2000

	// Get strictness from settings
	if testSettings != nil {
		strictStr := testSettings.Get("strictness")
		if strictStr != "" {
			fmt.Sscanf(strictStr, "%d", &watermark)
		}
	}

	// Find the keyword filter in the loaded Filters slice
	if testFilters == nil {
		return 0, watermark
	}

	for i := range *testFilters {
		f := &(*testFilters)[i]
		if f.Id == "bVxTPTOXiqGRbhF" {
			// Use the filter's own Strictness (kept in sync by runtime.go)
			watermark = f.Strictness

			// Run the same scoring logic as FilterWords
			bodyLower := strings.ToLower(body)
			for _, entry := range f.FileContents {
				keyword := strings.ToLower(entry.Content)
				if keyword == "" {
					continue
				}
				count := strings.Count(bodyLower, keyword)
				score += count * entry.Score
			}
			return score, watermark
		}
	}

	return 0, watermark
}
