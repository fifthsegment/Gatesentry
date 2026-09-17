package gatesentryf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

func TestInitUpgradePreservesConfigurationAndDevices(t *testing.T) {
	dir := t.TempDir()
	oldBaseDir := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	oldStorageBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() {
		SetBaseDir(oldBaseDir)
		gatesentry2storage.SetBaseDir(oldStorageBaseDir)
	})

	// Seed a pre-schema settings store with legacy rules, credentials, and
	// explicitly disabled optional features.
	seedSettings, err := gatesentry2storage.OpenMapStore("GSSettings", true)
	if err != nil {
		t.Fatal(err)
	}
	legacyUsers := `[{"username":"admin","password":"old-secret","Base64String":"b2xkLXNlY3JldA==","allowaccess":true,"dataconsumed":12}]`
	legacyRules := `[{"id":"rule-1","name":"block ads","action":"block","domain":"ads.example","priority":2}]`
	if err := seedSettings.UpdateValues(map[string]string{
		"version":                   "1.19.0",
		"authusers":                 legacyUsers,
		"rules":                     legacyRules,
		"enable_https_filtering":    "false",
		"enable_ai_image_filtering": "false",
		"enable_dns_server":         "true",
		"timezone":                  "Europe/Oslo",
		"strictness":                "2000",
		"general_settings":          `{"log_location":"./log.db"}`,
		"blocktimes":                `{"fromhours":0,"tohours":0,"fromminutes":58,"tominutes":59}`,
		"dns_custom_entries":        `[]`,
		"capem":                     "",
		"keypem":                    "",
	}); err != nil {
		t.Fatal(err)
	}

	// Seed the new device assignment store with one durable assignment.
	deviceStore, err := gatesentry2storage.OpenMapStore("GSDevices", true)
	if err != nil {
		t.Fatal(err)
	}
	assignment := map[string]discovery.DeviceAssignment{
		"dev-1": {
			ID:         "dev-1",
			DNSName:    "laptop",
			ManualName: "Family laptop",
			Owner:      "Vivienne",
			Category:   "kids",
			Hostnames:  []string{"laptop"},
			MACs:       []string{"aa:bb:cc:dd:ee:ff"},
			FirstSeen:  time.Now(),
			Persistent: true,
		},
	}
	payload, err := json.Marshal(map[string]interface{}{"version": 1, "assignments": assignment})
	if err != nil {
		t.Fatal(err)
	}
	if err := deviceStore.Update(discovery.DeviceAssignmentsKey, string(payload)); err != nil {
		t.Fatal(err)
	}

	runtime := &GSRuntime{DNSServerChannel: make(chan int, 1)}
	oldR := R
	R = runtime
	t.Cleanup(func() { R = oldR })
	if err := runtime.Init(); err != nil {
		t.Fatal(err)
	}

	// Schema version is stamped and defaults are seeded without overwriting.
	value, err := runtime.GSSettings.GetE(SettingsSchemaKey)
	if err != nil {
		t.Fatal(err)
	}
	if value != "1" {
		t.Fatalf("settings schema = %q", value)
	}
	usersJSON, err := runtime.GSSettings.GetE("authusers")
	if err != nil {
		t.Fatal(err)
	}
	var users []GatesentryTypes.GSUser
	if err := json.Unmarshal([]byte(usersJSON), &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].User != "admin" || users[0].DataConsumed != 12 || users[0].Base64String != "b2xkLXNlY3JldA==" {
		t.Fatalf("users were not preserved: %s", usersJSON)
	}
	rulesJSON, err := runtime.GSSettings.GetE("rules")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rulesJSON, `"id":"rule-1"`) {
		t.Fatalf("rules were not preserved: %s", rulesJSON)
	}
	for key, want := range map[string]string{
		"enable_https_filtering":    "false",
		"enable_ai_image_filtering": "false",
		"enable_dns_server":         "true",
	} {
		got, err := runtime.GSSettings.GetE(key)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}

	// The DNS server attaches GSDevices to the device store through this same
	// AttachPersistence call. DNS startup is asynchronous, so the restart path
	// is exercised directly to keep the fixture deterministic.
	ds := discovery.NewDeviceStore("local")
	if err := ds.AttachPersistence(runtime.GSDevices); err != nil {
		t.Fatal(err)
	}
	device := ds.GetDevice("dev-1")
	if device == nil {
		t.Fatal("device assignment not restored")
	}
	if device.ManualName != "Family laptop" || device.Owner != "Vivienne" || device.Category != "kids" {
		t.Fatalf("device assignment fields = %+v", device)
	}
}

func TestInitUpgradeFailsClosedOnNewerSettingsSchema(t *testing.T) {
	dir := t.TempDir()
	oldBaseDir := GSBASEDIR
	SetBaseDir(dir + string(os.PathSeparator))
	oldStorageBaseDir := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() {
		SetBaseDir(oldBaseDir)
		gatesentry2storage.SetBaseDir(oldStorageBaseDir)
	})

	store, err := gatesentry2storage.OpenMapStore("GSSettings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(SettingsSchemaKey, "99"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "GSSettings")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	runtime := &GSRuntime{DNSServerChannel: make(chan int, 1)}
	oldR := R
	R = runtime
	t.Cleanup(func() { R = oldR })
	err = runtime.Init()
	if err == nil || !strings.Contains(err.Error(), "newer than this release supports") {
		t.Fatalf("Init error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed upgrade must leave settings byte-identical")
	}
}
