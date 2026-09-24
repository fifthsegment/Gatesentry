package diagnostics

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gatesentryDiscovery "bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentryStorage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
)

func testStore(t *testing.T) *gatesentryStorage.MapStore {
	t.Helper()
	dir := t.TempDir()
	old := gatesentryStorage.GSBASEDIR
	gatesentryStorage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentryStorage.SetBaseDir(old) })
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "installation.key"), key, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := gatesentryStorage.OpenMapStore("GSSettings", false)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func certificatePEM(t *testing.T, notAfter time.Time) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "GateSentry"}, NotBefore: notAfter.Add(-365 * 24 * time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestRunReportsFailuresAndCoverageUncertainty(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	store := testStore(t)
	if err := store.Update("capem", certificatePEM(t, now.Add(30*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	report := Run(Deps{Now: func() time.Time { return now }, Settings: store, DnsInfo: &gatesentryTypes.DnsServerInfo{NumberDomainsBlocked: 5, LastUpdated: int(now.Add(-48 * time.Hour).Unix())}, Devices: gatesentryDiscovery.NewDeviceStore("local"), Listener: func(context.Context, string, string) error { return errors.New("connection refused") }, UpstreamCheck: func(context.Context, string) error { return nil }})
	if report.Overall != StatusFailed {
		t.Fatalf("overall=%q", report.Overall)
	}
	byName := map[string]Check{}
	for _, c := range report.Checks {
		byName[c.Name] = c
	}
	if byName["listeners"].Status != StatusFailed || byName["blocklist"].Status != StatusFailed {
		t.Fatalf("expected visible failures: %+v", report.Checks)
	}
	if byName["policy"].Status != StatusUnknown || byName["policy"].Recovery == "" {
		t.Fatalf("expected policy uncertainty and recovery: %+v", byName["policy"])
	}
	if byName["storage"].Status != StatusOK || byName["certificate"].Status != StatusOK {
		t.Fatalf("expected healthy storage and certificate: %+v", report.Checks)
	}
}

func TestCreateBundleUsesSafeAllowlist(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	store := testStore(t)
	secrets := map[string]string{"admin_password": "secret-password", "keypem": "private-key-material", "capem": certificatePEM(t, now.Add(24*time.Hour)), "ai_openai_api_key": "sk-secret", "logs": "private.example/path", "image_payload": "base64-image", "timezone": "UTC", "dns_resolver": "1.1.1.1:53", "enable_dns_server": "true"}
	if err := store.UpdateValues(secrets); err != nil {
		t.Fatal(err)
	}
	filters := []gatesentryFilters.GSFilter{{Id: "one", FilterName: "Blocked URLs", Handles: "url", FileContents: []gatesentryFilters.GsFilterLine{{Content: "private.example", Score: 1}}}}
	data, err := CreateBundle(Deps{Now: func() time.Time { return now }, Settings: store, DnsInfo: &gatesentryTypes.DnsServerInfo{}, Filters: &filters, Listener: func(context.Context, string, string) error { return nil }, UpstreamCheck: func(context.Context, string) error { return nil }}, "test-v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-password", "private-key-material", "sk-secret", "private.example", "base64-image", "BEGIN CERTIFICATE"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("bundle leaked %q: %s", forbidden, data)
		}
	}
	var bundle Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Settings["timezone"] != "UTC" || bundle.Settings["dns_resolver"] != "1.1.1.1:53" {
		t.Fatalf("safe settings missing: %+v", bundle.Settings)
	}
	if len(bundle.Filters) != 1 {
		t.Fatalf("filter summary missing: %+v", bundle.Filters)
	}
}

func TestRunDoesNotExposeStableTailscaleNodeIDs(t *testing.T) {
	devices := gatesentryDiscovery.NewDeviceStore("local")
	if _, err := devices.UpsertDeviceE(&gatesentryDiscovery.Device{
		ID: "canonical-phone", IPv4: "192.0.2.70", Persistent: true,
	}); err != nil {
		t.Fatal(err)
	}
	const nodeID = "node-stable-secret"
	if _, err := devices.LinkTailscaleIdentityE("canonical-phone", gatesentryDiscovery.TailscaleIdentity{
		NodeID: nodeID, Addresses: []string{"100.64.0.70"}, Online: true,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(Run(Deps{
		Devices:       devices,
		Listener:      func(context.Context, string, string) error { return nil },
		UpstreamCheck: func(context.Context, string) error { return nil },
	}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), nodeID) {
		t.Fatalf("diagnostics leaked stable Tailscale node ID: %s", data)
	}
}
