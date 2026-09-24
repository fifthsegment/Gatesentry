package gatesentryWebserverEndpoints

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	"github.com/gorilla/mux"
	"github.com/tidwall/buntdb"
)

func devicePolicyTestSetup(t *testing.T) (func(), *gatesentryLogger.Log) {
	t.Helper()
	_, cleanupPolicy := policyTestService(t)

	originalDevices := gatesentryDnsServer.GetDeviceStore()
	ds := discovery.NewDeviceStore("local")
	if _, err := ds.UpsertDeviceE(&discovery.Device{
		ID:        "device-1",
		Hostnames: []string{"laptop"},
		IPv4:      "192.0.2.10",
		Source:    discovery.SourceManual,
		Sources:   []discovery.DiscoverySource{discovery.SourceManual},
	}); err != nil {
		t.Fatal(err)
	}
	gatesentryDnsServer.SetDeviceStoreForTests(ds)

	log := gatesentryLogger.NewLogger(t.TempDir() + "/device-activity.db")
	return func() {
		_ = log.Database.Close()
		gatesentryDnsServer.SetDeviceStoreForTests(originalDevices)
		cleanupPolicy()
	}, log
}

func seedDevicePolicyGroup(t *testing.T, svc *gatesentryPolicy.Service) {
	t.Helper()
	if err := svc.SaveGroups([]gatesentryPolicy.PolicyGroup{{
		ID:             "kids",
		Name:           "Kids",
		BlockedDomains: []string{"*.games.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}
}

func deviceRequest(method, path, id, query, body string) (*http.Request, *httptest.ResponseRecorder) {
	var reader *bytes.Buffer
	if body == "" {
		reader = bytes.NewBuffer(nil)
	} else {
		reader = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path+query, reader)
	req = mux.SetURLVars(req, map[string]string{"id": id})
	return req, httptest.NewRecorder()
}

func TestDevicePolicyViewReportsAssignmentAndCoverage(t *testing.T) {
	cleanup, _ := devicePolicyTestSetup(t)
	defer cleanup()
	svc := gatesentryDnsServer.GetPolicyService()
	seedDevicePolicyGroup(t, svc)
	if err := svc.SetDeviceAssignment("device-1", "kids"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Reload(); err != nil {
		t.Fatal(err)
	}

	req, recorder := deviceRequest(http.MethodGet, "/api/devices/device-1/policy", "device-1", "", "")
	GSApiDevicePolicyGet(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		DeviceID   string `json:"device_id"`
		Assignment *struct {
			ID             string   `json:"id"`
			BlockedDomains []string `json:"blocked_domains"`
		} `json:"assignment"`
		Identity struct {
			DeviceID    string `json:"device_id"`
			GroupID     string `json:"group_id"`
			Source      string `json:"source"`
			Explanation string `json:"explanation"`
		} `json:"identity"`
		LastSeen  time.Time `json:"last_seen"`
		Addresses []struct {
			IP                string `json:"ip"`
			DNSResolvesDevice bool   `json:"dns_resolves_device"`
			SharedAddress     bool   `json:"shared_address"`
			StaleObservation  bool   `json:"stale_observation"`
		} `json:"addresses"`
		Coverage struct {
			Confidence      string   `json:"confidence"`
			Summary         string   `json:"summary"`
			Caveats         []string `json:"caveats"`
			DNSGroupApplies bool     `json:"dns_group_applies"`
		} `json:"coverage"`
		MetadataEnforcesRules bool   `json:"metadata_enforces_filtering"`
		MetadataNote          string `json:"metadata_note"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.DeviceID != "device-1" {
		t.Fatalf("device id = %q", body.DeviceID)
	}
	if body.Assignment == nil ||
		body.Assignment.ID != "kids" ||
		len(body.Assignment.BlockedDomains) != 1 ||
		body.Assignment.BlockedDomains[0] != "*.games.example" {
		t.Fatalf("assignment = %+v", body.Assignment)
	}
	if body.Identity.DeviceID != "device-1" ||
		body.Identity.GroupID != "kids" ||
		body.Identity.Source != string(gatesentryPolicy.SourceDevice) ||
		body.Identity.Explanation == "" {
		t.Fatalf("identity = %+v", body.Identity)
	}
	if body.LastSeen.IsZero() {
		t.Fatal("last_seen missing")
	}
	if len(body.Addresses) != 1 ||
		body.Addresses[0].IP != "192.0.2.10" ||
		!body.Addresses[0].DNSResolvesDevice ||
		body.Addresses[0].SharedAddress ||
		body.Addresses[0].StaleObservation {
		t.Fatalf("addresses = %+v", body.Addresses)
	}
	if body.Coverage.Confidence != "high" ||
		body.Coverage.Summary == "" ||
		!body.Coverage.DNSGroupApplies ||
		len(body.Coverage.Caveats) == 0 {
		t.Fatalf("coverage = %+v", body.Coverage)
	}
	if body.MetadataEnforcesRules {
		t.Fatal("metadata must never be reported as enforcing filtering")
	}
	if body.MetadataNote == "" {
		t.Fatal("metadata note missing")
	}
}

func TestDevicePolicyViewWithoutAssignmentUsesDefault(t *testing.T) {
	cleanup, _ := devicePolicyTestSetup(t)
	defer cleanup()
	svc := gatesentryDnsServer.GetPolicyService()
	seedDevicePolicyGroup(t, svc)

	req, recorder := deviceRequest(http.MethodGet, "/api/devices/device-1/policy", "device-1", "", "")
	GSApiDevicePolicyGet(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Assignment *struct {
			ID string `json:"id"`
		} `json:"assignment"`
		EffectivePolicy *struct {
			ID      string `json:"id"`
			Default bool   `json:"default"`
		} `json:"effective_policy"`
		Coverage struct {
			Confidence string   `json:"confidence"`
			Caveats    []string `json:"caveats"`
		} `json:"coverage"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Assignment != nil {
		t.Fatalf("assignment = %+v, want null", body.Assignment)
	}
	if body.EffectivePolicy == nil || body.EffectivePolicy.ID != gatesentryPolicy.DefaultGroupID || !body.EffectivePolicy.Default {
		t.Fatalf("effective policy = %+v, want the default policy", body.EffectivePolicy)
	}
	if body.Coverage.Confidence != "high" {
		t.Fatalf("confidence = %q, want high (identity resolves; only the group is absent)", body.Coverage.Confidence)
	}
	found := false
	for _, caveat := range body.Coverage.Caveats {
		if caveat == "No policy is assigned; the default policy applies." {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing default-policy caveat: %v", body.Coverage.Caveats)
	}
}

func TestDeviceAssignmentSetClearAndUnknownGroup(t *testing.T) {
	cleanup, _ := devicePolicyTestSetup(t)
	defer cleanup()
	svc := gatesentryDnsServer.GetPolicyService()
	seedDevicePolicyGroup(t, svc)

	put := func(id, body string) *httptest.ResponseRecorder {
		req, recorder := deviceRequest(http.MethodPut, "/api/devices/"+id+"/assignment", id, "", body)
		GSApiDeviceAssignmentSet(recorder, req)
		return recorder
	}

	recorder := put("device-1", `{"group_id":"kids"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("set status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var set struct {
		Assignment *struct {
			ID string `json:"id"`
		} `json:"assignment"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &set); err != nil {
		t.Fatal(err)
	}
	if set.Assignment == nil || set.Assignment.ID != "kids" {
		t.Fatalf("set response assignment = %+v", set.Assignment)
	}
	if got := svc.Snapshot().Assignments["device-1"]; got != "kids" {
		t.Fatalf("snapshot assignment = %q, want kids", got)
	}

	recorder = put("device-1", `{"group_id":"missing"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown group status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = put("device-1", `{"group_id":""}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("clear status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var cleared struct {
		Assignment *struct {
			ID string `json:"id"`
		} `json:"assignment"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.Assignment != nil {
		t.Fatalf("cleared assignment = %+v, want null", cleared.Assignment)
	}
	if _, exists := svc.Snapshot().Assignments["device-1"]; exists {
		t.Fatal("cleared assignment still in snapshot")
	}
	if got := gatesentryDnsServer.GetDeviceStore().GetDevice("device-1"); got == nil || !got.Persistent {
		t.Fatalf("clearing assignment discarded durable identity: %+v", got)
	}

	recorder = put("missing-device", `{"group_id":"kids"}`)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown device status = %d", recorder.Code)
	}
}

type testDeviceResolver struct {
	store *discovery.DeviceStore
}

func (r testDeviceResolver) ResolveDeviceByIP(ip string) (string, bool, bool) {
	deviceID, ambiguous, _, _ := r.store.ResolveIPClaim(ip)
	return deviceID, ambiguous, false
}

func TestDeviceAssignmentPersistsPassiveIdentityAcrossRestart(t *testing.T) {
	_, cleanupPolicy := policyTestService(t)
	defer cleanupPolicy()
	svc := gatesentryDnsServer.GetPolicyService()
	seedDevicePolicyGroup(t, svc)
	devices, err := gatesentry2storage.OpenMapStore("GSDevices", true)
	if err != nil {
		t.Fatal(err)
	}
	ds := discovery.NewDeviceStore("local")
	if err := ds.AttachPersistence(devices); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&discovery.Device{
		ID: "phone", IPv4: "192.0.2.10", MACs: []string{"aa:bb:cc:dd:ee:ff"},
		Source: discovery.SourcePassive, Sources: []discovery.DiscoverySource{discovery.SourcePassive},
	}); err != nil {
		t.Fatal(err)
	}
	gatesentryDnsServer.SetDeviceStoreForTests(ds)

	req, recorder := deviceRequest(http.MethodPut, "/api/devices/phone/assignment", "phone", "", `{"group_id":"kids"}`)
	GSApiDeviceAssignmentSet(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("assignment status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	restarted := discovery.NewDeviceStore("local")
	if err := restarted.AttachPersistence(devices); err != nil {
		t.Fatal(err)
	}
	if got := restarted.GetDevice("phone"); got == nil || !got.Persistent || got.IPv4 != "" {
		t.Fatalf("restored device = %+v", got)
	}
	id, created, err := restarted.ObserveDevice(discovery.Device{
		IPv4: "192.0.2.44", MACs: []string{"aa:bb:cc:dd:ee:ff"},
		Source: discovery.SourcePassive, Sources: []discovery.DiscoverySource{discovery.SourcePassive},
	})
	if err != nil || created || id != "phone" {
		t.Fatalf("rediscovery = id %q created=%v err=%v", id, created, err)
	}
	if got := restarted.FindDeviceByIP("192.0.2.44"); got == nil || got.ID != "phone" {
		t.Fatalf("address did not resolve to restored identity: %+v", got)
	}
	settings, err := gatesentry2storage.OpenMapStore("settings", false)
	if err != nil {
		t.Fatal(err)
	}
	restartedPolicy, err := gatesentryPolicy.NewService(settings, testDeviceResolver{store: restarted})
	if err != nil {
		t.Fatal(err)
	}
	identity := restartedPolicy.ResolveIdentityForDNS("192.0.2.44", "")
	if identity.DeviceID != "phone" || identity.GroupID != "kids" {
		t.Fatalf("identity after restart = %+v", identity)
	}
}

func TestDeviceAssignmentPersistenceFailureDoesNotCommitPolicy(t *testing.T) {
	_, cleanupPolicy := policyTestService(t)
	base := gatesentry2storage.GSBASEDIR
	defer cleanupPolicy()
	svc := gatesentryDnsServer.GetPolicyService()
	seedDevicePolicyGroup(t, svc)
	devices, err := gatesentry2storage.OpenMapStore("GSDevices", true)
	if err != nil {
		t.Fatal(err)
	}
	ds := discovery.NewDeviceStore("local")
	if err := ds.AttachPersistence(devices); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.UpsertDeviceE(&discovery.Device{ID: "phone", IPv4: "192.0.2.10", Source: discovery.SourcePassive}); err != nil {
		t.Fatal(err)
	}
	gatesentryDnsServer.SetDeviceStoreForTests(ds)
	if err := os.RemoveAll(base); err != nil {
		t.Fatal(err)
	}

	req, recorder := deviceRequest(http.MethodPut, "/api/devices/phone/assignment", "phone", "", `{"group_id":"kids"}`)
	GSApiDeviceAssignmentSet(recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("assignment status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if _, exists := svc.Snapshot().Assignments["phone"]; exists {
		t.Fatal("policy assignment committed after device persistence failed")
	}
	if got := ds.GetDevice("phone"); got == nil || got.Persistent {
		t.Fatalf("failed persistence changed device = %+v", got)
	}
}

func insertDeviceActivity(t *testing.T, log *gatesentryLogger.Log, ip, url string) {
	t.Helper()
	at := time.Now().Unix()
	value := fmt.Sprintf(`{"time": %d, "ip":%q, "url":%q, "type":"dns", "dnsResponseType":"blocked"}`, at, ip, url)
	err := log.Database.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set("activity-"+url, value, nil)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeviceActivityEndpoint(t *testing.T) {
	cleanup, log := devicePolicyTestSetup(t)
	defer cleanup()
	insertDeviceActivity(t, log, "192.0.2.10", "games.example")
	insertDeviceActivity(t, log, "198.51.100.7", "other-client.example")

	req, recorder := deviceRequest(http.MethodGet, "/api/devices/device-1/activity", "device-1", "", "")
	GSApiDeviceActivityGet(recorder, req, log)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []struct {
			IP   string `json:"ip"`
			URL  string `json:"url"`
			Type string `json:"type"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 1 || len(body.Items) != 1 {
		t.Fatalf("activity = %+v, want only this device's entries", body)
	}
	if body.Items[0].IP != "192.0.2.10" || body.Items[0].URL != "games.example" || body.Items[0].Type != "dns" {
		t.Fatalf("activity item = %+v", body.Items[0])
	}

	req, recorder = deviceRequest(http.MethodGet, "/api/devices/device-1/activity", "device-1", "?since=abc", "")
	GSApiDeviceActivityGet(recorder, req, log)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid since status = %d", recorder.Code)
	}

	req, recorder = deviceRequest(http.MethodGet, "/api/devices/device-1/activity", "device-1", "?limit=-1", "")
	GSApiDeviceActivityGet(recorder, req, log)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("negative limit status = %d", recorder.Code)
	}

	req, recorder = deviceRequest(http.MethodGet, "/api/devices/missing/activity", "missing", "", "")
	GSApiDeviceActivityGet(recorder, req, log)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown device status = %d", recorder.Code)
	}

	req, recorder = deviceRequest(http.MethodGet, "/api/devices/device-1/activity", "device-1", "", "")
	GSApiDeviceActivityGet(recorder, req, nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing logger status = %d", recorder.Code)
	}
}

func TestDevicePolicyEndpointsUnavailableWithoutDependencies(t *testing.T) {
	originalDevices := gatesentryDnsServer.GetDeviceStore()
	originalPolicy := gatesentryDnsServer.GetPolicyService()
	ds := discovery.NewDeviceStore("local")
	if _, err := ds.UpsertDeviceE(&discovery.Device{ID: "device-1", IPv4: "192.0.2.10"}); err != nil {
		t.Fatal(err)
	}
	gatesentryDnsServer.SetDeviceStoreForTests(ds)
	gatesentryDnsServer.SetPolicyServiceForTests(nil)
	defer func() {
		gatesentryDnsServer.SetDeviceStoreForTests(originalDevices)
		gatesentryDnsServer.SetPolicyServiceForTests(originalPolicy)
	}()

	req, recorder := deviceRequest(http.MethodGet, "/api/devices/device-1/policy", "device-1", "", "")
	GSApiDevicePolicyGet(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("policy without service status = %d", recorder.Code)
	}

	req, recorder = deviceRequest(http.MethodPut, "/api/devices/device-1/assignment", "device-1", "", `{"group_id":"kids"}`)
	GSApiDeviceAssignmentSet(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("assignment without service status = %d", recorder.Code)
	}

	// A missing device store is a 503 even when the policy service exists.
	_, cleanupPolicy := policyTestService(t)
	gatesentryDnsServer.SetDeviceStoreForTests(nil)
	req, recorder = deviceRequest(http.MethodGet, "/api/devices/device-1/policy", "device-1", "", "")
	GSApiDevicePolicyGet(recorder, req)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("policy without device store status = %d", recorder.Code)
	}
	req, recorder = deviceRequest(http.MethodGet, "/api/devices/device-1/activity", "device-1", "", "")
	GSApiDeviceActivityGet(recorder, req, gatesentryLogger.NewLogger(t.TempDir()+"/activity-503.db"))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("activity without device store status = %d", recorder.Code)
	}
	cleanupPolicy()
}
