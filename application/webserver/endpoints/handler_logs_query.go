package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
	gatesentryproxy "bitbucket.org/abdullah_irfan/gatesentryproxy"
	"github.com/tidwall/buntdb"
)

// LogQueryEntry is a single log entry returned by the query API.
type LogQueryEntry struct {
	Time        int64  `json:"time"`
	IP          string `json:"ip"`
	URL         string `json:"url"`
	Type        string `json:"type"`
	Action      string `json:"action"`
	ActionLabel string `json:"action_label"`
	RuleName    string `json:"rule_name,omitempty"`
}

// LogQueryResponse is the response from the log query API.
type LogQueryResponse struct {
	Entries  []LogQueryEntry `json:"entries"`
	Total    int             `json:"total"`
	TotalAll int             `json:"total_all"`
	HasMore  bool            `json:"has_more"`
}

// proxyActionLabel returns a human-friendly label for a proxy action.
func proxyActionLabel(action string) string {
	switch action {
	case string(gatesentryproxy.ProxyActionBlockedUrl):
		return "Blocked (Domain/URL)"
	case string(gatesentryproxy.ProxyActionBlockedTextContent):
		return "Blocked (Keywords)"
	case string(gatesentryproxy.ProxyActionBlockedMediaContent):
		return "Blocked (Media)"
	case string(gatesentryproxy.ProxyActionBlockedFileType):
		return "Blocked (File Type)"
	case string(gatesentryproxy.ProxyActionBlockedTime):
		return "Blocked (Time)"
	case string(gatesentryproxy.ProxyActionBlockedInternetForUser):
		return "Blocked (User)"
	case string(gatesentryproxy.ProxyActionAuthFailure):
		return "Auth Failure"
	case string(gatesentryproxy.ProxyActionSSLBump):
		return "Allowed (MITM)"
	case string(gatesentryproxy.ProxyActionSSLDirect):
		return "Allowed (Passthrough)"
	case string(gatesentryproxy.ProxyActionFilterNone):
		return "Allowed"
	case string(gatesentryproxy.ProxyActionFilterError):
		return "Error"
	default:
		return action
	}
}

// dnsActionLabel returns a human-friendly label for a DNS response type.
func dnsActionLabel(action string) string {
	switch action {
	case "blocked":
		return "Blocked"
	case "cached":
		return "Cached"
	case "forward":
		return "Forwarded"
	case "ddns-add":
		return "DDNS Add"
	case "ddns-delete":
		return "DDNS Delete"
	case "ddns-rejected":
		return "DDNS Rejected"
	case "ddns-ptr":
		return "DDNS PTR (auto)"
	default:
		return action
	}
}

// isProxyBlocked returns true if the proxy action represents a blocked request.
func isProxyBlocked(action string) bool {
	switch action {
	case string(gatesentryproxy.ProxyActionBlockedTextContent),
		string(gatesentryproxy.ProxyActionBlockedMediaContent),
		string(gatesentryproxy.ProxyActionBlockedFileType),
		string(gatesentryproxy.ProxyActionBlockedTime),
		string(gatesentryproxy.ProxyActionBlockedInternetForUser),
		string(gatesentryproxy.ProxyActionBlockedUrl),
		string(gatesentryproxy.ProxyActionAuthFailure):
		return true
	}
	return false
}

// ApiLogsQuery handles GET /api/logs/query with filtering parameters:
//
//	?seconds=N       - time window (default 300 = 5 minutes)
//	&type=dns|proxy  - filter by entry type (default: all)
//	&filter=blocked|allowed - filter by action category (default: all)
//	&search=text     - text search in URL or IP
//	&user=ip         - filter by user/IP
//	&limit=N         - max entries to return (default 500, max 2000)
func ApiLogsQuery(w http.ResponseWriter, r *http.Request, logger *gatesentryLogger.Log) {
	q := r.URL.Query()

	seconds := 300
	if s := q.Get("seconds"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			seconds = v
		}
	}
	// Cap at 7 days
	if seconds > 604800 {
		seconds = 604800
	}

	limit := 500
	if s := q.Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 2000 {
		limit = 2000
	}

	typeFilter := strings.ToLower(q.Get("type"))     // "dns", "proxy", or ""
	actionFilter := strings.ToLower(q.Get("filter")) // "blocked", "allowed", or ""
	searchText := strings.ToLower(q.Get("search"))
	userFilter := q.Get("user")

	now := time.Now()
	totime := now.Unix()
	fromtime := totime - int64(seconds)

	from := gatesentryUtils.Int64toString(fromtime)
	to := gatesentryUtils.Int64toString(totime)

	var entries []LogQueryEntry
	totalAll := 0

	logger.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries", `{"time":`+to+`}`, `{"time":`+from+`}`, func(key, value string) bool {
			totalAll++

			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(value), &raw); err != nil {
				return true
			}

			entryType, _ := raw["type"].(string)
			if entryType != "dns" && entryType != "proxy" {
				return true
			}

			// Type filter
			if typeFilter != "" && entryType != typeFilter {
				return true
			}

			url, _ := raw["url"].(string)
			ip, _ := raw["ip"].(string)
			ruleName, _ := raw["ruleName"].(string)
			timeVal := int64(0)
			if t, ok := raw["time"].(float64); ok {
				timeVal = int64(t)
			}

			// Determine action string
			action := ""
			if entryType == "dns" {
				action, _ = raw["dnsResponseType"].(string)
			} else {
				action, _ = raw["proxyResponseType"].(string)
			}

			// Action filter
			if actionFilter == "blocked" {
				if entryType == "dns" && action != "blocked" {
					return true
				}
				if entryType == "proxy" && !isProxyBlocked(action) {
					return true
				}
			} else if actionFilter == "allowed" {
				if entryType == "dns" && action == "blocked" {
					return true
				}
				if entryType == "proxy" && isProxyBlocked(action) {
					return true
				}
			}

			// User filter
			if userFilter != "" && ip != userFilter {
				return true
			}

			// Search filter
			if searchText != "" {
				if !strings.Contains(strings.ToLower(url), searchText) &&
					!strings.Contains(strings.ToLower(ip), searchText) &&
					!strings.Contains(strings.ToLower(action), searchText) &&
					!strings.Contains(strings.ToLower(ruleName), searchText) {
					return true
				}
			}

			// Clean up proxy URLs
			if entryType == "proxy" {
				url = strings.Replace(url, "http://", "", 1)
			}

			// Build label
			label := ""
			if entryType == "dns" {
				label = dnsActionLabel(action)
			} else {
				label = proxyActionLabel(action)
			}

			if len(entries) < limit {
				entries = append(entries, LogQueryEntry{
					Time:        timeVal,
					IP:          ip,
					URL:         url,
					Type:        entryType,
					Action:      action,
					ActionLabel: label,
					RuleName:    ruleName,
				})
			}

			return true
		})
	})

	if entries == nil {
		entries = []LogQueryEntry{}
	}

	resp := LogQueryResponse{
		Entries:  entries,
		Total:    len(entries),
		TotalAll: totalAll,
		HasMore:  totalAll > len(entries),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
