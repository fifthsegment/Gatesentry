package gatesentryDnsServer

import (
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
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
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

// Phase 4: DNS response cache — keyed by (qname, qtype), TTL-aware.
// Reduces upstream queries from ~20ms per lookup to <1ms for cached entries.
type dnsCacheEntry struct {
	msg       *dns.Msg  // cached response (deep-copied on insert)
	expiresAt time.Time // absolute expiry based on minimum TTL
}

var (
	dnsCache    = make(map[string]*dnsCacheEntry)
	dnsCacheMu  sync.RWMutex
	dnsCacheMax = 10000 // max entries before eviction
)

// dnsCacheKey builds a cache key from question name and type.
func dnsCacheKey(qname string, qtype uint16) string {
	t := dns.TypeToString[qtype]
	if t == "" {
		// Fall back to numeric qtype for unknown/unsupported types to avoid key collisions.
		return strings.ToLower(qname) + "/" + fmt.Sprint(qtype)
	}
	return strings.ToLower(qname) + "/" + t
}

// dnsCacheGet returns a cached response if it exists and hasn't expired.
// The returned message has TTLs decremented to reflect elapsed time.
func dnsCacheGet(qname string, qtype uint16) *dns.Msg {
	key := dnsCacheKey(qname, qtype)
	dnsCacheMu.RLock()
	entry, ok := dnsCache[key]
	dnsCacheMu.RUnlock()
	if !ok {
		return nil
	}

	remaining := time.Until(entry.expiresAt)
	if remaining <= 0 {
		// Expired — remove lazily
		dnsCacheMu.Lock()
		delete(dnsCache, key)
		dnsCacheMu.Unlock()
		return nil
	}

	// Deep-copy the cached message and adjust TTLs
	msg := entry.msg.Copy()
	ttlSec := uint32(remaining.Seconds())
	for _, rr := range msg.Answer {
		rr.Header().Ttl = ttlSec
	}
	for _, rr := range msg.Ns {
		rr.Header().Ttl = ttlSec
	}
	for _, rr := range msg.Extra {
		if rr.Header().Rrtype != dns.TypeOPT {
			rr.Header().Ttl = ttlSec
		}
	}
	return msg
}

// dnsCachePut stores a DNS response in the cache. TTL is taken from the
// minimum TTL across all answer/authority records (minimum 5s, maximum 1h).
func dnsCachePut(qname string, qtype uint16, msg *dns.Msg) {
	if msg == nil {
		return
	}

	// Find minimum TTL across all records
	var minTTL uint32 = 3600 // 1 hour cap
	found := false
	for _, rr := range msg.Answer {
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
			found = true
		}
	}
	for _, rr := range msg.Ns {
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
			found = true
		}
	}
	if !found {
		// No records with TTL — cache for 60s (negative responses, etc.)
		minTTL = 60
	}
	// Enforce minimum cache time of 5 seconds
	if minTTL < 5 {
		minTTL = 5
	}

	key := dnsCacheKey(qname, qtype)

	dnsCacheMu.Lock()
	// Incremental eviction: remove expired entries first, then oldest if still over limit
	if len(dnsCache) >= dnsCacheMax {
		now := time.Now()
		expired := 0
		for k, e := range dnsCache {
			if now.After(e.expiresAt) {
				delete(dnsCache, k)
				expired++
			}
		}
		// If still over 90% capacity after removing expired, evict 10% oldest
		if len(dnsCache) >= dnsCacheMax*9/10 {
			evictCount := dnsCacheMax / 10
			for k := range dnsCache {
				delete(dnsCache, k)
				evictCount--
				if evictCount <= 0 {
					break
				}
			}
		}
		log.Printf("[DNS Cache] Evicted %d expired + trimmed to %d entries (max %d)", expired, len(dnsCache), dnsCacheMax)
	}
	dnsCache[key] = &dnsCacheEntry{
		msg:       msg.Copy(),
		expiresAt: time.Now().Add(time.Duration(minTTL) * time.Second),
	}
	dnsCacheMu.Unlock()
}

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

var server *dns.Server        // UDP server
var tcpServer *dns.Server     // TCP server for large queries (>512 bytes)
var serverRunning atomic.Bool // Thread-safe flag for server state
var restartDnsSchedulerChan chan bool

// deviceStore is the central device inventory and DNS record store.
// Discovery sources populate it; handleDNSRequest reads from it.
// Initialized in StartDNSServer().
var deviceStore *discovery.DeviceStore

// mdnsBrowser performs periodic mDNS/Bonjour scanning to discover devices.
// Initialized in StartDNSServer() when mDNS browsing is enabled.
var mdnsBrowser *discovery.MDNSBrowser

// GetDeviceStore returns the global device store for use by discovery sources,
// the API layer, and other packages. Returns nil before StartDNSServer is called.
func GetDeviceStore() *discovery.DeviceStore {
	return deviceStore
}

// GetMDNSBrowser returns the global mDNS browser instance, or nil if not started.
func GetMDNSBrowser() *discovery.MDNSBrowser {
	return mdnsBrowser
}

const BLOCKLIST_HOURLY_UPDATE_INTERVAL = 10

func StartDNSServer(basePath string, ilogger *gatesentryLogger.Log, blockedLists []string, settings *gatesentry2storage.MapStore, dnsinfo *gatesentryTypes.DnsServerInfo) {

	if server != nil || serverRunning.Load() {
		fmt.Println("DNS server is already running")
		restartDnsSchedulerChan <- true
		return
	}

	logger = ilogger
	logsPath = basePath + logsPath
	SetExternalResolver(settings.Get("dns_resolver"))
	// InitializeLogs()
	// go gatesentryDnsFilter.InitializeBlockedDomains(&blockedDomains, &blockedLists)

	// Initialize the device store with configured zones (default: "local").
	// Supports multiple comma-separated zones for split-horizon DNS.
	// Example: "jvj28.com,local" → devices resolve as both
	//   macmini.jvj28.com AND macmini.local
	// The first zone is the primary (used for PTR targets).
	zoneSetting := settings.Get("dns_local_zone")
	if zoneSetting == "" {
		zoneSetting = "local"
	}
	// Parse comma-separated zones
	var zones []string
	for _, z := range strings.Split(zoneSetting, ",") {
		z = strings.TrimSpace(z)
		if z != "" {
			zones = append(zones, z)
		}
	}
	if len(zones) == 0 {
		zones = []string{"local"}
	}
	deviceStore = discovery.NewDeviceStoreMultiZone(zones...)
	log.Printf("[DNS] Device store initialized with zones: %v (primary: %s)", zones, zones[0])

	// Start mDNS/Bonjour browser for automatic device discovery (Phase 3).
	// Browses common service types (_airplay._tcp, _googlecast._tcp, _printer._tcp, etc.)
	// and feeds discovered devices into the device store.
	// Enabled by default. Set setting "mdns_browser_enabled" to "false" to disable.
	mdnsEnabled := settings.Get("mdns_browser_enabled")
	if mdnsEnabled != "false" {
		mdnsBrowser = discovery.NewMDNSBrowser(deviceStore, discovery.DefaultScanInterval)
		mdnsBrowser.Start()
	}

	// Configure DDNS (Phase 4: RFC 2136 Dynamic DNS UPDATE handler).
	// Settings: ddns_enabled, ddns_tsig_required, ddns_tsig_key_name,
	//           ddns_tsig_key_secret, ddns_tsig_algorithm
	ddnsEnabledStr := settings.Get("ddns_enabled")
	if ddnsEnabledStr == "false" {
		ddnsEnabled = false
	} else {
		ddnsEnabled = true
	}

	ddnsTSIGRequiredStr := settings.Get("ddns_tsig_required")
	if ddnsTSIGRequiredStr == "true" {
		ddnsTSIGRequired = true
	} else {
		ddnsTSIGRequired = false
	}

	// Build TSIG secret map for server-level TSIG verification.
	// The miekg/dns server automatically verifies TSIG on incoming messages
	// when TsigSecret is set, and exposes the result via w.TsigStatus().
	var tsigSecrets map[string]string
	tsigKeyName := settings.Get("ddns_tsig_key_name")
	tsigKeySecret := settings.Get("ddns_tsig_key_secret")
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

	restartDnsSchedulerChan = make(chan bool)

	go gatesentryDnsScheduler.RunScheduler(
		&blockedDomains,
		&blockedLists,
		&internalRecords,
		&exceptionDomains,
		&mutex,
		settings,
		dnsinfo,
		BLOCKLIST_HOURLY_UPDATE_INTERVAL,
		restartDnsSchedulerChan,
	)
	restartDnsSchedulerChan <- true

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

	// Passive discovery: record that we saw a query from this client IP.
	// Runs in a goroutine to avoid adding latency to DNS responses.
	// The device store handles deduplication and MAC correlation internally.
	if deviceStore != nil {
		clientIP := discovery.ExtractClientIP(w.RemoteAddr())
		if clientIP != "" {
			go deviceStore.ObservePassiveQuery(clientIP)
		}
	}

	for _, q := range r.Question {
		domain := strings.ToLower(q.Name)
		domain = domain[:len(domain)-1] // Strip trailing dot

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
				logger.LogDNS(domain, "dns", "device")
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

		log.Println("[DNS] Domain requested:", domain, " Length of internal records = ", internalRecordsLen)

		// LogQuery(domain)
		if isException {
			log.Println("Domain is exception : ", domain)
			logger.LogDNS(domain, "dns", "exception")

		} else if isInternal {
			log.Println("Domain is internal : ", domain, " - ", internalIP)
			response := new(dns.Msg)
			response.SetRcode(r, dns.RcodeSuccess)
			response.Answer = append(response.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP(internalIP),
			})
			logger.LogDNS(domain, "dns", "internal")
			w.WriteMsg(response)
			return
		} else if isBlocked {
			log.Println("[DNS] Domain is blocked : ", domain)
			response := new(dns.Msg)
			response.SetRcode(r, dns.RcodeNameError)
			response.Answer = append(response.Answer, &dns.CNAME{
				Hdr:    dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 3600},
				Target: "blocked.local.",
			})
			logger.LogDNS(domain, "dns", "blocked")
			w.WriteMsg(response)
			return
		} else {
			logger.LogDNS(domain, "dns", "forward")
		}

		// --- 3. Forward to external resolver (with cache) ---
		// Check cache first — avoids upstream round-trip for repeated queries.
		if cached := dnsCacheGet(q.Name, q.Qtype); cached != nil {
			cached.SetReply(r)
			cached.Authoritative = false
			w.WriteMsg(cached)
			return
		}

		// Cache miss — forward to external resolver.
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

		// Cache the upstream response for future queries.
		dnsCachePut(q.Name, q.Qtype, resp)

		// Phase 4: Propagate the upstream rcode (NXDOMAIN, NOERROR, etc.)
		// and copy answers + authority section (contains SOA for negative responses).
		m.Rcode = resp.Rcode
		for _, answer := range resp.Answer {
			m.Answer = append(m.Answer, answer)
		}
		for _, ns := range resp.Ns {
			m.Ns = append(m.Ns, ns)
		}
	}
	w.WriteMsg(m)
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
