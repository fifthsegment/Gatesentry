package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	"github.com/miekg/dns"
)

const (
	// OnboardingProgressKey stores setup progress as a JSON value in
	// GSSettings. It is an optional schema-1 key: migration leaves it missing
	// and the handler seeds it on first read.
	OnboardingProgressKey = "onboarding_progress"
	// protectionCheckDomain is the controlled label queried by the
	// end-to-end protection check. Evidence records only this label and its
	// decision, never other domains a user resolves through GateSentry.
	protectionCheckDomain     = "gatesentry-protection-check.invalid"
	onboardingProgressVersion = 1
	onboardingStateOK         = "ok"
	onboardingStateFailed     = "failed"
	onboardingStateUnknown    = "unknown"
	onboardingUpstreamTimeout = 3 * time.Second
	onboardingEvidenceLimit   = 20
)

// OnboardingCheckStep reports one readiness check with a concrete action.
type OnboardingCheckStep struct {
	State  string `json:"state"`
	Action string `json:"action,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// ProtectionCheck is durable evidence that the running GateSentry DNS server
// made a filtering decision for the controlled test domain.
type ProtectionCheck struct {
	At       time.Time `json:"at"`
	Domain   string    `json:"domain"`
	Decision string    `json:"decision"`
	Expected string    `json:"expected"`
	Rcode    string    `json:"rcode"`
	CNAME    string    `json:"cname,omitempty"`
	Upstream string    `json:"upstream"`
	Error    string    `json:"error,omitempty"`
}

// OnboardingProgress is the persisted setup-progress record. StartedAt and
// FirstExplainedProtectionAt measure time-to-first-explained-protection
// without storing browsing data: only the controlled test domain is kept.
type OnboardingProgress struct {
	Version                    int               `json:"version"`
	StartedAt                  time.Time         `json:"started_at"`
	FirstExplainedProtectionAt time.Time         `json:"first_explained_protection_at,omitempty"`
	LastFailedAction           string            `json:"last_failed_action,omitempty"`
	ProtectionChecks           []ProtectionCheck `json:"protection_checks,omitempty"`
}

// OnboardingStatusResponse reports progress through DNS-first onboarding.
type OnboardingStatusResponse struct {
	Version     int                 `json:"version"`
	Steps       []OnboardingStep    `json:"steps"`
	Progress    OnboardingProgress  `json:"progress"`
	DNSRunning  bool                `json:"dns_running"`
	ListenAddr  string              `json:"listen_addr"`
	ListenPort  string              `json:"listen_port"`
	Upstream    string              `json:"upstream"`
	Blocked     int                 `json:"blocked_domains"`
	Blocklist   OnboardingCheckStep `json:"blocklist"`
	Bind        OnboardingCheckStep `json:"bind"`
	Resolver    OnboardingCheckStep `json:"resolver"`
	DeviceCount int                 `json:"device_count"`
}

// OnboardingStep is one named stage in the guided flow.
type OnboardingStep struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// OnboardingDeps carries explicit handler dependencies.
type OnboardingDeps struct {
	Settings      *gatesentry2storage.MapStore
	DnsServerInfo *gatesentryTypes.DnsServerInfo
	// ListenAddr and ListenPort default to the DNS server values when empty.
	ListenAddr string
	ListenPort string
	// Now lets tests control timestamps.
	Now func() time.Time
}

// onboardingState owns the transient blocked marker and resolver hook.
// Durable progress lives in GSSettings.
var (
	onboardingMu              sync.Mutex
	onboardingMarkerInstalled bool
	onboardingResolveUpstream = defaultOnboardingResolveUpstream
)

// SetOnboardingBlockedMarker installs the controlled domain in the DNS
// blocked map so the live query produces a real filtering decision.
func SetOnboardingBlockedMarker() error {
	onboardingMu.Lock()
	defer onboardingMu.Unlock()
	if onboardingMarkerInstalled {
		return nil
	}
	gatesentryDnsServer.AddBlockedDomainForTest(protectionCheckDomain)
	onboardingMarkerInstalled = true
	return nil
}

// ClearOnboardingBlockedMarker removes the controlled domain.
func ClearOnboardingBlockedMarker() {
	onboardingMu.Lock()
	defer onboardingMu.Unlock()
	if !onboardingMarkerInstalled {
		return
	}
	gatesentryDnsServer.RemoveBlockedDomainForTest(protectionCheckDomain)
	onboardingMarkerInstalled = false
}

// SetOnboardingUpstreamResolver swaps the upstream test hook and returns the
// previous function so callers can restore it.
func SetOnboardingUpstreamResolver(next func(query *dns.Msg, resolver string) (*dns.Msg, error)) func(query *dns.Msg, resolver string) (*dns.Msg, error) {
	onboardingMu.Lock()
	defer onboardingMu.Unlock()
	previous := onboardingResolveUpstream
	if next != nil {
		onboardingResolveUpstream = next
	}
	return previous
}

func defaultOnboardingResolveUpstream(query *dns.Msg, resolver string) (*dns.Msg, error) {
	client := new(dns.Client)
	client.Timeout = onboardingUpstreamTimeout
	resp, _, err := client.Exchange(query, resolver)
	return resp, err
}

func onboardingNow(deps OnboardingDeps) time.Time {
	if deps.Now != nil {
		return deps.Now().UTC()
	}
	return time.Now().UTC()
}

func loadOnboardingProgress(deps OnboardingDeps) (OnboardingProgress, error) {
	var progress OnboardingProgress
	if deps.Settings == nil {
		return progress, errors.New("onboarding settings store is nil")
	}
	raw, err := deps.Settings.GetE(OnboardingProgressKey)
	if err != nil {
		return progress, fmt.Errorf("read onboarding progress: %w", err)
	}
	if raw == "" {
		progress.Version = onboardingProgressVersion
		progress.StartedAt = onboardingNow(deps)
		return progress, nil
	}
	if err := json.Unmarshal([]byte(raw), &progress); err != nil {
		return progress, fmt.Errorf("parse onboarding progress: %w", err)
	}
	if progress.Version != onboardingProgressVersion {
		return progress, fmt.Errorf("unsupported onboarding progress version %d", progress.Version)
	}
	if progress.StartedAt.IsZero() {
		return progress, errors.New("onboarding progress missing started_at")
	}
	return progress, nil
}

func saveOnboardingProgress(deps OnboardingDeps, progress OnboardingProgress) error {
	if deps.Settings == nil {
		return errors.New("onboarding settings store is nil")
	}
	// The evidence list contains only the controlled test domain; the bound
	// keeps repeated checks from growing the settings value without limit.
	if len(progress.ProtectionChecks) > onboardingEvidenceLimit {
		progress.ProtectionChecks = progress.ProtectionChecks[len(progress.ProtectionChecks)-onboardingEvidenceLimit:]
	}
	encoded, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("encode onboarding progress: %w", err)
	}
	return deps.Settings.Update(OnboardingProgressKey, string(encoded))
}

// GSApiOnboardingStatusGET reports setup progress and readiness checks.
func GSApiOnboardingStatusGET(w http.ResponseWriter, r *http.Request, deps OnboardingDeps) {
	progress, err := loadOnboardingProgress(deps)
	if err != nil {
		http.Error(w, `{"error":"unable to read onboarding progress"}`, http.StatusInternalServerError)
		return
	}
	status := buildOnboardingStatus(deps, progress)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(status)
}

// GSApiOnboardingProtectionCheckPOST runs the end-to-end protection check
// through the live GateSentry DNS listener, persists the decision, and
// returns the evidence. Server startup is never treated as protection.
func GSApiOnboardingProtectionCheckPOST(w http.ResponseWriter, r *http.Request, deps OnboardingDeps) {
	progress, err := loadOnboardingProgress(deps)
	if err != nil {
		http.Error(w, `{"error":"unable to read onboarding progress"}`, http.StatusInternalServerError)
		return
	}

	listenAddr, listenPort := onboardingListener(deps)
	bindState, _ := onboardingBindCheck(listenAddr, listenPort)
	if bindState != onboardingStateOK {
		http.Error(w, `{"error":"DNS server is not listening; fix the port failure and retry"}`, http.StatusServiceUnavailable)
		return
	}

	if err := SetOnboardingBlockedMarker(); err != nil {
		http.Error(w, `{"error":"unable to install the controlled protection-check domain"}`, http.StatusInternalServerError)
		return
	}

	query := new(dns.Msg)
	query.SetQuestion(dns.Fqdn(protectionCheckDomain), dns.TypeA)
	resp, exchangeErr := exchangeOnboardingQuery(listenAddr, listenPort, query)

	check := ProtectionCheck{
		At:       onboardingNow(deps),
		Domain:   protectionCheckDomain,
		Expected: "blocked",
		Upstream: gatesentryDnsServer.GetExternalResolver(),
	}
	decision := onboardingStateFailed
	detail := ""
	action := "GateSentry did not block the controlled test domain. Refresh the blocklists, verify the upstream resolver, and retry."
	if exchangeErr != nil {
		check.Error = exchangeErr.Error()
		check.Rcode = dns.RcodeToString[dns.RcodeServerFailure]
		detail = fmt.Sprintf("The controlled domain did not resolve through GateSentry: %v", exchangeErr)
	} else {
		check.Rcode = dns.RcodeToString[resp.Rcode]
		for _, answer := range resp.Answer {
			if cname, ok := answer.(*dns.CNAME); ok {
				check.CNAME = cname.Target
			}
		}
		if resp.Rcode == dns.RcodeNameError && check.CNAME == "blocked.local." {
			decision = onboardingStateOK
			check.Decision = "blocked"
			detail = "GateSentry answered NXDOMAIN and explained it with a blocked.local CNAME."
			action = ""
		} else {
			check.Decision = "not-blocked"
			detail = fmt.Sprintf("GateSentry returned %s without the blocked.local explanation.", check.Rcode)
		}
	}

	if decision == onboardingStateOK {
		if progress.FirstExplainedProtectionAt.IsZero() {
			progress.FirstExplainedProtectionAt = check.At
		}
		progress.LastFailedAction = ""
	} else {
		progress.LastFailedAction = action
	}
	progress.ProtectionChecks = append(progress.ProtectionChecks, check)
	if err := saveOnboardingProgress(deps, progress); err != nil {
		http.Error(w, `{"error":"unable to persist protection check evidence"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(struct {
		Check  ProtectionCheck `json:"check"`
		State  string          `json:"state"`
		Detail string          `json:"detail"`
		Action string          `json:"action,omitempty"`
	}{Check: check, State: decision, Detail: detail, Action: action})
}

func onboardingListener(deps OnboardingDeps) (string, string) {
	listenAddr, listenPort := deps.ListenAddr, deps.ListenPort
	if listenAddr == "" {
		listenAddr = gatesentryDnsServer.GetListenAddr()
	}
	if listenPort == "" {
		listenPort = gatesentryDnsServer.GetListenPort()
	}
	return listenAddr, listenPort
}

func exchangeOnboardingQuery(listenAddr, listenPort string, query *dns.Msg) (*dns.Msg, error) {
	server := onboardingLoopbackHost(listenAddr)
	client := new(dns.Client)
	client.Timeout = onboardingUpstreamTimeout
	resp, _, err := client.Exchange(query, net.JoinHostPort(server, listenPort))
	return resp, err
}

func onboardingLoopbackHost(listenAddr string) string {
	if listenAddr == "" || listenAddr == "0.0.0.0" || listenAddr == "::" || listenAddr == "[::]" {
		return "127.0.0.1"
	}
	return listenAddr
}

func buildOnboardingStatus(deps OnboardingDeps, progress OnboardingProgress) OnboardingStatusResponse {
	listenAddr, listenPort := onboardingListener(deps)
	upstream := gatesentryDnsServer.GetExternalResolver()
	response := OnboardingStatusResponse{
		Version:    onboardingProgressVersion,
		ListenAddr: listenAddr,
		ListenPort: listenPort,
		Upstream:   upstream,
		Progress:   progress,
	}

	bindState, bindDetail := onboardingBindCheck(listenAddr, listenPort)
	response.Bind = OnboardingCheckStep{State: bindState, Detail: bindDetail}
	if bindState != onboardingStateOK {
		response.Bind.Action = "Another DNS server may own the port (for example systemd-resolved on 127.0.0.53:53). Stop it, or move GateSentry with GATESENTRY_DNS_ADDR/GATESENTRY_DNS_PORT, then restart."
	}

	if deviceStore := gatesentryDnsServer.GetDeviceStore(); deviceStore != nil {
		response.DeviceCount = deviceStore.DeviceCount()
	}
	if deps.DnsServerInfo != nil {
		response.Blocked = deps.DnsServerInfo.NumberDomainsBlocked
	}
	if response.Blocked == 0 {
		response.Blocklist = OnboardingCheckStep{
			State:  onboardingStateFailed,
			Detail: "No blocked domains are loaded.",
			Action: "Check DNS blocklist URLs and outbound access, refresh the blocklists, then retry.",
		}
	} else {
		response.Blocklist = OnboardingCheckStep{
			State:  onboardingStateOK,
			Detail: fmt.Sprintf("%d blocked domains are loaded.", response.Blocked),
		}
	}

	if bindState == onboardingStateOK {
		upstreamState, upstreamDetail := checkOnboardingUpstream(listenAddr, listenPort, upstream)
		response.Resolver = OnboardingCheckStep{State: upstreamState, Detail: upstreamDetail}
		if upstreamState != onboardingStateOK {
			response.Resolver.Action = "Set a reachable upstream resolver (for example 1.1.1.1:53) in DNS settings, then retry."
		}
	} else {
		response.Resolver = OnboardingCheckStep{
			State:  onboardingStateUnknown,
			Detail: "The DNS listener is not ready, so the upstream path was not exercised.",
		}
	}

	response.DNSRunning = bindState == onboardingStateOK && response.Resolver.State == onboardingStateOK
	response.Steps = onboardingSteps(response, progress)
	return response
}

func checkOnboardingUpstream(listenAddr, listenPort, upstream string) (string, string) {
	query := new(dns.Msg)
	// A PTR lookup for 127.0.0.1 never enters the blocked path, so it
	// exercises the live forwarder and the configured upstream resolver.
	query.SetQuestion("1.0.0.127.in-addr.arpa.", dns.TypePTR)
	server := net.JoinHostPort(onboardingLoopbackHost(listenAddr), listenPort)
	client := new(dns.Client)
	client.Timeout = onboardingUpstreamTimeout
	resp, _, err := client.Exchange(query, server)
	if err != nil {
		return onboardingStateFailed, fmt.Sprintf("Upstream %s did not answer: %v.", upstream, err)
	}
	if resp.Rcode == dns.RcodeServerFailure {
		return onboardingStateFailed, fmt.Sprintf("GateSentry returned SERVFAIL while using upstream %s.", upstream)
	}
	return onboardingStateOK, fmt.Sprintf("A live query through GateSentry reached upstream %s and returned %s.", upstream, dns.RcodeToString[resp.Rcode])
}

func onboardingSteps(status OnboardingStatusResponse, progress OnboardingProgress) []OnboardingStep {
	steps := make([]OnboardingStep, 0, 6)
	steps = append(steps, OnboardingStep{Name: "resolver", State: status.Resolver.State})
	steps = append(steps, OnboardingStep{Name: "blocklist", State: status.Blocklist.State})
	steps = append(steps, OnboardingStep{Name: "clients", State: onboardingStateUnknown})
	steps = append(steps, OnboardingStep{Name: "first_device", State: onboardingStepState(status.DeviceCount > 0)})
	protectionState := onboardingStateFailed
	if !progress.FirstExplainedProtectionAt.IsZero() {
		protectionState = onboardingStateOK
	} else if !status.DNSRunning {
		protectionState = onboardingStateUnknown
	}
	steps = append(steps, OnboardingStep{Name: "protection_check", State: protectionState})
	steps = append(steps, OnboardingStep{Name: "https_inspection", State: onboardingStateUnknown})
	return steps
}

func onboardingStepState(complete bool) string {
	if complete {
		return onboardingStateOK
	}
	return onboardingStateFailed
}

// onboardingBindCheck reports whether the configured listener answers a
// DNS query. The protection check, not this check, is what proves filtering.
func onboardingBindCheck(listenAddr, listenPort string) (string, string) {
	return onboardingBindCheckWithTimeout(listenAddr, listenPort, 300*time.Millisecond)
}

// onboardingBindCheckWithTimeout performs the listener probe with the given
// deadline so tests can bound their run time.
func onboardingBindCheckWithTimeout(listenAddr, listenPort string, timeout time.Duration) (string, string) {
	if _, err := strconv.Atoi(listenPort); err != nil {
		return onboardingStateFailed, fmt.Sprintf("DNS listen port %q is not numeric.", listenPort)
	}
	probe := net.JoinHostPort(onboardingLoopbackHost(listenAddr), listenPort)
	probeQuery := new(dns.Msg)
	probeQuery.SetQuestion("gatesentry-bind-probe.invalid.", dns.TypeA)
	probeClient := new(dns.Client)
	probeClient.Timeout = timeout
	resp, _, err := probeClient.Exchange(probeQuery, probe)
	if err != nil {
		return onboardingStateFailed, fmt.Sprintf("GateSentry is not answering on %s: %v.", probe, err)
	}
	if resp == nil {
		return onboardingStateFailed, fmt.Sprintf("GateSentry returned no DNS response on %s.", probe)
	}
	return onboardingStateOK, fmt.Sprintf("The DNS listener on %s is answering.", probe)
}
