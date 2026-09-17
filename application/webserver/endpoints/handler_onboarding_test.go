package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	"github.com/miekg/dns"
)

func onboardingStore(t *testing.T) *gatesentry2storage.MapStore {
	t.Helper()
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// startProtectionCheckDNS starts a real DNS server that reproduces the
// blocked decision GateSentry sends: NXDOMAIN plus the blocked.local CNAME.
func startProtectionCheckDNS(t *testing.T, blockedDomain string) (*dns.Server, string) {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	server := &dns.Server{PacketConn: conn, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		response := new(dns.Msg)
		response.SetReply(r)
		if len(r.Question) > 0 && r.Question[0].Name == dns.Fqdn(blockedDomain) {
			response.SetRcode(r, dns.RcodeNameError)
			response.Answer = append(response.Answer, &dns.CNAME{
				Hdr:    dns.RR_Header{Name: dns.Fqdn(blockedDomain), Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 3600},
				Target: "blocked.local.",
			})
		} else {
			response.SetRcode(r, dns.RcodeSuccess)
		}
		if err := w.WriteMsg(response); err != nil {
			t.Errorf("write DNS response: %v", err)
		}
	})}
	go server.ActivateAndServe()
	t.Cleanup(func() {
		if err := server.Shutdown(); err != nil {
			t.Errorf("shutdown DNS test server: %v", err)
		}
	})
	return server, conn.LocalAddr().(*net.UDPAddr).String()
}

func freeUDPPort(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func onboardingTestPort(hostPort string) string {
	_, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "59999"
	}
	return port
}

func TestOnboardingStatusSeparatesStartupFromProtection(t *testing.T) {
	store := onboardingStore(t)
	_, dnsAddr := startProtectionCheckDNS(t, protectionCheckDomain)

	deps := OnboardingDeps{
		Settings:      store,
		DnsServerInfo: &gatesentryTypes.DnsServerInfo{NumberDomainsBlocked: 12},
		ListenAddr:    "127.0.0.1",
		ListenPort:    onboardingTestPort(dnsAddr),
		Now:           func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) },
	}
	rec := httptest.NewRecorder()
	GSApiOnboardingStatusGET(rec, httptest.NewRequest(http.MethodGet, "/api/onboarding/status", nil), deps)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body %s", rec.Code, rec.Body.String())
	}
	var body OnboardingStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.DNSRunning {
		t.Fatalf("DNSRunning = false; bind %+v resolver %+v", body.Bind, body.Resolver)
	}
	if body.Bind.State != onboardingStateOK || body.Resolver.State != onboardingStateOK || body.Blocklist.State != onboardingStateOK {
		t.Fatalf("readiness steps: bind %+v resolver %+v blocklist %+v", body.Bind, body.Resolver, body.Blocklist)
	}
	if body.Progress.FirstExplainedProtectionAt.IsZero() == false {
		t.Fatal("startup must not set first_explained_protection_at")
	}
	if body.Progress.Version != onboardingProgressVersion || body.Progress.StartedAt.IsZero() {
		t.Fatalf("progress not seeded: %+v", body.Progress)
	}
	// Read again: the seeded record must round-trip through the store.
	rec2 := httptest.NewRecorder()
	GSApiOnboardingStatusGET(rec2, httptest.NewRequest(http.MethodGet, "/api/onboarding/status", nil), deps)
	var body2 OnboardingStatusResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &body2); err != nil {
		t.Fatal(err)
	}
	if !body2.Progress.StartedAt.Equal(body.Progress.StartedAt) {
		t.Fatalf("StartedAt changed: %v -> %v", body.Progress.StartedAt, body2.Progress.StartedAt)
	}
}

func TestOnboardingStatusReportsActionablePortAndBlocklistFailures(t *testing.T) {
	store := onboardingStore(t)
	deps := OnboardingDeps{
		Settings:   store,
		ListenAddr: "127.0.0.1",
		// Nothing listens here; bind must fail with an actionable action.
		ListenPort: freeUDPPort(t),
	}
	rec := httptest.NewRecorder()
	GSApiOnboardingStatusGET(rec, httptest.NewRequest(http.MethodGet, "/api/onboarding/status", nil), deps)
	var body OnboardingStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Bind.State != onboardingStateFailed || body.Bind.Action == "" {
		t.Fatalf("bind step = %+v", body.Bind)
	}
	if body.DNSRunning {
		t.Fatal("DNSRunning must be false when the listener is unavailable")
	}
	if body.Resolver.State != onboardingStateUnknown {
		t.Fatalf("resolver state = %q, expected unknown when the listener is down", body.Resolver.State)
	}
	if body.Blocklist.State != onboardingStateFailed || body.Blocklist.Action == "" {
		t.Fatalf("blocklist step = %+v", body.Blocklist)
	}
}

func TestOnboardingProtectionCheckPersistsExplainedDecision(t *testing.T) {
	store := onboardingStore(t)
	_, dnsAddr := startProtectionCheckDNS(t, protectionCheckDomain)
	ClearOnboardingBlockedMarker()
	t.Cleanup(ClearOnboardingBlockedMarker)

	deps := OnboardingDeps{
		Settings:   store,
		ListenAddr: "127.0.0.1",
		ListenPort: onboardingTestPort(dnsAddr),
		Now:        func() time.Time { return time.Date(2026, 9, 17, 12, 5, 0, 0, time.UTC) },
	}
	rec := httptest.NewRecorder()
	GSApiOnboardingProtectionCheckPOST(rec, httptest.NewRequest(http.MethodPost, "/api/onboarding/protection-check", nil), deps)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Check  ProtectionCheck
		State  string
		Detail string
		Action string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.State != onboardingStateOK {
		t.Fatalf("state = %q body %+v", body.State, body)
	}
	if body.Check.Domain != protectionCheckDomain || body.Check.Decision != "blocked" {
		t.Fatalf("check = %+v", body.Check)
	}
	if body.Check.Rcode != "NXDOMAIN" || body.Check.CNAME != "blocked.local." {
		t.Fatalf("rcode/CNAME = %q/%q", body.Check.Rcode, body.Check.CNAME)
	}

	raw, err := store.GetE(OnboardingProgressKey)
	if err != nil {
		t.Fatal(err)
	}
	var progress OnboardingProgress
	if err := json.Unmarshal([]byte(raw), &progress); err != nil {
		t.Fatal(err)
	}
	if progress.FirstExplainedProtectionAt.IsZero() || len(progress.ProtectionChecks) != 1 {
		t.Fatalf("progress = %+v", progress)
	}
	if progress.ProtectionChecks[0].Domain != protectionCheckDomain {
		t.Fatalf("persisted domain = %q", progress.ProtectionChecks[0].Domain)
	}

	statusRec := httptest.NewRecorder()
	GSApiOnboardingStatusGET(statusRec, httptest.NewRequest(http.MethodGet, "/api/onboarding/status", nil), deps)
	var status OnboardingStatusResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Progress.FirstExplainedProtectionAt.IsZero() {
		t.Fatal("status did not report persisted protection evidence")
	}
}

func TestOnboardingProtectionCheckRejectsMissingListener(t *testing.T) {
	store := onboardingStore(t)
	deps := OnboardingDeps{
		Settings:   store,
		ListenAddr: "127.0.0.1",
		ListenPort: freeUDPPort(t),
	}
	rec := httptest.NewRecorder()
	GSApiOnboardingProtectionCheckPOST(rec, httptest.NewRequest(http.MethodPost, "/api/onboarding/protection-check", nil), deps)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d", rec.Code)
	}
	if raw, err := store.GetE(OnboardingProgressKey); err != nil || raw != "" {
		t.Fatalf("failed check must not write progress: raw %q err %v", raw, err)
	}
}

func TestOnboardingProgressStoreFailuresAreVisible(t *testing.T) {
	dir := t.TempDir()
	old := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(dir + string(os.PathSeparator))
	t.Cleanup(func() { gatesentry2storage.SetBaseDir(old) })
	store, err := gatesentry2storage.OpenMapStore("settings", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(OnboardingProgressKey, "{not-json"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings"), []byte("malformed"), 0600); err != nil {
		t.Fatal(err)
	}
	deps := OnboardingDeps{Settings: store, ListenAddr: "127.0.0.1", ListenPort: freeUDPPort(t)}
	rec := httptest.NewRecorder()
	GSApiOnboardingStatusGET(rec, httptest.NewRequest(http.MethodGet, "/api/onboarding/status", nil), deps)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt progress returned %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	GSApiOnboardingProtectionCheckPOST(rec2, httptest.NewRequest(http.MethodPost, "/api/onboarding/protection-check", nil), deps)
	if rec2.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt progress check returned %d", rec2.Code)
	}
}
