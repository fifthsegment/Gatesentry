package gatesentryWebserverEndpoints

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentryStorage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

// inspectionTestStore mirrors the diagnostics test helper: a fresh MapStore
// with an installation key so encryption is available.
func inspectionTestStore(t *testing.T) *gatesentryStorage.MapStore {
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

func inspectionCertPEM(t *testing.T, notAfter time.Time) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "GateSentryTest"},
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func inspectionNoBumpFilters() *[]gatesentryFilters.GSFilter {
	return &[]gatesentryFilters.GSFilter{
		{
			Id:         "CeBqssmRbqXzbHR",
			FilterName: "Exception Hosts",
			Handles:    "url/https_dontbump",
			FileContents: []gatesentryFilters.GsFilterLine{
				{Content: "github.com", Score: 1},
				{Content: "app.snapchat.com", Score: 1},
			},
		},
	}
}

func TestInspectionStatusDisabledReportsExclusionsAndCounts(t *testing.T) {
	store := inspectionTestStore(t)
	if err := store.Update("enable_https_filtering", "false"); err != nil {
		t.Fatal(err)
	}
	if err := store.Update("capem", inspectionCertPEM(t, time.Now().Add(30*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	filters := inspectionNoBumpFilters()

	// Seed two inspect decisions and one bypass decision so the counts are
	// exercised even though inspection is disabled (the counts describe recorded
	// decisions, not live state).
	l := gatesentryLogger.NewLogger(t.TempDir() + "/inspection.db")
	t.Cleanup(func() { _ = l.Database.Close() })
	now := time.Now().Unix()
	for i := 0; i < 2; i++ {
		d := gatesentryPolicy.NewDecision(gatesentryPolicy.ActionDecisionInspect, gatesentryPolicy.LayerExplicitProxy, "bumped.example")
		d.ClientIP = "192.0.2.10"
		d.ResponseType = "ssl-bump"
		d.Timestamp = time.Unix(now-int64(i), 0)
		l.LogDecision(d)
	}
	dBypass := gatesentryPolicy.NewDecision(gatesentryPolicy.ActionDecisionBypass, gatesentryPolicy.LayerExplicitProxy, "github.com")
	dBypass.ClientIP = "192.0.2.10"
	dBypass.ResponseType = "ssldirect"
	dBypass.Timestamp = time.Unix(now, 0)
	l.LogDecision(dBypass)
	time.Sleep(150 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/certificate/inspection", nil)
	rec := httptest.NewRecorder()
	GSApiInspectionGET(rec, req, InspectionDeps{Settings: store, Filters: filters, Logger: l})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v, body=%s", err, rec.Body.String())
	}
	if body["enabled"] != false {
		t.Fatalf("enabled = %v, want false", body["enabled"])
	}
	// Exclusions surface from the existing no-bump filter.
	exclusions, _ := body["exclusions"].([]interface{})
	if len(exclusions) != 2 {
		t.Fatalf("exclusions = %v, want 2", exclusions)
	}
	cert, ok := body["certificate"].(map[string]interface{})
	if !ok {
		t.Fatalf("certificate missing: %v", body)
	}
	if cert["name"] != "GateSentryTest" {
		t.Fatalf("cert name = %v", cert["name"])
	}
	if cert["expired"] != false {
		t.Fatalf("expired = %v, want false", cert["expired"])
	}
	if body["inspect_count"].(float64) != 2 {
		t.Fatalf("inspect_count = %v, want 2", body["inspect_count"])
	}
	if body["bypass_count"].(float64) != 1 {
		t.Fatalf("bypass_count = %v, want 1", body["bypass_count"])
	}
	caveat, _ := body["coverage_caveat"].(string)
	if caveat == "" {
		t.Fatal("coverage caveat must not be empty")
	}
	if body["certificate_error"] != nil && body["certificate_error"] != "" {
		t.Fatalf("unexpected certificate_error: %v", body["certificate_error"])
	}
}

func TestInspectionStatusEnabledWithExpiredCertReportsError(t *testing.T) {
	store := inspectionTestStore(t)
	if err := store.Update("enable_https_filtering", "true"); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-24 * time.Hour)
	if err := store.Update("capem", inspectionCertPEM(t, past)); err != nil {
		t.Fatal(err)
	}
	filters := inspectionNoBumpFilters()

	req := httptest.NewRequest(http.MethodGet, "/api/certificate/inspection", nil)
	rec := httptest.NewRecorder()
	GSApiInspectionGET(rec, req, InspectionDeps{Settings: store, Filters: filters, Logger: nil})
	var body map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["enabled"] != true {
		t.Fatalf("enabled = %v, want true", body["enabled"])
	}
	cert, _ := body["certificate"].(map[string]interface{})
	if cert == nil {
		t.Fatalf("certificate should still be present even when expired: %v", body)
	}
	if cert["expired"] != true {
		t.Fatalf("expired = %v, want true", cert["expired"])
	}
}

func TestInspectionStatusMissingCertReportsCertificateError(t *testing.T) {
	store := inspectionTestStore(t)
	if err := store.Update("enable_https_filtering", "true"); err != nil {
		t.Fatal(err)
	}
	// No capem stored.
	filters := inspectionNoBumpFilters()

	req := httptest.NewRequest(http.MethodGet, "/api/certificate/inspection", nil)
	rec := httptest.NewRecorder()
	GSApiInspectionGET(rec, req, InspectionDeps{Settings: store, Filters: filters, Logger: nil})
	var body map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["certificate"] != nil {
		t.Fatalf("certificate should be nil when unavailable: %v", body)
	}
	if body["certificate_error"] == "" {
		t.Fatalf("expected certificate_error, got: %v", body)
	}
}

func TestInspectionStatusCertRotationReflectedNotStale(t *testing.T) {
	store := inspectionTestStore(t)
	if err := store.Update("enable_https_filtering", "true"); err != nil {
		t.Fatal(err)
	}
	first := inspectionCertPEM(t, time.Now().Add(30*24*time.Hour))
	if err := store.Update("capem", first); err != nil {
		t.Fatal(err)
	}
	// Read the first certificate.
	detail1, err := ParseCertDetail(store)
	if err != nil {
		t.Fatal(err)
	}
	// Rotate the certificate to a new expiry.
	second := inspectionCertPEM(t, time.Now().Add(60*24*time.Hour))
	if err := store.Update("capem", second); err != nil {
		t.Fatal(err)
	}
	detail2, err := ParseCertDetail(store)
	if err != nil {
		t.Fatal(err)
	}
	if detail1.NotAfter == detail2.NotAfter {
		t.Fatalf("certificate rotation not reflected: notAfter is stale (%s == %s)", detail1.NotAfter, detail2.NotAfter)
	}
}

func TestInspectionStatusExplicitProxyInspectDecision(t *testing.T) {
	// Explicit proxy mode: a ProxyActionSSLBump decision maps to inspect.
	l := gatesentryLogger.NewLogger(t.TempDir() + "/explicit.db")
	t.Cleanup(func() { _ = l.Database.Close() })
	now := time.Now().Unix()
	d := gatesentryPolicy.NewDecision(gatesentryPolicy.ActionDecisionInspect, gatesentryPolicy.LayerExplicitProxy, "bumped.example")
	d.ClientIP = "192.0.2.20"
	d.ResponseType = "ssl-bump"
	d.Timestamp = time.Unix(now, 0)
	l.LogDecision(d)
	time.Sleep(150 * time.Millisecond)

	store := inspectionTestStore(t)
	filters := inspectionNoBumpFilters()

	req := httptest.NewRequest(http.MethodGet, "/api/certificate/inspection", nil)
	rec := httptest.NewRecorder()
	GSApiInspectionGET(rec, req, InspectionDeps{Settings: store, Filters: filters, Logger: l})
	var body map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["inspect_count"].(float64) != 1 {
		t.Fatalf("inspect_count = %v, want 1 (explicit proxy ssl-bump)", body["inspect_count"])
	}
	if body["bypass_count"].(float64) != 0 {
		t.Fatalf("bypass_count = %v, want 0", body["bypass_count"])
	}
}

func TestInspectionStatusTransparentProxyBypassDecision(t *testing.T) {
	// Transparent proxy mode: a ProxyActionSSLDirect decision maps to bypass
	// (ssldirect). A host on the no-bump list is tunnelled, not inspected.
	l := gatesentryLogger.NewLogger(t.TempDir() + "/transparent.db")
	t.Cleanup(func() { _ = l.Database.Close() })
	now := time.Now().Unix()
	d := gatesentryPolicy.NewDecision(gatesentryPolicy.ActionDecisionBypass, gatesentryPolicy.LayerTransparentProxy, "github.com")
	d.ClientIP = "192.0.2.30"
	d.ResponseType = "ssldirect"
	d.Timestamp = time.Unix(now, 0)
	l.LogDecision(d)
	time.Sleep(150 * time.Millisecond)

	store := inspectionTestStore(t)
	filters := inspectionNoBumpFilters()

	req := httptest.NewRequest(http.MethodGet, "/api/certificate/inspection", nil)
	rec := httptest.NewRecorder()
	GSApiInspectionGET(rec, req, InspectionDeps{Settings: store, Filters: filters, Logger: l})
	var body map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["bypass_count"].(float64) != 1 {
		t.Fatalf("bypass_count = %v, want 1 (transparent proxy ssldirect for no-bump host)", body["bypass_count"])
	}
	if body["inspect_count"].(float64) != 0 {
		t.Fatalf("inspect_count = %v, want 0", body["inspect_count"])
	}
	// The no-bump host must appear in exclusions (single source of truth).
	exclusions, _ := body["exclusions"].([]interface{})
	found := false
	for _, h := range exclusions {
		if h == "github.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("github.com not in exclusions: %v", exclusions)
	}
}

func TestInspectionStatusNeverLeaksPrivateKey(t *testing.T) {
	store := inspectionTestStore(t)
	if err := store.Update("enable_https_filtering", "true"); err != nil {
		t.Fatal(err)
	}
	if err := store.Update("capem", inspectionCertPEM(t, time.Now().Add(30*24*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := store.Update("keypem", "super-secret-private-key-material"); err != nil {
		t.Fatal(err)
	}
	filters := inspectionNoBumpFilters()

	req := httptest.NewRequest(http.MethodGet, "/api/certificate/inspection", nil)
	rec := httptest.NewRecorder()
	GSApiInspectionGET(rec, req, InspectionDeps{Settings: store, Filters: filters, Logger: nil})
	if body := rec.Body.String(); strings.Contains(body, "super-secret-private-key-material") {
		t.Fatalf("response leaked private key material: %s", body)
	}
}
