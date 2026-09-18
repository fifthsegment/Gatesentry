package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
)

// decisionItem is the redacted, user-facing representation of a stored
// LogEntry. DeviceID is intentionally omitted: the /api/decisions response
// must never expose another household user's browsing history through a
// single decision record, so the stable identifier is dropped at the API
// boundary. Device-to-name enrichment happens only in the summary endpoint,
// which aggregates counts rather than per-device rows.
type decisionItem struct {
	Time              int64  `json:"time"`
	IP                string `json:"ip"`
	URL               string `json:"url"`
	Type              string `json:"type"`
	Layer             string `json:"layer"`
	Action            string `json:"action"`
	MatchedRule       string `json:"matched_rule"`
	Reason            string `json:"reason"`
	GroupID           string `json:"group_id"`
	Source            string `json:"source"`
	PolicyRevision    int    `json:"policy_revision"`
	DNSResponseType   string `json:"dnsResponseType"`
	ProxyResponseType string `json:"proxyResponseType"`
}

// affectedDevice enriches a client-address tally with the device name
// discovered for that address, when one is known. The logger only stores
// ClientIP; the API layer owns discovery, so the join lives here.
type affectedDevice struct {
	IP     string `json:"ip"`
	Device string `json:"device"`
	Count  int    `json:"count"`
}

// decisionSummaryResponse is the dashboard payload. AffectedDevices is the
// ClientIP tally enriched with device names from discovery; addresses with no
// known device keep their IP and an empty name so the count is not lost.
type decisionSummaryResponse struct {
	Window             string                           `json:"window"`
	From               int64                            `json:"from"`
	To                 int64                            `json:"to"`
	Total              int                              `json:"total"`
	ByAction           map[string]int                   `json:"by_action"`
	ByLayer            map[string]int                   `json:"by_layer"`
	TopBlockedDomains  []gatesentryLogger.DecisionCount `json:"top_blocked_domains"`
	TopReasons         []gatesentryLogger.DecisionCount `json:"top_reasons"`
	AffectedDevices    []affectedDevice                 `json:"affected_devices"`
	InspectionFailures int                              `json:"inspection_failures"`
}

// hasDecisionFilter reports whether any narrowing query parameter was set, so
// the handlers can default a bare request to a sensible recent window.
func hasDecisionFilter(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("from") != "" || q.Get("to") != "" || q.Get("action") != "" ||
		q.Get("layer") != "" || q.Get("domain") != "" || q.Get("ip") != "" ||
		q.Get("group") != "" || q.Get("reason") != ""
}

// GSApiDecisionsGET returns filtered, paginated decision history.
// GET /api/decisions?from=&to=&ip=&group=&domain=&action=&layer=&reason=&limit=&offset=
// All filter parameters are optional. DeviceID is never returned; the field
// is dropped at this boundary so a single decision record cannot leak a
// stable device identifier or another user's browsing history. A bare request
// (no filters) defaults to the last hour so the live view stays bounded.
func GSApiDecisionsGET(w http.ResponseWriter, r *http.Request, logger *gatesentryLogger.Log) {
	if logger == nil {
		http.Error(w, "{\"error\":\"Decision logger not initialized\"}", http.StatusServiceUnavailable)
		return
	}
	from, ok := parseUnixParam(r, "from")
	if !ok {
		http.Error(w, "{\"error\":\"from must be a non-negative unix timestamp\"}", http.StatusBadRequest)
		return
	}
	to, ok := parseUnixParam(r, "to")
	if !ok {
		http.Error(w, "{\"error\":\"to must be a non-negative unix timestamp\"}", http.StatusBadRequest)
		return
	}
	limit, ok := parseIntParam(r, "limit")
	if !ok {
		http.Error(w, "{\"error\":\"limit must be a non-negative integer\"}", http.StatusBadRequest)
		return
	}
	offset, ok := parseIntParam(r, "offset")
	if !ok {
		http.Error(w, "{\"error\":\"offset must be a non-negative integer\"}", http.StatusBadRequest)
		return
	}
	f := gatesentryLogger.DecisionFilter{
		IP:      r.URL.Query().Get("ip"),
		GroupID: r.URL.Query().Get("group"),
		Domain:  r.URL.Query().Get("domain"),
		Action:  r.URL.Query().Get("action"),
		Layer:   r.URL.Query().Get("layer"),
		Reason:  r.URL.Query().Get("reason"),
		From:    from,
		To:      to,
		Limit:   limit,
		Offset:  offset,
	}
	if !hasDecisionFilter(r) {
		f.From = time.Now().Unix() - 3600
	}
	entries, err := logger.QueryDecisions(f)
	if err != nil {
		http.Error(w, "{\"error\":\"Unable to read decisions\"}", http.StatusInternalServerError)
		return
	}
	items := make([]decisionItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, decisionItem{
			Time:              e.Time,
			IP:                e.IP,
			URL:               e.URL,
			Type:              e.Type,
			Layer:             e.Layer,
			Action:            e.Action,
			MatchedRule:       e.MatchedRule,
			Reason:            e.Reason,
			GroupID:           e.GroupID,
			Source:            e.Source,
			PolicyRevision:    e.PolicyRevision,
			DNSResponseType:   e.DNSResponseType,
			ProxyResponseType: e.ProxyResponseType,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}

// GSApiDecisionSummaryGET returns aggregated decision counts for the
// dashboard: frequent blocks, most-affected devices, and inspection failures.
// GET /api/decisions/summary?from=&to=&action=&layer=&domain=&group=&ip=&reason=
// The affected-address tally is enriched with device names from discovery;
// an address with no known device keeps its IP so the count stays visible.
// A bare request defaults to the full seven-day retention window.
func GSApiDecisionSummaryGET(w http.ResponseWriter, r *http.Request, logger *gatesentryLogger.Log) {
	if logger == nil {
		http.Error(w, "{\"error\":\"Decision logger not initialized\"}", http.StatusServiceUnavailable)
		return
	}
	from, ok := parseUnixParam(r, "from")
	if !ok {
		http.Error(w, "{\"error\":\"from must be a non-negative unix timestamp\"}", http.StatusBadRequest)
		return
	}
	to, ok := parseUnixParam(r, "to")
	if !ok {
		http.Error(w, "{\"error\":\"to must be a non-negative unix timestamp\"}", http.StatusBadRequest)
		return
	}
	f := gatesentryLogger.DecisionFilter{
		IP:      r.URL.Query().Get("ip"),
		GroupID: r.URL.Query().Get("group"),
		Domain:  r.URL.Query().Get("domain"),
		Action:  r.URL.Query().Get("action"),
		Layer:   r.URL.Query().Get("layer"),
		Reason:  r.URL.Query().Get("reason"),
		From:    from,
		To:      to,
	}
	if !hasDecisionFilter(r) {
		f.From = time.Now().Unix() - 7*24*3600
	}
	summary, err := logger.DecisionSummary(f)
	if err != nil {
		http.Error(w, "{\"error\":\"Unable to read decision summary\"}", http.StatusInternalServerError)
		return
	}
	resp := decisionSummaryResponse{
		Window:             summary.Window,
		From:               summary.From,
		To:                 summary.To,
		Total:              summary.Total,
		ByAction:           summary.ByAction,
		ByLayer:            summary.ByLayer,
		TopBlockedDomains:  summary.TopBlockedDomains,
		TopReasons:         summary.TopReasons,
		InspectionFailures: summary.InspectionFailures,
	}
	resp.AffectedDevices = enrichAffectedDevices(summary.AffectedAddresses)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// enrichAffectedDevices joins the client-address tally with device names from
// discovery. It never adds a second decision record; it only labels counts.
// If the device store is unavailable (DNS server not running), addresses are
// returned with empty device names so the summary degrades gracefully.
func enrichAffectedDevices(addresses []gatesentryLogger.DecisionCount) []affectedDevice {
	out := make([]affectedDevice, 0, len(addresses))
	for _, a := range addresses {
		out = append(out, affectedDevice{
			IP:     a.Key,
			Device: deviceNameForIPFromStore(a.Key),
			Count:  a.Count,
		})
	}
	return out
}

// deviceNameForIPFromStore looks up the friendly device name for a client
// address via the discovery store. It returns "" when no device is known or
// the store is unavailable (e.g. DNS server not running in tests); the
// caller keeps the IP so the count remains visible.
func deviceNameForIPFromStore(ip string) string {
	ds := gatesentryDnsServer.GetDeviceStore()
	if ds == nil {
		return ""
	}
	for _, d := range ds.GetAllDevices() {
		if d.IPv4 == ip || d.IPv6 == ip {
			return d.GetDisplayName()
		}
	}
	return ""
}

// parseUnixParam parses an optional unix-seconds query parameter. It returns
// (0, true) for an absent value and (0, false) for a malformed one; the
// caller writes the 400 response so the handler stops instead of continuing
// with a half-written response.
func parseUnixParam(r *http.Request, name string) (int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}

// parseIntParam parses an optional integer query parameter, returning
// (0, true) for an absent value and (0, false) for a malformed one; the
// caller writes the 400 response.
func parseIntParam(r *http.Request, name string) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0, false
	}
	return v, true
}
