package gatesentryWebserverEndpoints

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTailscale "bitbucket.org/abdullah_irfan/gatesentryf/tailscale"
	"github.com/gorilla/mux"
)

type fakeTailscaleManager struct {
	snapshot  gatesentryTailscale.ManagerSnapshot
	peers     []gatesentryTailscale.Peer
	enableErr error
	calls     []bool
}

func (m *fakeTailscaleManager) Snapshot() gatesentryTailscale.ManagerSnapshot { return m.snapshot }
func (m *fakeTailscaleManager) Peers() []gatesentryTailscale.Peer {
	return append([]gatesentryTailscale.Peer(nil), m.peers...)
}
func (m *fakeTailscaleManager) Peer(id string) (gatesentryTailscale.Peer, bool) {
	for _, peer := range m.peers {
		if peer.NodeID == id {
			return peer, true
		}
	}
	return gatesentryTailscale.Peer{}, false
}
func (m *fakeTailscaleManager) SetEnabled(_ context.Context, enabled bool) error {
	m.calls = append(m.calls, enabled)
	m.snapshot.Enabled = enabled
	if enabled {
		m.snapshot.Status = gatesentryTailscale.StatusConnecting
	} else {
		m.snapshot.Status = gatesentryTailscale.StatusDisabled
	}
	return m.enableErr
}
func (*fakeTailscaleManager) Stop() {}

type blockingTailscaleManager struct {
	mu            sync.Mutex
	snapshot      gatesentryTailscale.ManagerSnapshot
	calls         []bool
	firstEntered  chan struct{}
	secondEntered chan struct{}
	releaseFirst  chan struct{}
	releaseOnce   sync.Once
}

func newBlockingTailscaleManager() *blockingTailscaleManager {
	return &blockingTailscaleManager{
		snapshot:      gatesentryTailscale.ManagerSnapshot{Enabled: true, Status: gatesentryTailscale.StatusConnected},
		firstEntered:  make(chan struct{}),
		secondEntered: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
}

func (m *blockingTailscaleManager) Snapshot() gatesentryTailscale.ManagerSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot
}

func (*blockingTailscaleManager) Peers() []gatesentryTailscale.Peer { return nil }
func (*blockingTailscaleManager) Peer(string) (gatesentryTailscale.Peer, bool) {
	return gatesentryTailscale.Peer{}, false
}

func (m *blockingTailscaleManager) SetEnabled(_ context.Context, enabled bool) error {
	m.mu.Lock()
	callIndex := len(m.calls)
	m.calls = append(m.calls, enabled)
	m.mu.Unlock()

	switch callIndex {
	case 0:
		close(m.firstEntered)
		<-m.releaseFirst
	case 1:
		close(m.secondEntered)
	}

	m.mu.Lock()
	m.snapshot.Enabled = enabled
	if enabled {
		m.snapshot.Status = gatesentryTailscale.StatusConnecting
	} else {
		m.snapshot.Status = gatesentryTailscale.StatusDisabled
	}
	m.mu.Unlock()
	return nil
}

func (*blockingTailscaleManager) Stop() {}

func (m *blockingTailscaleManager) release() {
	m.releaseOnce.Do(func() { close(m.releaseFirst) })
}

func (m *blockingTailscaleManager) Calls() []bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]bool(nil), m.calls...)
}

func strictTestJSON(r *http.Request, value interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("extra JSON")
	}
	return nil
}

func setupTailscaleHandlers(t *testing.T, manager gatesentryDnsServer.TailscaleManager, devices ...discovery.Device) func() {
	t.Helper()
	originalManager := gatesentryDnsServer.GetTailscaleManager()
	originalStore := gatesentryDnsServer.GetDeviceStore()
	store := discovery.NewDeviceStore("local")
	for i := range devices {
		if _, err := store.UpsertDeviceE(&devices[i]); err != nil {
			t.Fatal(err)
		}
	}
	gatesentryDnsServer.SetDeviceStoreForTests(store)
	gatesentryDnsServer.SetTailscaleManagerForTests(manager)
	return func() {
		gatesentryDnsServer.SetTailscaleManagerForTests(originalManager)
		gatesentryDnsServer.SetDeviceStoreForTests(originalStore)
	}
}

func setupEmptyPolicyService(t *testing.T) func() {
	t.Helper()
	oldBase := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(t.TempDir())
	service, err := gatesentryPolicy.NewService(gatesentry2storage.NewMapStore("tailscale-policy", false), nil)
	if err != nil {
		t.Fatal(err)
	}
	original := gatesentryDnsServer.GetPolicyService()
	gatesentryDnsServer.SetPolicyServiceForTests(service)
	return func() {
		gatesentryDnsServer.SetPolicyServiceForTests(original)
		gatesentry2storage.SetBaseDir(oldBase)
	}
}

func assertNoStore(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestTailscaleStatusIsSanitizedAndNoStore(t *testing.T) {
	manager := &fakeTailscaleManager{snapshot: gatesentryTailscale.ManagerSnapshot{
		Enabled: true, Detected: true, Status: gatesentryTailscale.StatusConnected,
		Self: &gatesentryTailscale.Peer{NodeID: "secret-node", Name: "gateway"},
	}}
	cleanup := setupTailscaleHandlers(t, manager)
	defer cleanup()
	recorder := httptest.NewRecorder()
	GSApiTailscaleStatusGET(recorder, httptest.NewRequest(http.MethodGet, "/api/tailscale/status", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	assertNoStore(t, recorder)
	if strings.Contains(recorder.Body.String(), "secret-node") {
		t.Fatalf("status leaked node ID: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"self_name":"gateway"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestTailscaleConfigUsesStrictParserAndPersists(t *testing.T) {
	manager := &fakeTailscaleManager{snapshot: gatesentryTailscale.ManagerSnapshot{Status: gatesentryTailscale.StatusDisabled}}
	cleanup := setupTailscaleHandlers(t, manager)
	defer cleanup()
	oldBase := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(t.TempDir())
	defer gatesentry2storage.SetBaseDir(oldBase)
	settings := gatesentry2storage.NewMapStore("tailscale-settings", false)

	bad := httptest.NewRecorder()
	GSApiTailscaleConfigPUT(bad, httptest.NewRequest(http.MethodPut, "/api/tailscale/config", strings.NewReader(`{"enabled":true,"extra":1}`)), settings, strictTestJSON)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad status = %d: %s", bad.Code, bad.Body.String())
	}
	assertNoStore(t, bad)
	if len(manager.calls) != 0 {
		t.Fatal("manager called for invalid request")
	}

	good := httptest.NewRecorder()
	GSApiTailscaleConfigPUT(good, httptest.NewRequest(http.MethodPut, "/api/tailscale/config", strings.NewReader(`{"enabled":true}`)), settings, strictTestJSON)
	if good.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", good.Code, good.Body.String())
	}
	assertNoStore(t, good)
	if value, err := settings.GetE(TailscaleIdentityEnabledSetting); err != nil || value != "true" {
		t.Fatalf("stored = %q, err=%v", value, err)
	}
}

func TestTailscaleConfigSerializesRollbackBeforeNewerRequest(t *testing.T) {
	manager := newBlockingTailscaleManager()
	defer manager.release()
	cleanup := setupTailscaleHandlers(t, manager)
	defer cleanup()

	oldBase := gatesentry2storage.GSBASEDIR
	defer gatesentry2storage.SetBaseDir(oldBase)
	invalidBase := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(invalidBase, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	gatesentry2storage.SetBaseDir(invalidBase)
	failingSettings := gatesentry2storage.NewMapStore("tailscale-settings", false)
	gatesentry2storage.SetBaseDir(t.TempDir())
	successfulSettings := gatesentry2storage.NewMapStore("tailscale-settings", false)

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/tailscale/config", strings.NewReader(`{"enabled":false}`))
		GSApiTailscaleConfigPUT(recorder, request, failingSettings, strictTestJSON)
		firstDone <- recorder
	}()

	select {
	case <-manager.firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first config request did not enter manager transition")
	}

	secondParsed := make(chan struct{})
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/tailscale/config", strings.NewReader(`{"enabled":false}`))
		GSApiTailscaleConfigPUT(recorder, request, successfulSettings, func(r *http.Request, value interface{}) error {
			err := strictTestJSON(r, value)
			close(secondParsed)
			return err
		})
		secondDone <- recorder
	}()

	select {
	case <-secondParsed:
	case <-time.After(time.Second):
		t.Fatal("second config request was not parsed")
	}
	select {
	case <-manager.secondEntered:
		t.Fatal("another manager transition ran while the first request was in progress")
	case <-time.After(100 * time.Millisecond):
	}

	manager.release()
	var first, second *httptest.ResponseRecorder
	select {
	case first = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first config request did not complete")
	}
	select {
	case second = <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second config request did not complete")
	}
	if first.Code != http.StatusInternalServerError || second.Code != http.StatusOK {
		t.Fatalf("statuses = %d, %d; bodies = %q, %q", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	if got := manager.Snapshot().Enabled; got {
		t.Fatal("failed request rollback overwrote the newer successful disable")
	}
	if value, err := successfulSettings.GetE(TailscaleIdentityEnabledSetting); err != nil || value != "false" {
		t.Fatalf("stored = %q, err=%v", value, err)
	}
	calls := manager.Calls()
	if len(calls) != 3 || calls[0] || !calls[1] || calls[2] {
		t.Fatalf("manager calls = %v, want [false true false]", calls)
	}
}

func TestTailscaleConfigPersistenceFailureRestoresActualPriorState(t *testing.T) {
	manager := &fakeTailscaleManager{snapshot: gatesentryTailscale.ManagerSnapshot{
		Enabled: true, Status: gatesentryTailscale.StatusConnected,
	}}
	cleanup := setupTailscaleHandlers(t, manager)
	defer cleanup()

	oldBase := gatesentry2storage.GSBASEDIR
	invalidBase := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(invalidBase, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	gatesentry2storage.SetBaseDir(invalidBase)
	defer gatesentry2storage.SetBaseDir(oldBase)
	settings := gatesentry2storage.NewMapStore("tailscale-settings", false)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/tailscale/config", strings.NewReader(`{"enabled":true}`))
	GSApiTailscaleConfigPUT(recorder, request, settings, strictTestJSON)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	assertNoStore(t, recorder)
	if recorder.Body.String() != "{\"error\":\"Unable to save Tailscale configuration\"}\n" {
		t.Fatalf("unsanitized error body = %q", recorder.Body.String())
	}
	if !manager.snapshot.Enabled {
		t.Fatal("manager did not restore its enabled state")
	}
	if len(manager.calls) != 2 || !manager.calls[0] || !manager.calls[1] {
		t.Fatalf("manager calls = %v, want [true true]", manager.calls)
	}
}

func TestTailscalePeersSuggestExactHostnameOnly(t *testing.T) {
	peers := []gatesentryTailscale.Peer{
		{NodeID: "node-host", Name: "laptop", Addresses: []string{"100.64.0.1"}},
		{NodeID: "node-address", Name: "different", Addresses: []string{"192.0.2.20"}},
	}
	manager := &fakeTailscaleManager{
		snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Status: gatesentryTailscale.StatusConnected, Peers: peers},
		peers:    peers,
	}
	cleanup := setupTailscaleHandlers(t, manager,
		discovery.Device{ID: "hostname-device", Hostnames: []string{"laptop"}, IPv4: "192.0.2.10", LastSeen: time.Now()},
		discovery.Device{ID: "address-device", Hostnames: []string{"desktop"}, IPv4: "192.0.2.20", LastSeen: time.Now()},
	)
	defer cleanup()
	recorder := httptest.NewRecorder()
	GSApiTailscalePeersGET(recorder, httptest.NewRequest(http.MethodGet, "/api/tailscale/peers", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	assertNoStore(t, recorder)
	var body struct {
		Peers []tailscalePeerResponse `json:"peers"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Peers[0].Suggestion == nil || body.Peers[0].Suggestion.DeviceID != "hostname-device" {
		t.Fatalf("hostname suggestion = %+v", body.Peers[0].Suggestion)
	}
	if body.Peers[1].Suggestion != nil {
		t.Fatalf("address-only suggestion = %+v", body.Peers[1].Suggestion)
	}
}

func TestTailscalePeersReportsOrdinaryNonConnectedStates(t *testing.T) {
	tests := []struct {
		name     string
		snapshot gatesentryTailscale.ManagerSnapshot
		message  string
	}{
		{name: "disabled", snapshot: gatesentryTailscale.ManagerSnapshot{Status: gatesentryTailscale.StatusDisabled}, message: "Tailscale identity collection is disabled."},
		{name: "connecting", snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Detected: true, Status: gatesentryTailscale.StatusConnecting}, message: "Connecting to Tailscale."},
		{name: "unavailable", snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Status: gatesentryTailscale.StatusUnavailable}, message: "Tailscale is not currently available."},
		{name: "degraded", snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Detected: true, Status: gatesentryTailscale.StatusDegraded, Peers: []gatesentryTailscale.Peer{{NodeID: "stale-node"}}}, message: "Tailscale identity data may be stale."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeTailscaleManager{snapshot: test.snapshot, peers: test.snapshot.Peers}
			cleanup := setupTailscaleHandlers(t, manager)
			defer cleanup()
			recorder := httptest.NewRecorder()
			GSApiTailscalePeersGET(recorder, httptest.NewRequest(http.MethodGet, "/api/tailscale/peers", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			assertNoStore(t, recorder)
			var response tailscalePeerListResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.State != string(test.snapshot.Status) || response.Message != test.message {
				t.Fatalf("state response = %+v", response.tailscaleStatusResponse)
			}
			if response.Peers == nil || len(response.Peers) != 0 {
				t.Fatalf("peers = %+v, want a present empty list", response.Peers)
			}
			if strings.Contains(recorder.Body.String(), "stale-node") {
				t.Fatalf("non-connected response exposed stale peers: %s", recorder.Body.String())
			}
		})
	}
}

func TestTailscaleLinkRejectsDisabledAndDegradedManager(t *testing.T) {
	for _, test := range []struct {
		name     string
		snapshot gatesentryTailscale.ManagerSnapshot
		want     int
	}{
		{name: "disabled", snapshot: gatesentryTailscale.ManagerSnapshot{Status: gatesentryTailscale.StatusDisabled}, want: http.StatusConflict},
		{name: "degraded", snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Status: gatesentryTailscale.StatusDegraded}, want: http.StatusServiceUnavailable},
		{name: "unavailable", snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Status: gatesentryTailscale.StatusUnavailable}, want: http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakeTailscaleManager{snapshot: test.snapshot}
			cleanup := setupTailscaleHandlers(t, manager, discovery.Device{ID: "device-1", IPv4: "192.0.2.1"})
			defer cleanup()
			request := httptest.NewRequest(http.MethodPut, "/api/devices/device-1/tailscale", strings.NewReader(`{"node_id":"node-1"}`))
			request = mux.SetURLVars(request, map[string]string{"id": "device-1"})
			recorder := httptest.NewRecorder()
			GSApiDeviceTailscaleLinkPUT(recorder, request, strictTestJSON)
			if recorder.Code != test.want {
				t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
			}
			assertNoStore(t, recorder)
		})
	}
}

func TestTailscaleLinkAndUnlink(t *testing.T) {
	manager := &fakeTailscaleManager{snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Status: gatesentryTailscale.StatusConnected}, peers: []gatesentryTailscale.Peer{{NodeID: "node-1", Name: "laptop", Addresses: []string{"100.64.0.1"}}}}
	cleanup := setupTailscaleHandlers(t, manager, discovery.Device{ID: "device-1", Hostnames: []string{"lan-laptop"}, IPv4: "192.0.2.10", LastSeen: time.Now()})
	defer cleanup()
	cleanupPolicy := setupEmptyPolicyService(t)
	defer cleanupPolicy()

	linkRequest := httptest.NewRequest(http.MethodPut, "/api/devices/device-1/tailscale", strings.NewReader(`{"node_id":"node-1"}`))
	linkRequest = mux.SetURLVars(linkRequest, map[string]string{"id": "device-1"})
	linked := httptest.NewRecorder()
	GSApiDeviceTailscaleLinkPUT(linked, linkRequest, strictTestJSON)
	if linked.Code != http.StatusOK {
		t.Fatalf("link status = %d: %s", linked.Code, linked.Body.String())
	}
	assertNoStore(t, linked)
	if got := gatesentryDnsServer.GetDeviceStore().FindDeviceByTailscaleNode("node-1"); got == nil || got.ID != "device-1" {
		t.Fatalf("linked device = %+v", got)
	}

	unlinkRequest := httptest.NewRequest(http.MethodDelete, "/api/devices/device-1/tailscale/node-1", nil)
	unlinkRequest = mux.SetURLVars(unlinkRequest, map[string]string{"id": "device-1", "nodeID": "node-1"})
	unlinked := httptest.NewRecorder()
	GSApiDeviceTailscaleUnlinkDELETE(unlinked, unlinkRequest)
	if unlinked.Code != http.StatusOK {
		t.Fatalf("unlink status = %d: %s", unlinked.Code, unlinked.Body.String())
	}
	assertNoStore(t, unlinked)
	if got := gatesentryDnsServer.GetDeviceStore().FindDeviceByTailscaleNode("node-1"); got != nil {
		t.Fatalf("link remains: %+v", got)
	}
}

func TestTailscaleLinkRejectsPolicyReferencedAddressClaimant(t *testing.T) {
	manager := &fakeTailscaleManager{snapshot: gatesentryTailscale.ManagerSnapshot{Enabled: true, Status: gatesentryTailscale.StatusConnected}, peers: []gatesentryTailscale.Peer{{NodeID: "node-1", Addresses: []string{"100.64.0.1"}}}}
	cleanup := setupTailscaleHandlers(t, manager,
		discovery.Device{ID: "target", IPv4: "192.0.2.10", LastSeen: time.Now()},
		discovery.Device{ID: "protected", IPv4: "100.64.0.1", LastSeen: time.Now()},
	)
	defer cleanup()
	oldBase := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(t.TempDir())
	defer gatesentry2storage.SetBaseDir(oldBase)
	settings := gatesentry2storage.NewMapStore("policy", false)
	svc, err := gatesentryPolicy.NewService(settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{ID: "kids", Name: "Kids"}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDeviceAssignment("protected", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
	oldPolicy := gatesentryDnsServer.GetPolicyService()
	gatesentryDnsServer.SetPolicyServiceForTests(svc)
	defer gatesentryDnsServer.SetPolicyServiceForTests(oldPolicy)

	request := httptest.NewRequest(http.MethodPut, "/api/devices/target/tailscale", strings.NewReader(`{"node_id":"node-1"}`))
	request = mux.SetURLVars(request, map[string]string{"id": "target"})
	recorder := httptest.NewRecorder()
	GSApiDeviceTailscaleLinkPUT(recorder, request, strictTestJSON)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	assertNoStore(t, recorder)
}
