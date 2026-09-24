package gatesentryDnsServer

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryDnsScheduler "bitbucket.org/abdullah_irfan/gatesentryf/dns/scheduler"
	gatesentryDnsUtils "bitbucket.org/abdullah_irfan/gatesentryf/dns/utils"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTailscale "bitbucket.org/abdullah_irfan/gatesentryf/tailscale"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	"github.com/miekg/dns"
)

// normalizeResolver ensures the resolver address has a port suffix
// If no port is specified, :53 is appended
// Properly handles IPv6 addresses (e.g., [2001:4860:4860::8888]:53)
func normalizeResolver(resolver string) string {
	if resolver == "" {
		return "8.8.8.8:53"
	}
	// Try to split host and port - if it fails, no port is specified
	host, port, err := net.SplitHostPort(resolver)
	if err != nil {
		// No port specified (or invalid format), add default port
		// net.JoinHostPort handles IPv6 bracketing automatically
		return net.JoinHostPort(resolver, "53")
	}
	// Port was specified, return as-is (already valid format)
	if port == "" {
		return net.JoinHostPort(host, "53")
	}
	return resolver
}

type QueryLog struct {
	Domain string
	Time   time.Time
}

// Environment variable names for DNS server configuration
const (
	// ENV_DNS_LISTEN_ADDR sets the IP address to bind the DNS server (default: 0.0.0.0)
	ENV_DNS_LISTEN_ADDR = "GATESENTRY_DNS_ADDR"
	// ENV_DNS_LISTEN_PORT sets the port for UDP/TCP DNS server (default: 53)
	ENV_DNS_LISTEN_PORT = "GATESENTRY_DNS_PORT"
	// ENV_DNS_EXTERNAL_RESOLVER sets the external DNS resolver (default: 8.8.8.8:53)
	ENV_DNS_EXTERNAL_RESOLVER = "GATESENTRY_DNS_RESOLVER"
)

var (
	externalResolver = "8.8.8.8:53"
	listenAddr       = "0.0.0.0"
	listenPort       = "53"
	// RWMutex allows concurrent reads while blocking writes.
	// Use RLock() for reading blockedDomains/exceptionDomains/internalRecords
	// Use Lock() when updating these maps (in scheduler/filter initialization)
	mutex            sync.RWMutex
	blockedDomains   = make(map[string]bool)
	exceptionDomains = make(map[string]bool)
	internalRecords  = make(map[string]string)
	localIp, _       = gatesentryDnsUtils.GetLocalIP()
	queryLogs        = make(map[string][]QueryLog)
	logMutex         sync.Mutex
	logsFile         *os.File
	fileMutex        sync.Mutex
	logsPath         = "dns_logs.txt"
	logger           *gatesentryLogger.Log
)

func init() {
	// Load configuration from environment variables
	if envAddr := os.Getenv(ENV_DNS_LISTEN_ADDR); envAddr != "" {
		listenAddr = envAddr
		log.Printf("[DNS] Using listen address from environment: %s", listenAddr)
	}
	if envPort := os.Getenv(ENV_DNS_LISTEN_PORT); envPort != "" {
		listenPort = envPort
		log.Printf("[DNS] Using listen port from environment: %s", listenPort)
	}
	if envResolver := os.Getenv(ENV_DNS_EXTERNAL_RESOLVER); envResolver != "" {
		externalResolver = normalizeResolver(envResolver)
		log.Printf("[DNS] Using external resolver from environment: %s", externalResolver)
	}
}

// GetListenAddr returns the current DNS listen address
func GetListenAddr() string {
	return listenAddr
}

// SetListenAddr sets the DNS listen address
func SetListenAddr(addr string) {
	if addr != "" {
		listenAddr = addr
	}
}

// GetListenPort returns the current DNS listen port
func GetListenPort() string {
	return listenPort
}

// SetListenPort sets the DNS listen port
func SetListenPort(port string) {
	if port != "" {
		listenPort = port
	}
}

func SetExternalResolver(resolver string) {
	if resolver != "" {
		externalResolver = normalizeResolver(resolver)
	}
}

// GetExternalResolver returns the resolver used for upstream DNS lookups.
func GetExternalResolver() string {
	return externalResolver
}

var server *dns.Server        // UDP server
var tcpServer *dns.Server     // TCP server for large queries (>512 bytes)
var serverRunning atomic.Bool // Thread-safe flag for server state
var restartDnsSchedulerChan chan bool

// deviceStore is the central device inventory and DNS record store.
// Discovery sources populate it; handleDNSRequest reads from it.
// Initialized in StartDNSServer().
var deviceStore *discovery.DeviceStore

// TailscaleManager is the lifecycle surface used by the authenticated API.
// Keeping this interface at the integration boundary lets handler tests supply
// deterministic manager state without contacting the host tailscaled socket.
type TailscaleManager interface {
	Snapshot() gatesentryTailscale.ManagerSnapshot
	Peers() []gatesentryTailscale.Peer
	Peer(string) (gatesentryTailscale.Peer, bool)
	SetEnabled(context.Context, bool) error
	Stop()
}

type tailscaleManagerLifecycle struct {
	mu      sync.RWMutex
	manager TailscaleManager
	stopped bool
}

func (l *tailscaleManagerLifecycle) get() TailscaleManager {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.manager
}

// publish installs a newly constructed manager unless shutdown has begun. The
// stopped flag is permanent because policy initialization is process-lifetime
// and guarded by policyStartOnce.
func (l *tailscaleManagerLifecycle) publish(manager TailscaleManager) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped || l.manager != nil {
		return false
	}
	l.manager = manager
	return true
}

func (l *tailscaleManagerLifecycle) stop() {
	l.mu.Lock()
	l.stopped = true
	manager := l.manager
	l.manager = nil
	l.mu.Unlock()
	if manager != nil {
		manager.Stop()
	}
}

var tailscaleManagers tailscaleManagerLifecycle

// GetTailscaleManager returns the process-lifetime Tailscale manager. It is
// initialized with the canonical device store by StartPolicyEnforcement.
func GetTailscaleManager() TailscaleManager {
	return tailscaleManagers.get()
}

// SetTailscaleManagerForTests replaces the process manager for endpoint tests.
func SetTailscaleManagerForTests(manager TailscaleManager) {
	tailscaleManagers.mu.Lock()
	tailscaleManagers.manager = manager
	tailscaleManagers.mu.Unlock()
}

// StopPolicyEnforcement releases process-lifetime integrations that are not
// owned by the DNS listener. It is called during application shutdown, not by
// StopDNSServer, because proxy-only deployments still use device policy state.
func StopPolicyEnforcement() {
	tailscaleManagers.stop()
}

// startTailscaleManager constructs before publishing so slow host detection
// does not block API reads or shutdown. If shutdown wins that race, the new
// manager is stopped without ever becoming globally visible.
func startTailscaleManager(lifecycle *tailscaleManagerLifecycle, enabled bool, newManager func() TailscaleManager) {
	manager := newManager()
	if manager == nil {
		return
	}
	if !lifecycle.publish(manager) {
		manager.Stop()
		return
	}
	if enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		if err := manager.SetEnabled(ctx, true); err != nil {
			// Manager status is the operator-facing diagnostic. Do not log raw
			// LocalAPI details or stable peer identifiers.
			log.Printf("[Tailscale] identity collection enabled but backend is unavailable")
		}
		cancel()
	}
}

// tailscaleDeviceAdapter is the only bridge between the LocalAPI model and
// discovery's linked identity model.
type tailscaleDeviceAdapter struct {
	store        *discovery.DeviceStore
	protectedIDs func() map[string]bool
}

func (a tailscaleDeviceAdapter) ApplyTailscaleSnapshot(peers []gatesentryTailscale.Peer) error {
	identities := make([]discovery.TailscaleIdentity, 0, len(peers))
	for _, peer := range peers {
		identities = append(identities, discovery.TailscaleIdentity{
			NodeID: peer.NodeID, Name: peer.Name, DNSName: peer.DNSName,
			Addresses: append([]string(nil), peer.Addresses...), Online: peer.Online,
			LastSeen: peer.LastSeen, WoLMACs: append([]string(nil), peer.WoLMACs...),
		})
	}
	var protectedIDs map[string]bool
	if a.protectedIDs != nil {
		protectedIDs = a.protectedIDs()
	}
	return a.store.ApplyTailscaleSnapshotProtected(identities, protectedIDs)
}

func (a tailscaleDeviceAdapter) ClearTailscaleObservations() {
	a.store.ClearTailscaleObservations()
}

// policyService is the dedicated policy engine used by DNS enforcement.
// Discovery owns device observations; this service owns how a resolved
// identity maps to an enforcement decision. Initialized in StartDNSServer().
var policyService *gatesentryPolicy.Service

// categoryIndex holds the downloaded per-category domain feeds. The blocklist
// refresh is the only writer; policy evaluation and the API only read it. It is
// process-wide because the refresh runs on the scheduler goroutine while
// enforcement runs on the DNS query path, and both must see one set of data.
var categoryIndex *gatesentryPolicy.CategoryIndex

// mdnsBrowser performs periodic mDNS/Bonjour scanning to discover devices.
// Initialized in StartDNSServer() when mDNS browsing is enabled.
var mdnsBrowser *discovery.MDNSBrowser

// GetDeviceStore returns the global device store for use by discovery sources,
// the API layer, and other packages. Returns nil before StartDNSServer is called.
func GetDeviceStore() *discovery.DeviceStore {
	return deviceStore
}

// deviceResolver adapts the discovery-owned device store to the policy
// service's read-only resolver interface. It never mutates device state.
type deviceResolver struct{}

// policyStaleDeviceThreshold bounds how old a device's LastSeen may be
// before its IP association is treated as stale. Every DNS query refreshes
// the observed device asynchronously (ObservePassiveQuery), so any device
// that is still active keeps itself fresh; a threshold longer than the
// discovery OnlineThreshold avoids flapping idle-but-present devices while
// still releasing a group after enough time for DHCP reassignment.
const policyStaleDeviceThreshold = 1 * time.Hour

// ResolveDeviceByIP reports the device observed at an IP, whether the
// address is ambiguous (more than one device currently indexed to it), and
// whether the winning observation is stale.
func (deviceResolver) ResolveDeviceByIP(ip string) (string, bool, bool) {
	if deviceStore == nil {
		return "", false, false
	}
	deviceID, ambiguous, lastSeen, tailscaleAlias := deviceStore.ResolveIPClaim(ip)
	if deviceID == "" {
		return "", ambiguous, false
	}
	// The claim index includes both LAN and explicitly linked Tailscale
	// addresses. A shared overlay/LAN address must fall back to the default
	// policy just like any other ambiguous client address.
	if !ambiguous && tailscaleAlias {
		return deviceID, false, false
	}
	stale := lastSeen.IsZero() || time.Since(lastSeen) > policyStaleDeviceThreshold
	return deviceID, ambiguous, stale
}

// GetPolicyService returns the DNS policy service, or nil before startup.
func GetPolicyService() *gatesentryPolicy.Service {
	return policyService
}

// GetCategoryIndex returns the downloaded category index, or nil before
// startup. The API reads it to report category coverage; policy evaluation
// reads it through the policy service.
func GetCategoryIndex() *gatesentryPolicy.CategoryIndex {
	return categoryIndex
}

// referencedCategoryIDs reports the categories policy groups select, which the
// blocklist refresh downloads alongside the gateway-wide selection. When policy
// failed to load no group exists, so nothing is referenced.
func referencedCategoryIDs() []string {
	if policyService == nil {
		return nil
	}
	return policyService.ReferencedCategories()
}

// RequestBlocklistRefresh asks the DNS scheduler to re-download blocklists and
// category feeds now instead of waiting for the next interval. It never blocks
// the caller: when a refresh is already queued or the scheduler is not running
// the request is dropped, because the periodic refresh still covers it.
func RequestBlocklistRefresh() bool {
	if restartDnsSchedulerChan == nil {
		return false
	}
	select {
	case restartDnsSchedulerChan <- true:
		return true
	default:
		return false
	}
}

// SetPolicyServiceForTests installs a policy service for handler tests. It
// exists only so the webserver can exercise its endpoints against a real
// service without starting the DNS listener; production wiring uses
// StartDNSServer.
func SetPolicyServiceForTests(svc *gatesentryPolicy.Service) {
	policyService = svc
}

// SetDeviceStoreForTests installs a device store for handler tests. It
// exists only so the webserver can exercise device endpoints against a real
// store without starting the DNS listener; production wiring uses
// StartDNSServer.
func SetDeviceStoreForTests(store *discovery.DeviceStore) {
	deviceStore = store
}

// GetMDNSBrowser returns the global mDNS browser instance, or nil if not started.
func GetMDNSBrowser() *discovery.MDNSBrowser {
	return mdnsBrowser
}

const BLOCKLIST_HOURLY_UPDATE_INTERVAL = 10

// policyStartOnce guards StartPolicyEnforcement. Policy state outlives the DNS
// listener: stopping and restarting DNS must not drop devices, policies, or
// downloaded category feeds that the proxy is still enforcing.
var policyStartOnce sync.Once

// StartPolicyEnforcement sets up the device store, the policy service, the
// category index, and the blocklist refresh. The proxy enforces policies too,
// so this runs at startup whether or not the DNS listener is enabled; a
// gateway used only as a proxy still blocks a policy's categories. It is safe
// to call more than once.
func StartPolicyEnforcement(basePath string, blockedLists []string, settings *gatesentry2storage.MapStore, devices *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo) {
	policyStartOnce.Do(func() {
		readSetting := func(key, conservativeFallback string) string {
			value, err := settings.GetE(key)
			if err != nil {
				log.Printf("[DNS] storage error while reading %q; retaining known-good value: %v", key, err)
				if value == "" {
					return conservativeFallback
				}
			}
			return value
		}
		// Initialize the device store with configured zones (default: "local").
		// Supports multiple comma-separated zones for split-horizon DNS.
		// Example: "jvj28.com,local" → devices resolve as both
		//   macmini.jvj28.com AND macmini.local
		// The first zone is the primary (used for PTR targets).
		zones := localZones(readSetting("dns_local_zone", "local"))
		deviceStore = discovery.NewDeviceStoreMultiZone(zones...)
		log.Printf("[DNS] Device store initialized with zones: %v (primary: %s)", zones, zones[0])
		// Load policy before attaching legacy device assignments. Attachment may
		// prune transient v1 records, and records referenced by an assignment,
		// pause, or exception must survive that migration.
		policySvc, policyErr := gatesentryPolicy.NewService(settings, deviceResolver{})
		if policyErr != nil {
			log.Printf("[DNS] Policy service unavailable, retaining default enforcement: %v", policyErr)
			// Without policy references it is unsafe to run destructive legacy
			// device migration. Leave persistence unattached rather than deleting
			// a record that policy state may still reference.
			if devices != nil {
				log.Printf("[DNS] Device persistence disabled because protected policy references are unavailable")
			}
		} else {
			if devices != nil {
				if err := deviceStore.AttachPersistenceWithProtectedIDs(devices, policySvc.ReferencedDeviceIDs()); err != nil {
					log.Printf("[DNS] Device persistence disabled after load error: %v", err)
				}
			}
			// Category rules are only as good as the downloaded feeds. The index is
			// attached before the scheduler starts so the first refresh fills it and
			// enforcement never reads a partially built map.
			categoryIndex = gatesentryPolicy.NewCategoryIndex()
			policySvc.SetCategoryIndex(categoryIndex)
			if err := migrateLegacyPolicy(policySvc, settings); err != nil {
				log.Printf("[DNS] Legacy configuration migration unavailable, retaining default enforcement: %v", err)
			}
			if err := migrateLegacyBlockList(policySvc, basePath); err != nil {
				log.Printf("[DNS] Legacy block list migration unavailable; the proxy keeps enforcing it: %v", err)
			}
			policyService = policySvc
		}

		// Start the process-lifetime manager only after policy and persistence
		// share the canonical store. Collection remains disabled by default.
		startTailscaleManager(
			&tailscaleManagers,
			readSetting("tailscale_identity_enabled", "false") == "true",
			func() TailscaleManager {
				return gatesentryTailscale.NewManager(
					gatesentryTailscale.NewClient(),
					tailscaleDeviceAdapter{
						store: deviceStore,
						protectedIDs: func() map[string]bool {
							if policyService == nil {
								return nil
							}
							return policyService.ReferencedDeviceIDs()
						},
					},
					0,
				)
			},
		)

		restartDnsSchedulerChan = make(chan bool)

		go gatesentryDnsScheduler.RunScheduler(
			&blockedDomains,
			&blockedLists,
			&internalRecords,
			&exceptionDomains,
			&mutex,
			settings,
			dnsinfo,
			categoryIndex,
			referencedCategoryIDs,
			BLOCKLIST_HOURLY_UPDATE_INTERVAL,
			restartDnsSchedulerChan,
		)
		restartDnsSchedulerChan <- true
	})
}

// localZones parses the comma-separated dns_local_zone setting, falling back
// to "local" when it names nothing.
func localZones(setting string) []string {
	var zones []string
	for _, z := range strings.Split(setting, ",") {
		z = strings.TrimSpace(z)
		if z != "" {
			zones = append(zones, z)
		}
	}
	if len(zones) == 0 {
		zones = []string{"local"}
	}
	return zones
}

func StartDNSServer(basePath string, ilogger *gatesentryLogger.Log, blockedLists []string, settings *gatesentry2storage.MapStore, devices *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo) {

	if server != nil || serverRunning.Load() {
		fmt.Println("DNS server is already running")
		restartDnsSchedulerChan <- true
		return
	}
	StartPolicyEnforcement(basePath, blockedLists, settings, devices, dnsinfo)
	logger = ilogger
	logsPath = basePath + logsPath
	readSetting := func(key, conservativeFallback string) string {
		value, err := settings.GetE(key)
		if err != nil {
			log.Printf("[DNS] storage error while reading %q; retaining known-good value: %v", key, err)
			if value == "" {
				return conservativeFallback
			}
		}
		return value
	}
	SetExternalResolver(readSetting("dns_resolver", "8.8.8.8:53"))
	// The device store outlives the listener, so a zone changed while DNS was
	// stopped is applied when it starts again.
	deviceStore.SetZones(localZones(readSetting("dns_local_zone", "local")))
	// InitializeLogs()
	// go gatesentryDnsFilter.InitializeBlockedDomains(&blockedDomains, &blockedLists)

	// Start mDNS/Bonjour browser for automatic device discovery (Phase 3).
	// Browses common service types (_airplay._tcp, _googlecast._tcp, _printer._tcp, etc.)
	// and feeds discovered devices into the device store.
	// Enabled by default. Set setting "mdns_browser_enabled" to "false" to disable.
	mdnsEnabled := readSetting("mdns_browser_enabled", "false")
	if mdnsEnabled != "false" {
		mdnsBrowser = discovery.NewMDNSBrowser(deviceStore, discovery.DefaultScanInterval)
		mdnsBrowser.Start()
	}

	// Configure DDNS (Phase 4: RFC 2136 Dynamic DNS UPDATE handler).
	// Settings: ddns_enabled, ddns_tsig_required, ddns_tsig_key_name,
	//           ddns_tsig_key_secret, ddns_tsig_algorithm
	ddnsEnabledStr := readSetting("ddns_enabled", "false")
	if ddnsEnabledStr == "false" {
		ddnsEnabled = false
	} else {
		ddnsEnabled = true
	}

	ddnsTSIGRequiredStr := readSetting("ddns_tsig_required", "true")
	if ddnsTSIGRequiredStr == "true" {
		ddnsTSIGRequired = true
	} else {
		ddnsTSIGRequired = false
	}

	// Build TSIG secret map for server-level TSIG verification.
	// The miekg/dns server automatically verifies TSIG on incoming messages
	// when TsigSecret is set, and exposes the result via w.TsigStatus().
	var tsigSecrets map[string]string
	tsigKeyName := readSetting("ddns_tsig_key_name", "")
	tsigKeySecret := readSetting("ddns_tsig_key_secret", "")
	if tsigKeyName != "" && tsigKeySecret != "" {
		if !strings.HasSuffix(tsigKeyName, ".") {
			tsigKeyName += "."
		}
		tsigSecrets = map[string]string{tsigKeyName: tsigKeySecret}
		log.Printf("[DDNS] TSIG configured: key=%s", strings.TrimSuffix(tsigKeyName, "."))
	}

	if ddnsEnabled {
		log.Printf("[DDNS] Dynamic DNS updates enabled (TSIG required: %v)", ddnsTSIGRequired)
	} else {
		log.Println("[DDNS] Dynamic DNS updates disabled")
	}

	serverRunning.Store(true)
	// go PrintQueryLogsPeriodically()
	// Listen for incoming DNS requests on configured address:port (default: 0.0.0.0:53)
	// Use net.JoinHostPort to properly handle IPv6 addresses (adds brackets)
	bindAddr := net.JoinHostPort(listenAddr, listenPort)

	// Start TCP server in a goroutine for large DNS queries (>512 bytes)
	// TCP is required for DNSSEC, large TXT records, zone transfers, etc.
	// MsgAcceptFunc is overridden to accept UPDATE opcode (default rejects it).
	// TsigSecret enables server-level TSIG verification for DDNS.
	tcpServer = &dns.Server{
		Addr:          bindAddr,
		Net:           "tcp",
		MsgAcceptFunc: ddnsMsgAcceptFunc,
		TsigSecret:    tsigSecrets,
	}
	tcpServer.Handler = dns.HandlerFunc(handleDNSRequest)
	go func() {
		fmt.Printf("DNS forwarder listening on %s (TCP). Handles large queries >512 bytes.\n", bindAddr)
		if err := tcpServer.ListenAndServe(); err != nil {
			log.Printf("[DNS] TCP server error: %v", err)
		}
	}()

	// Start UDP server (blocks)
	server = &dns.Server{
		Addr:          bindAddr,
		Net:           "udp",
		MsgAcceptFunc: ddnsMsgAcceptFunc,
		TsigSecret:    tsigSecrets,
	}
	server.Handler = dns.HandlerFunc(handleDNSRequest)

	fmt.Printf("DNS forwarder listening on %s (UDP). Local IP: %s. External resolver: %s\n", bindAddr, localIp, externalResolver)
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
		// os.Exit(1)
		return
	}

}

func StopDNSServer() {
	if server == nil || !serverRunning.Load() {
		fmt.Println("DNS server is already stopped")
		return
	}

	// Stop mDNS browser if running
	if mdnsBrowser != nil {
		mdnsBrowser.Stop()
		mdnsBrowser = nil
	}

	// Stop TCP server if running
	if tcpServer != nil {
		if err := tcpServer.Shutdown(); err != nil {
			log.Printf("[DNS] Error shutting down TCP server: %v", err)
		}
		tcpServer = nil
	}

	// Stop UDP server
	if server != nil {
		if err := server.Shutdown(); err != nil {
			log.Printf("[DNS] Error shutting down UDP server: %v", err)
		}
		server = nil
	}

	serverRunning.Store(false)
}

// AddBlockedDomainForTest adds one domain to the in-memory blocked map.
// The name is scoped to tests and diagnostics; the scheduler still owns
// blocklist-initialized entries.
func AddBlockedDomainForTest(domain string) {
	mutex.Lock()
	defer mutex.Unlock()
	blockedDomains[strings.ToLower(domain)] = true
}

// RemoveBlockedDomainForTest removes one domain from the blocked map.
func RemoveBlockedDomainForTest(domain string) {
	mutex.Lock()
	defer mutex.Unlock()
	delete(blockedDomains, strings.ToLower(domain))
}

// policyMatchLabel renders the group rule that decided a query for logs and
// explanations. A category match is reported by the operator-facing category
// name, because "category:social" alone does not tell an administrator what
// list was applied.
func policyMatchLabel(decision gatesentryPolicy.DNSDecision) string {
	matched := decision.MatchedDomain
	if id, ok := strings.CutPrefix(matched, "category:"); ok {
		matched = categoryMatchLabel(id)
	}
	if matched == "*" {
		matched = "all traffic"
	}
	label := "policy " + decision.GroupID + ": " + matched
	if decision.RuleID != "" {
		label += " (rule " + decision.RuleID + ")"
	}
	return label
}

// categoryMatchLabel names a category for logs and explanations. The ID alone
// does not tell an administrator what list was applied, and the catalog name is
// what the UI showed when the category was selected.
func categoryMatchLabel(categoryID string) string {
	if category, found := gatesentryPolicy.GetCategory(categoryID); found {
		return "category " + category.Name
	}
	return "category " + categoryID
}

// dnsDecision builds a structured DNS filtering decision for logging.
// It stamps the time, attaches identity and policy revision, and fills the
// adapter-native ResponseType so legacy log/stat viewers keep working.
func dnsDecision(action gatesentryPolicy.DecisionAction, domain, clientIP, responseLabel, matchedRule, reason string, identity gatesentryPolicy.Identity, revision int) gatesentryPolicy.Decision {
	d := gatesentryPolicy.NewDecision(action, gatesentryPolicy.LayerDNS, domain).
		WithIdentity(identity).WithPolicyRevision(revision)
	d.ClientIP = clientIP
	d.ResponseType = responseLabel
	d.MatchedRule = matchedRule
	d.Reason = reason
	return d
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	// Check if server is running (atomic read - no lock needed)
	if !serverRunning.Load() {
		log.Println("DNS server is not running")
		w.Close()
		return
	}

	// Route DDNS UPDATE messages to the dedicated handler.
	// UPDATE messages have a different structure (zone section, update section)
	// and are handled entirely separately from standard queries.
	if r.Opcode == dns.OpcodeUpdate {
		handleDDNSUpdate(w, r)
		return
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	clientIP := discovery.ExtractClientIP(w.RemoteAddr())

	// Passive discovery: record that we saw a query from this client IP.
	// Runs in a goroutine to avoid adding latency to DNS responses.
	// The device store handles deduplication and MAC correlation internally.
	if deviceStore != nil {
		if clientIP != "" {
			go deviceStore.ObservePassiveQuery(clientIP)
		}
	}

	for _, q := range r.Question {
		domain := strings.ToLower(q.Name)
		domain = domain[:len(domain)-1] // Strip trailing dot

		// Resolve identity once for decision provenance across all
		// enforcement paths below. ResolveIdentityForDNS treats a live
		// query as evidence that the address is active.
		var dnsIdentity gatesentryPolicy.Identity
		var policyRevision int
		if policyService != nil {
			dnsIdentity = policyService.ResolveIdentityForDNS(clientIP, "")
			policyRevision = policyService.Snapshot().Version
		}

		// --- 1. Device store lookup (supports A, AAAA, PTR) ---
		// The device store has its own RWMutex — no need to hold the shared mutex.
		if deviceStore != nil {
			var records []discovery.DnsRecord

			// PTR queries: check reverse lookup index
			if q.Qtype == dns.TypePTR && isReverseDomain(domain) {
				records = deviceStore.LookupReverse(domain)
			} else {
				// Forward queries: A, AAAA, or ANY
				records = deviceStore.LookupName(domain, q.Qtype)
			}

			if len(records) > 0 {
				log.Printf("[DNS] Device store hit: %s %s (%d records)",
					domain, dns.TypeToString[q.Qtype], len(records))
				response := new(dns.Msg)
				response.SetRcode(r, dns.RcodeSuccess)
				response.Authoritative = true
				for _, rec := range records {
					rr := rec.ToRR()
					if rr != nil {
						response.Answer = append(response.Answer, rr)
					}
				}
				logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionAllow,
					domain, clientIP, "device", "device store", "local device record", dnsIdentity, policyRevision))
				w.WriteMsg(response)
				return
			}
		}

		// --- 2. Legacy path: exception / internal / blocked ---
		// Use read lock — allows concurrent DNS queries while blocking filter updates
		mutex.RLock()
		internalRecordsLen := len(internalRecords)
		isException := exceptionDomains[domain]
		internalIP, isInternal := internalRecords[domain]
		isBlocked := blockedDomains[domain]
		mutex.RUnlock()

		// Gateway-wide categories are evaluated with the client's policy below,
		// so a policy's allowed domains can exempt a category hit the same way
		// they exempt the blocklist, on DNS and on the proxy alike.
		blockLabel := "blocklist"
		blockReason := "global blocklist"

		log.Println("[DNS] Domain requested:", domain, " Length of internal records = ", internalRecordsLen)

		if isException {
			log.Println("Domain is exception : ", domain)
			logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionBypass,
				domain, clientIP, "exception", "exception", "exception domain", dnsIdentity, policyRevision))
		} else if isInternal {
			log.Println("Domain is internal : ", domain, " - ", internalIP)
			response := new(dns.Msg)
			response.SetRcode(r, dns.RcodeSuccess)
			response.Answer = append(response.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(internalIP),
			})
			logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionAllow,
				domain, clientIP, "internal", "internal record", "internal record", dnsIdentity, policyRevision))
			w.WriteMsg(response)
			return
		}

		// --- 2.5 Policy enforcement ---
		// Every client resolves to a policy (its own or the default). The
		// policy's rules and lists decide first; a policy allow exempts the
		// domain from the global blocklist, a policy block answers with a block,
		// and no opinion leaves the global blocklist in charge. Rules the proxy
		// has to decide (URL, response type, proxy user) never decide here.
		policyDecision := gatesentryPolicy.DNSDecision{}
		if policyService != nil {
			policyDecision = policyService.EvaluateDNS(dnsIdentity, domain)
			if policyDecision.Action == gatesentryPolicy.ActionBlock {
				log.Printf("[DNS] Domain blocked by policy %s: %s (%s)", policyDecision.GroupID, domain, policyDecision.Reason)
				logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionBlock,
					domain, clientIP, "blocked",
					policyMatchLabel(policyDecision),
					policyDecision.Reason, dnsIdentity, policyRevision))
				response := new(dns.Msg)
				response.SetRcode(r, dns.RcodeNameError)
				response.Answer = append(response.Answer, &dns.CNAME{
					Hdr:    dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: blockedAnswerTTL},
					Target: "blocked.local.",
				})
				w.WriteMsg(response)
				return
			}
			if policyDecision.Action == gatesentryPolicy.ActionAllow && isBlocked {
				log.Printf("[DNS] Domain allowed by policy %s: %s", policyDecision.GroupID, domain)
				logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionBypass,
					domain, clientIP, "exception",
					policyMatchLabel(policyDecision),
					policyDecision.Reason+" exempts the blocklist", dnsIdentity, policyRevision))
			}
		}
		if policyDecision.Action != gatesentryPolicy.ActionAllow && isBlocked {
			log.Println("[DNS] Domain is blocked : ", domain)
			response := new(dns.Msg)
			response.SetRcode(r, dns.RcodeNameError)
			response.Answer = append(response.Answer, &dns.CNAME{
				Hdr:    dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: blockedAnswerTTL},
				Target: "blocked.local.",
			})
			logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionBlock,
				domain, clientIP, "blocked", blockLabel, blockReason, dnsIdentity, policyRevision))
			w.WriteMsg(response)
			return
		}

		// --- 2.6 Safe search ---
		// A policy with safe search answers the search engines' host names
		// with their restricted-mode endpoints. The target is resolved
		// upstream, so the answer follows the provider's current addresses.
		if policyDecision.SafeSearch {
			if target := gatesentryPolicy.SafeSearchTarget(domain); target != "" {
				response, err := safeSearchResponse(r, q, target)
				if err != nil {
					// Failing closed: answering with the unrestricted address
					// would silently turn safe search off.
					log.Printf("[DNS] Safe search lookup for %s failed: %v", target, err)
					response = new(dns.Msg)
					response.SetRcode(r, dns.RcodeServerFailure)
				} else {
					logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionAllow,
						domain, clientIP, "safesearch", "safe search", "rewritten to "+target, dnsIdentity, policyRevision))
				}
				w.WriteMsg(response)
				return
			}
		}
		logger.LogDecision(dnsDecision(gatesentryPolicy.ActionDecisionAllow,
			domain, clientIP, "forward", "", "no matching rule", dnsIdentity, policyRevision))

		// --- 3. Forward to external resolver ---
		// Forward request WITHOUT holding the mutex - this is the key fix!
		// External DNS queries can take time and should not block other requests
		// Detect if client connected via TCP and preserve that for forwarding
		useTCP := w.LocalAddr().Network() == "tcp"
		resp, err := forwardDNSRequest(r, useTCP)
		if err != nil {
			log.Println("[DNS] Error forwarding DNS request:", err)
			// Send SERVFAIL response instead of silently dropping the request.
			errMsg := new(dns.Msg)
			errMsg.SetRcode(r, dns.RcodeServerFailure)
			w.WriteMsg(errMsg)
			return
		}

		for _, answer := range resp.Answer {
			m.Answer = append(m.Answer, answer)
		}
	}
	w.WriteMsg(m)
}

// blockedAnswerTTL is the lifetime of a block answer. A short TTL is what lets
// a schedule or a pause take effect within a minute instead of whenever the
// client's cache expires.
const blockedAnswerTTL = 60

// safeSearchResponse answers a query for a search engine host with the
// provider's restricted-mode endpoint: a CNAME to the target followed by the
// target's own records, resolved upstream so they are always current.
func safeSearchResponse(r *dns.Msg, q dns.Question, target string) (*dns.Msg, error) {
	lookup := new(dns.Msg)
	lookup.SetQuestion(dns.Fqdn(target), q.Qtype)
	lookup.RecursionDesired = true
	upstream, err := forwardDNSRequest(lookup, false)
	if err != nil {
		return nil, err
	}
	if upstream.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("upstream answered %s", dns.RcodeToString[upstream.Rcode])
	}
	response := new(dns.Msg)
	response.SetReply(r)
	response.RecursionAvailable = true
	response.Answer = append(response.Answer, &dns.CNAME{
		Hdr:    dns.RR_Header{Name: q.Name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 300},
		Target: dns.Fqdn(target),
	})
	response.Answer = append(response.Answer, upstream.Answer...)
	return response, nil
}

// isReverseDomain returns true if the domain is a PTR reverse-lookup name.
func isReverseDomain(domain string) bool {
	return strings.HasSuffix(domain, ".in-addr.arpa") ||
		strings.HasSuffix(domain, ".ip6.arpa")
}

func forwardDNSRequest(r *dns.Msg, useTCP bool) (*dns.Msg, error) {
	c := new(dns.Client)
	c.Timeout = 3 * time.Second // Explicit timeout to prevent hanging under concurrent load

	// Use TCP if requested (e.g., client connected via TCP)
	if useTCP {
		c.Net = "tcp"
	}

	resp, _, err := c.Exchange(r, externalResolver)
	if err != nil {
		return nil, err
	}

	// If response is truncated and we used UDP, retry with TCP
	// This handles cases where upstream response is too large for UDP
	if resp.Truncated && !useTCP {
		log.Println("[DNS] Response truncated, retrying with TCP")
		c.Net = "tcp"
		tcpResp, _, tcpErr := c.Exchange(r, externalResolver)
		if tcpErr != nil {
			// TCP retry failed, return the truncated UDP response
			log.Println("[DNS] TCP retry failed:", tcpErr)
			return resp, nil
		}
		return tcpResp, nil
	}

	return resp, nil
}

// function that accepts two strings : domain and ip and returns an A record
func GetARecord(domain string, ip string) *dns.A {
	return &dns.A{
		Hdr: dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 3600},
		A:   net.ParseIP(ip),
	}
}

// function that accepts two strings : domain and ip and returns a TXT record
func GetTXTRecord(domain string, txt string) *dns.TXT {
	return &dns.TXT{
		Hdr: dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 3600},
		Txt: []string{txt},
	}
}

// function that accepts two strings : domain and ip and returns a CNAME record
func GetCNAMERecord(domain string, cname string) *dns.CNAME {
	return &dns.CNAME{
		Hdr:    dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 3600},
		Target: cname + ".",
	}
}
