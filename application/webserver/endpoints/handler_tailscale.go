package gatesentryWebserverEndpoints

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTailscale "bitbucket.org/abdullah_irfan/gatesentryf/tailscale"
	"github.com/gorilla/mux"
)

const TailscaleIdentityEnabledSetting = "tailscale_identity_enabled"

var tailscaleConfigMu sync.Mutex

type JSONRequestParser func(*http.Request, interface{}) error

type tailscaleStatusResponse struct {
	Enabled      bool      `json:"enabled"`
	Detected     bool      `json:"detected"`
	State        string    `json:"state"`
	BackendState string    `json:"backend_state"`
	SelfName     string    `json:"self_name"`
	LastRefresh  time.Time `json:"last_refresh,omitempty"`
	Message      string    `json:"message,omitempty"`
}

type tailscaleSuggestion struct {
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
	Confidence string `json:"confidence"`
}

type tailscalePeerListResponse struct {
	tailscaleStatusResponse
	Peers []tailscalePeerResponse `json:"peers"`
}

type tailscalePeerResponse struct {
	NodeID         string               `json:"node_id"`
	Name           string               `json:"name,omitempty"`
	DNSName        string               `json:"dns_name,omitempty"`
	Addresses      []string             `json:"addresses,omitempty"`
	Online         bool                 `json:"online"`
	LastSeen       time.Time            `json:"last_seen,omitempty"`
	MappedDeviceID string               `json:"mapped_device_id,omitempty"`
	Suggestion     *tailscaleSuggestion `json:"suggestion,omitempty"`
	WoLMACs        []string             `json:"wol_macs,omitempty"`
}

func tailscaleNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
}

func tailscaleError(w http.ResponseWriter, status int, message string) {
	tailscaleNoStore(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func tailscaleManagerOrError(w http.ResponseWriter) gatesentryDnsServer.TailscaleManager {
	manager := gatesentryDnsServer.GetTailscaleManager()
	if manager == nil {
		tailscaleError(w, http.StatusServiceUnavailable, "Tailscale integration is not initialized")
	}
	return manager
}

func tailscaleStatus(manager gatesentryDnsServer.TailscaleManager) tailscaleStatusResponse {
	return tailscaleStatusFromSnapshot(manager.Snapshot())
}

func tailscaleStatusFromSnapshot(snapshot gatesentryTailscale.ManagerSnapshot) tailscaleStatusResponse {
	response := tailscaleStatusResponse{
		Enabled: snapshot.Enabled, Detected: snapshot.Detected,
		State: string(snapshot.Status), LastRefresh: snapshot.LastRefresh,
	}
	if snapshot.Self != nil {
		response.SelfName = firstNonEmpty(snapshot.Self.Name, snapshot.Self.DNSName)
	}
	switch snapshot.Status {
	case gatesentryTailscale.StatusConnected:
		response.BackendState = "Running"
		response.Message = "Tailscale identity data is current."
	case gatesentryTailscale.StatusDegraded:
		response.BackendState = "Unavailable"
		response.Message = "Tailscale identity data may be stale."
	case gatesentryTailscale.StatusUnavailable:
		response.BackendState = "Unavailable"
		response.Message = "Tailscale is not currently available."
	case gatesentryTailscale.StatusDisabled:
		response.BackendState = "Disabled"
		response.Message = "Tailscale identity collection is disabled."
	case gatesentryTailscale.StatusStopped:
		response.BackendState = "Stopped"
		response.Message = "Tailscale identity collection is stopped."
	default:
		response.BackendState = "Connecting"
		response.Message = "Connecting to Tailscale."
	}
	return response
}

// GSApiTailscaleStatusGET reports only finite, sanitized manager state.
func GSApiTailscaleStatusGET(w http.ResponseWriter, _ *http.Request) {
	manager := tailscaleManagerOrError(w)
	if manager == nil {
		return
	}
	tailscaleNoStore(w)
	_ = json.NewEncoder(w).Encode(tailscaleStatus(manager))
}

// GSApiTailscaleConfigPUT enables or disables collection and persists the
// disabled-by-default setting. A failed refresh still leaves collection enabled
// so the manager can recover automatically when tailscaled becomes available.
func GSApiTailscaleConfigPUT(w http.ResponseWriter, r *http.Request, settings *gatesentry2storage.MapStore, parse JSONRequestParser) {
	tailscaleNoStore(w)
	manager := tailscaleManagerOrError(w)
	if manager == nil {
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if parse == nil || parse(r, &request) != nil || request.Enabled == nil {
		tailscaleError(w, http.StatusBadRequest, "Invalid Tailscale configuration")
		return
	}
	if settings == nil {
		tailscaleError(w, http.StatusServiceUnavailable, "Settings storage is unavailable")
		return
	}
	enabled := *request.Enabled
	tailscaleConfigMu.Lock()
	defer tailscaleConfigMu.Unlock()

	previousEnabled := manager.Snapshot().Enabled
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	err := manager.SetEnabled(ctx, enabled)
	cancel()
	if err != nil && !enabled {
		tailscaleError(w, http.StatusServiceUnavailable, "Unable to disable Tailscale integration")
		return
	}
	value := "false"
	if enabled {
		value = "true"
	}
	if persistErr := settings.Update(TailscaleIdentityEnabledSetting, value); persistErr != nil {
		rollback, rollbackCancel := context.WithTimeout(context.Background(), 6*time.Second)
		_ = manager.SetEnabled(rollback, previousEnabled)
		rollbackCancel()
		tailscaleError(w, http.StatusInternalServerError, "Unable to save Tailscale configuration")
		return
	}
	_ = json.NewEncoder(w).Encode(tailscaleStatus(manager))
}

// GSApiTailscalePeersGET lists safe peer metadata and explicit mappings.
// Suggestions use exact hostname equality only; addresses and WoL MACs are
// informational and never used to infer identity.
func GSApiTailscalePeersGET(w http.ResponseWriter, _ *http.Request) {
	tailscaleNoStore(w)
	manager := tailscaleManagerOrError(w)
	if manager == nil {
		return
	}
	snapshot := manager.Snapshot()
	response := tailscalePeerListResponse{
		tailscaleStatusResponse: tailscaleStatusFromSnapshot(snapshot),
		Peers:                   []tailscalePeerResponse{},
	}

	// Disabled, connecting, and unavailable are ordinary runtime states, not
	// request failures. Returning their sanitized state lets clients render an
	// accurate empty state rather than surfacing an error or claiming no peers
	// exist. A degraded snapshot may contain last-known peers, but those peers
	// are deliberately not offered for linking while the manager is unhealthy.
	if snapshot.Status != gatesentryTailscale.StatusConnected {
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	store := gatesentryDnsServer.GetDeviceStore()
	if store == nil {
		tailscaleError(w, http.StatusServiceUnavailable, "Device store is unavailable")
		return
	}
	peers := snapshot.Peers
	response.Peers = make([]tailscalePeerResponse, 0, len(peers))
	devices := store.GetAllDevices()
	for _, peer := range peers {
		item := tailscalePeerResponse{
			NodeID: peer.NodeID, Name: peer.Name, DNSName: peer.DNSName,
			Addresses: append([]string(nil), peer.Addresses...), Online: peer.Online,
			LastSeen: peer.LastSeen, WoLMACs: append([]string(nil), peer.WoLMACs...),
		}
		if mapped := store.FindDeviceByTailscaleNode(peer.NodeID); mapped != nil {
			item.MappedDeviceID = mapped.ID
		} else {
			item.Suggestion = exactHostnameSuggestion(peer, devices)
		}
		response.Peers = append(response.Peers, item)
	}
	_ = json.NewEncoder(w).Encode(response)
}

func exactHostnameSuggestion(peer gatesentryTailscale.Peer, devices []discovery.Device) *tailscaleSuggestion {
	peerNames := map[string]struct{}{}
	if name := normalizeSuggestedHostname(peer.Name); name != "" {
		peerNames[name] = struct{}{}
	}
	if dnsName := normalizeSuggestedHostname(peer.DNSName); dnsName != "" {
		peerNames[dnsName] = struct{}{}
		if dot := strings.IndexByte(dnsName, '.'); dot > 0 {
			peerNames[dnsName[:dot]] = struct{}{}
		}
	}
	if len(peerNames) == 0 {
		return nil
	}
	var match *discovery.Device
	for i := range devices {
		matched := false
		for _, value := range append(append([]string{devices[i].DNSName}, devices[i].Hostnames...), devices[i].MDNSNames...) {
			_, matched = peerNames[normalizeSuggestedHostname(value)]
			if matched {
				break
			}
		}
		if !matched {
			continue
		}
		if match != nil {
			return nil
		}
		match = &devices[i]
	}
	if match == nil {
		return nil
	}
	return &tailscaleSuggestion{DeviceID: match.ID, Name: match.GetDisplayName(), Reason: "Exact hostname match", Confidence: "high"}
}

func normalizeSuggestedHostname(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, ".")
	value = strings.TrimSuffix(value, ".local")
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// GSApiDeviceTailscaleLinkPUT explicitly links one current peer to a canonical
// device. It never accepts peer metadata from the caller.
func GSApiDeviceTailscaleLinkPUT(w http.ResponseWriter, r *http.Request, parse JSONRequestParser) {
	tailscaleNoStore(w)
	manager := tailscaleManagerOrError(w)
	if manager == nil {
		return
	}
	snapshot := manager.Snapshot()
	if !snapshot.Enabled {
		tailscaleError(w, http.StatusConflict, "Tailscale integration is disabled")
		return
	}
	if snapshot.Status != gatesentryTailscale.StatusConnected {
		tailscaleError(w, http.StatusServiceUnavailable, "Tailscale peer data is unavailable")
		return
	}
	store := gatesentryDnsServer.GetDeviceStore()
	if store == nil {
		tailscaleError(w, http.StatusServiceUnavailable, "Device store is unavailable")
		return
	}
	deviceID := mux.Vars(r)["id"]
	if store.GetDevice(deviceID) == nil {
		tailscaleError(w, http.StatusNotFound, "Device not found")
		return
	}
	var request struct {
		NodeID string `json:"node_id"`
	}
	if parse == nil || parse(r, &request) != nil || strings.TrimSpace(request.NodeID) == "" {
		tailscaleError(w, http.StatusBadRequest, "Invalid Tailscale link request")
		return
	}
	peer, ok := manager.Peer(strings.TrimSpace(request.NodeID))
	if !ok {
		tailscaleError(w, http.StatusNotFound, "Tailscale peer not found")
		return
	}
	service := gatesentryDnsServer.GetPolicyService()
	if service == nil {
		tailscaleError(w, http.StatusServiceUnavailable, "Policy protection is unavailable")
		return
	}
	updated, err := store.LinkTailscaleIdentityProtectedE(deviceID, discovery.TailscaleIdentity{
		NodeID: peer.NodeID, Name: peer.Name, DNSName: peer.DNSName,
		Addresses: append([]string(nil), peer.Addresses...), Online: peer.Online,
		LastSeen: peer.LastSeen, WoLMACs: append([]string(nil), peer.WoLMACs...),
	}, service.ReferencedDeviceIDs())
	if errors.Is(err, discovery.ErrTailscaleIdentityConflict) {
		tailscaleError(w, http.StatusConflict, "Tailscale peer conflicts with another device")
		return
	}
	if err != nil {
		tailscaleError(w, http.StatusInternalServerError, "Unable to persist Tailscale link")
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"device": updated})
}

// GSApiDeviceTailscaleUnlinkDELETE removes only the explicit overlay link.
func GSApiDeviceTailscaleUnlinkDELETE(w http.ResponseWriter, r *http.Request) {
	tailscaleNoStore(w)
	store := gatesentryDnsServer.GetDeviceStore()
	if store == nil {
		tailscaleError(w, http.StatusServiceUnavailable, "Device store is unavailable")
		return
	}
	updated, err := store.UnlinkTailscaleIdentityE(mux.Vars(r)["id"], mux.Vars(r)["nodeID"])
	if err != nil {
		if store.GetDevice(mux.Vars(r)["id"]) == nil {
			tailscaleError(w, http.StatusNotFound, "Device not found")
		} else {
			tailscaleError(w, http.StatusNotFound, "Tailscale link not found")
		}
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"device": updated})
}
