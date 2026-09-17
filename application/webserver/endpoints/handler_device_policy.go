package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	"github.com/gorilla/mux"
)

// deviceGroupView is the enforcing subset of a policy group. It deliberately
// excludes Users and UnknownDevicePolicy: those are configuration details,
// while this view answers which domain rules this device's assignment gives it.
type deviceGroupView struct {
	ID                     string                        `json:"id"`
	Name                   string                        `json:"name"`
	Action                 gatesentryPolicy.PolicyAction `json:"action"`
	Domains                []string                      `json:"domains"`
	Priority               int                           `json:"priority"`
	InapplicableConditions []string                      `json:"inapplicable_conditions"`
}

type deviceAddressView struct {
	IP                  string `json:"ip"`
	Source              string `json:"source"`
	Explanation         string `json:"explanation"`
	SharedAddress       bool   `json:"shared_address"`
	StaleObservation    bool   `json:"stale_observation"`
	DNSResolvesDevice   bool   `json:"dns_resolves_device"`
	ProxyResolvesDevice bool   `json:"proxy_resolves_device"`
}

type deviceCoverageView struct {
	Confidence        string   `json:"confidence"`
	Summary           string   `json:"summary"`
	Caveats           []string `json:"caveats"`
	DNSDeviceResolved bool     `json:"dns_device_resolved"`
	DNSGroupApplies   bool     `json:"dns_group_applies"`
	ProxyGroupApplies bool     `json:"proxy_group_applies"`
}

type devicePolicyResponse struct {
	DeviceID              string                    `json:"device_id"`
	Assignment            *deviceGroupView          `json:"assignment"`
	Identity              gatesentryPolicy.Identity `json:"identity"`
	LastSeen              time.Time                 `json:"last_seen"`
	Addresses             []deviceAddressView       `json:"addresses"`
	Coverage              deviceCoverageView        `json:"coverage"`
	MetadataEnforcesRules bool                      `json:"metadata_enforces_filtering"`
	MetadataNote          string                    `json:"metadata_note"`
}

// GSApiDevicePolicyGet reports the assignment and effective protection view
// for one device. It combines three owned sources without moving ownership:
// discovery owns the observed device record, the policy service owns identity
// resolution and enforcement, and storage owns durability.
// GET /api/devices/{id}/policy
func GSApiDevicePolicyGet(w http.ResponseWriter, r *http.Request) {
	ds := deviceStoreOrError(w)
	if ds == nil {
		return
	}
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	vars := mux.Vars(r)
	id := vars["id"]
	device := ds.GetDevice(id)
	if device == nil {
		http.Error(w, `{"error":"Device not found"}`, http.StatusNotFound)
		return
	}

	snapshot := svc.Snapshot()
	groupID := snapshot.Assignments[id]
	var assignment *deviceGroupView
	if group, ok := snapshot.Groups[groupID]; ok {
		assignment = groupView(group)
	}

	addressIPs := make([]string, 0, 2)
	if device.IPv4 != "" {
		addressIPs = append(addressIPs, device.IPv4)
	}
	if device.IPv6 != "" {
		addressIPs = append(addressIPs, device.IPv6)
	}
	addresses := make([]deviceAddressView, 0, len(addressIPs))
	for _, ip := range addressIPs {
		dnsIdentity := svc.ResolveIdentityForDNS(ip, "")
		proxyIdentity := svc.ResolveIdentity(ip, "")
		addresses = append(addresses, deviceAddressView{
			IP:                  ip,
			Source:              string(dnsIdentity.Source),
			Explanation:         dnsIdentity.Explanation,
			SharedAddress:       dnsIdentity.Source == gatesentryPolicy.SourceUnknownNAT,
			StaleObservation:    proxyIdentity.Source == gatesentryPolicy.SourceStaleDevice,
			DNSResolvesDevice:   dnsIdentity.DeviceID == id,
			ProxyResolvesDevice: proxyIdentity.DeviceID == id && proxyIdentity.GroupID == groupID,
		})
	}

	identity := gatesentryPolicy.Identity{Source: gatesentryPolicy.SourceUnknown, Explanation: "no observed address is available"}
	for _, address := range addresses {
		if address.DNSResolvesDevice {
			identity = svc.ResolveIdentityForDNS(address.IP, "")
			break
		}
	}
	if identity.Source == gatesentryPolicy.SourceUnknown && len(addresses) > 0 {
		identity = svc.ResolveIdentityForDNS(addresses[0].IP, "")
	}

	dnsResolved := false
	shared := false
	stale := false
	proxyResolved := false
	for _, address := range addresses {
		dnsResolved = dnsResolved || address.DNSResolvesDevice
		shared = shared || address.SharedAddress
		stale = stale || address.StaleObservation
		proxyResolved = proxyResolved || address.ProxyResolvesDevice
	}
	coverage := coverageView(dnsResolved, shared, stale, groupID)
	coverage.DNSDeviceResolved = dnsResolved
	coverage.ProxyGroupApplies = proxyResolved && groupID != ""

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devicePolicyResponse{
		DeviceID:              id,
		Assignment:            assignment,
		Identity:              identity,
		LastSeen:              device.LastSeen,
		Addresses:             addresses,
		Coverage:              coverage,
		MetadataEnforcesRules: false,
		MetadataNote:          "Owner and category are descriptive labels only. Only an explicit group assignment changes filtering.",
	})
}

func groupView(group gatesentryPolicy.PolicyGroup) *deviceGroupView {
	return &deviceGroupView{
		ID:                     group.ID,
		Name:                   group.Name,
		Action:                 group.Action,
		Domains:                group.Domains,
		Priority:               group.Priority,
		InapplicableConditions: gatesentryPolicy.DNSInapplicableConditions(),
	}
}

func coverageView(dnsResolved, shared, stale bool, groupID string) deviceCoverageView {
	view := deviceCoverageView{
		Caveats: []string{
			"DNS policy covers domain rules only; URL, MIME, keyword, and HTTPS-inspection rules do not apply to DNS decisions.",
			"A device is covered by DNS policy only while it uses GateSentry as its DNS resolver.",
			"Explicit proxy policy applies only to clients explicitly configured to send traffic through GateSentry.",
		},
		DNSGroupApplies: dnsResolved && groupID != "",
	}
	switch {
	case !dnsResolved && shared:
		view.Confidence = "low"
		view.Summary = "Another device shares this address, so requests from it use the default policy instead of this device's group."
	case !dnsResolved:
		view.Confidence = "none"
		view.Summary = "No current address resolves to this device, so its group assignment is not enforcing."
	case stale:
		view.Confidence = "medium"
		view.Summary = "DNS queries from this device use its assignment, but the observation is stale and proxy traffic uses the default policy."
	default:
		view.Confidence = "high"
		view.Summary = "Current DNS queries from this device resolve to it and use its assigned group."
	}
	if shared {
		view.Caveats = append(view.Caveats, "A shared or NATed address can make device identity ambiguous.")
	}
	if stale {
		view.Caveats = append(view.Caveats, "A stale observation may reflect an address that DHCP has reassigned.")
	}
	if groupID == "" {
		view.Caveats = append(view.Caveats, "No group is assigned; the gateway default policy applies.")
	}
	return view
}

type deviceAssignmentRequest struct {
	GroupID string `json:"group_id"`
}

// GSApiDeviceAssignmentSet assigns one device to one group, or clears the
// assignment when group_id is empty. It uses the policy service's per-device
// transaction rather than the replace-all assignments endpoint, so concurrent
// edits for other devices cannot be lost.
// PUT /api/devices/{id}/assignment
func GSApiDeviceAssignmentSet(w http.ResponseWriter, r *http.Request) {
	ds := deviceStoreOrError(w)
	if ds == nil {
		return
	}
	svc := policyServiceOrError(w)
	if svc == nil {
		return
	}
	vars := mux.Vars(r)
	id := vars["id"]
	if ds.GetDevice(id) == nil {
		http.Error(w, `{"error":"Device not found"}`, http.StatusNotFound)
		return
	}
	var req deviceAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	if err := svc.SetDeviceAssignment(id, req.GroupID); err != nil {
		if errors.Is(err, gatesentryPolicy.ErrUnknownGroup) {
			http.Error(w, `{"error":"Unknown policy group id"}`, http.StatusBadRequest)
			return
		}
		http.Error(w, `{"error":"Unable to persist device assignment"}`, http.StatusInternalServerError)
		return
	}
	if err := svc.Reload(); err != nil {
		http.Error(w, `{"error":"Unable to reload policy"}`, http.StatusInternalServerError)
		return
	}
	GSApiDevicePolicyGet(w, r)
}

type deviceActivityItem struct {
	Time              int64  `json:"time"`
	IP                string `json:"ip"`
	URL               string `json:"url"`
	Type              string `json:"type"`
	DNSResponseType   string `json:"dnsResponseType"`
	ProxyResponseType string `json:"proxyResponseType"`
}

// GSApiDeviceActivityGet returns decision-history entries for the device's
// current addresses. It reads the existing logger store; it never creates a
// second decision record.
// GET /api/devices/{id}/activity
func GSApiDeviceActivityGet(w http.ResponseWriter, r *http.Request, logger *gatesentryLogger.Log) {
	if logger == nil {
		http.Error(w, `{"error":"Decision logger not initialized"}`, http.StatusServiceUnavailable)
		return
	}
	ds := deviceStoreOrError(w)
	if ds == nil {
		return
	}
	vars := mux.Vars(r)
	id := vars["id"]
	device := ds.GetDevice(id)
	if device == nil {
		http.Error(w, `{"error":"Device not found"}`, http.StatusNotFound)
		return
	}
	since := int64(0)
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			http.Error(w, `{"error":"since must be a non-negative number of seconds"}`, http.StatusBadRequest)
			return
		}
		since = parsed
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			http.Error(w, `{"error":"limit must be a non-negative integer"}`, http.StatusBadRequest)
			return
		}
		limit = parsed
		if limit > 200 {
			limit = 200
		}
	}
	entries, err := logger.GetDeviceActivity([]string{device.IPv4, device.IPv6}, since, limit)
	if err != nil {
		http.Error(w, `{"error":"Unable to read device activity"}`, http.StatusInternalServerError)
		return
	}
	items := make([]deviceActivityItem, 0, len(entries))
	for _, entry := range entries {
		items = append(items, deviceActivityItem{
			Time:              entry.Time,
			IP:                entry.IP,
			URL:               entry.URL,
			Type:              entry.Type,
			DNSResponseType:   entry.DNSResponseType,
			ProxyResponseType: entry.ProxyResponseType,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"items": items,
		"total": len(items),
	})
}
