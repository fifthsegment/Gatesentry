package gatesentry2logger

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2utils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
	"github.com/tidwall/buntdb"
)

// DecisionCount is a labeled tally used by DecisionSummary.
type DecisionCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// DecisionFilter narrows the decision history for the /api/decisions query.
// Zero values mean "no constraint". The logger is device-agnostic: it filters
// on ClientIP (LogEntry.IP) rather than DeviceID, so the read path never
// resolves stable device identifiers. Device-to-name enrichment happens in
// the API layer, which owns discovery.
type DecisionFilter struct {
	From    int64  // unix seconds, inclusive; 0 = seven-day window start
	To      int64  // unix seconds, inclusive; 0 = now
	IP      string // case-insensitive substring on ClientIP
	GroupID string // exact match on the policy group in effect
	Domain  string // case-insensitive substring on Domain/URL
	Action  string // exact match on normalized DecisionAction
	Layer   string // exact match on DecisionLayer
	Reason  string // case-insensitive substring on Reason or MatchedRule
	Limit   int    // cap; 0 default 100, absolute max 500
	Offset  int    // skip after filtering, newest first
}

const defaultDecisionLimit = 100
const maxDecisionLimit = 500

func (f DecisionFilter) limit() int {
	if f.Limit <= 0 {
		return defaultDecisionLimit
	}
	if f.Limit > maxDecisionLimit {
		return maxDecisionLimit
	}
	return f.Limit
}

// window resolves the From/To bounds, defaulting to the full seven-day
// retention window covered by the entries index TTL.
func (f DecisionFilter) window() (from, to int64) {
	now := time.Now().Unix()
	to = f.To
	if to <= 0 {
		to = now
	}
	from = f.From
	if from <= 0 {
		from = now - int64(Log_Entry_Expires/time.Second)
	}
	if from > to {
		from = to
	}
	return from, to
}

// matches reports whether an entry satisfies every non-zero filter field.
// It is case-insensitive on free-text fields (IP, Domain, Reason) so
// operators do not have to match exact casing.
func (f DecisionFilter) matches(e LogEntry) bool {
	if f.IP != "" && !strings.Contains(strings.ToLower(e.IP), strings.ToLower(f.IP)) {
		return false
	}
	if f.GroupID != "" && e.GroupID != f.GroupID {
		return false
	}
	if f.Domain != "" && !strings.Contains(strings.ToLower(e.URL), strings.ToLower(f.Domain)) {
		return false
	}
	if f.Action != "" && e.Action != f.Action {
		return false
	}
	if f.Layer != "" && e.Layer != f.Layer {
		return false
	}
	if f.Reason != "" {
		hay := strings.ToLower(e.Reason + " " + e.MatchedRule)
		if !strings.Contains(hay, strings.ToLower(f.Reason)) {
			return false
		}
	}
	return true
}

// QueryDecisions returns filtered decision entries, newest first, within the
// From-To window and the bounded limit/offset. It scans the same entries
// index LogDecision writes to; it never creates a second decision record and
// it relies on the seven-day TTL for retention rather than its own expiry.
// Entries that fail to parse are skipped, matching the existing log viewers.
func (L *Log) QueryDecisions(f DecisionFilter) ([]LogEntry, error) {
	if L == nil || L.Database == nil {
		return []LogEntry{}, nil
	}
	from, to := f.window()
	limit := f.limit()
	results := []LogEntry{}
	seen := 0
	err := L.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries",
			"{\"time\":"+gatesentry2utils.Int64toString(to)+"}",
			"{\"time\":"+gatesentry2utils.Int64toString(from)+"}",
			func(key, value string) bool {
				var e LogEntry
				if err := json.Unmarshal([]byte(value), &e); err != nil {
					return true
				}
				if !f.matches(e) {
					return true
				}
				if seen < f.Offset {
					seen++
					return true
				}
				if len(results) >= limit {
					return false
				}
				results = append(results, e)
				return true
			})
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// DecisionSummary aggregates the decision history for a dashboard view. It
// is computed in a single index scan so a busy window does not require many
// round trips. The logger keeps ClientIP counts only; the API layer enriches
// affected addresses with device names from discovery.
type DecisionSummary struct {
	Window             string          `json:"window"`
	From               int64           `json:"from"`
	To                 int64           `json:"to"`
	Total              int             `json:"total"`
	ByAction           map[string]int  `json:"by_action"`
	ByLayer            map[string]int  `json:"by_layer"`
	TopBlockedDomains  []DecisionCount `json:"top_blocked_domains"`
	TopReasons         []DecisionCount `json:"top_reasons"`
	AffectedAddresses  []DecisionCount `json:"affected_addresses"`
	InspectionFailures int             `json:"inspection_failures"`
	// UniqueClients and UniqueDomains count distinct client addresses and
	// hosts seen in the window.
	UniqueClients int `json:"unique_clients"`
	UniqueDomains int `json:"unique_domains"`
	// TopDomains ranks every requested host, blocked or not.
	TopDomains []DecisionCount `json:"top_domains"`
	// BlocksByGroup counts blocks per policy group ID; the handler names them.
	BlocksByGroup []DecisionCount `json:"blocks_by_group"`
	// Timeline buckets the window by hour (up to two days) or by day.
	Timeline []TimelineBucket `json:"timeline"`
}

// TimelineBucket is one slice of the summary window.
type TimelineBucket struct {
	Start   int64 `json:"start"`
	Total   int   `json:"total"`
	Blocked int   `json:"blocked"`
}

// decisionHost reduces a logged URL to its host so proxy URLs and DNS names
// count toward the same domain.
func decisionHost(raw string) string {
	host := raw
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.TrimSuffix(strings.ToLower(host), ".")
}

// timelineStep picks the bucket size for a window: hourly for up to two
// days so a daily view has shape, daily beyond that.
func timelineStep(from, to int64) int64 {
	if to-from <= 2*86400 {
		return 3600
	}
	return 86400
}

// DecisionSummary aggregates decisions in the filter window into counts by
// action and layer, the most-blocked domains, the most-common reasons, the
// most-affected client addresses, and a count of decisions that could not
// reach a normal outcome (ActionDecisionError), which are surfaced to the
// operator as filter/inspection failures.
func (L *Log) DecisionSummary(f DecisionFilter) (DecisionSummary, error) {
	s := DecisionSummary{ByAction: map[string]int{}, ByLayer: map[string]int{}}
	if L == nil || L.Database == nil {
		return s, nil
	}
	from, to := f.window()
	s.From = from
	s.To = to
	s.Window = windowLabel(from, to)
	blockedDomains := map[string]int{}
	reasons := map[string]int{}
	addresses := map[string]int{}
	domains := map[string]int{}
	groups := map[string]int{}
	step := timelineStep(from, to)
	start := from - from%step
	buckets := make([]TimelineBucket, 0, (to-start)/step+1)
	for t := start; t <= to; t += step {
		buckets = append(buckets, TimelineBucket{Start: t})
	}
	err := L.Database.View(func(tx *buntdb.Tx) error {
		return tx.DescendRange("entries",
			"{\"time\":"+gatesentry2utils.Int64toString(to)+"}",
			"{\"time\":"+gatesentry2utils.Int64toString(from)+"}",
			func(key, value string) bool {
				var e LogEntry
				if err := json.Unmarshal([]byte(value), &e); err != nil {
					return true
				}
				if !f.matches(e) {
					return true
				}
				s.Total++
				blocked := e.Action == string(gatesentryPolicy.ActionDecisionBlock)
				if host := decisionHost(e.URL); host != "" {
					domains[host]++
				}
				if blocked && e.GroupID != "" {
					groups[e.GroupID]++
				}
				if i := (e.Time - start) / step; e.Time >= start && int(i) < len(buckets) {
					buckets[i].Total++
					if blocked {
						buckets[i].Blocked++
					}
				}
				if e.Action != "" {
					s.ByAction[e.Action]++
				}
				if e.Layer != "" {
					s.ByLayer[e.Layer]++
				}
				if blocked && e.URL != "" {
					blockedDomains[decisionHost(e.URL)]++
				}
				if e.Reason != "" {
					reasons[e.Reason]++
				}
				if e.IP != "" {
					addresses[e.IP]++
				}
				if e.Action == string(gatesentryPolicy.ActionDecisionError) {
					s.InspectionFailures++
				}
				return true
			})
	})
	if err != nil {
		return s, err
	}
	s.TopBlockedDomains = topN(blockedDomains, 10)
	s.TopReasons = topN(reasons, 10)
	s.AffectedAddresses = topN(addresses, 10)
	s.UniqueClients = len(addresses)
	s.UniqueDomains = len(domains)
	s.TopDomains = topN(domains, 10)
	s.BlocksByGroup = topN(groups, 10)
	s.Timeline = buckets
	return s, nil
}

// topN returns the n entries with the highest counts, breaking ties by key
// for deterministic output.
func topN(counts map[string]int, n int) []DecisionCount {
	out := make([]DecisionCount, 0, len(counts))
	for k, v := range counts {
		out = append(out, DecisionCount{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// windowLabel renders the scan span as a short, human-readable duration so
// the dashboard can label the summary without re-deriving it.
func windowLabel(from, to int64) string {
	span := to - from
	if span <= 0 {
		return "instant"
	}
	switch {
	case span >= 2*86400:
		return fmt.Sprintf("%d days", span/86400)
	case span >= 86400:
		return "1 day"
	case span >= 2*3600:
		return fmt.Sprintf("%d hours", span/3600)
	default:
		return fmt.Sprintf("%d seconds", span)
	}
}
