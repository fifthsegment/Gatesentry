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

	gatesentryDnsHttpServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/http"
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

var server *dns.Server    // UDP server
var tcpServer *dns.Server // TCP server for large queries (>512 bytes)
var serverRunning atomic.Bool // Thread-safe flag for server state
var restartDnsSchedulerChan chan bool

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
	go gatesentryDnsHttpServer.StartHTTPServer()
	// InitializeLogs()
	// go gatesentryDnsFilter.InitializeBlockedDomains(&blockedDomains, &blockedLists)
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
	tcpServer = &dns.Server{Addr: bindAddr, Net: "tcp"}
	tcpServer.Handler = dns.HandlerFunc(handleDNSRequest)
	go func() {
		fmt.Printf("DNS forwarder listening on %s (TCP). Handles large queries >512 bytes.\n", bindAddr)
		if err := tcpServer.ListenAndServe(); err != nil {
			log.Printf("[DNS] TCP server error: %v", err)
		}
	}()

	// Start UDP server (blocks)
	server = &dns.Server{Addr: bindAddr, Net: "udp"}
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

	gatesentryDnsHttpServer.StopHTTPServer()

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

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		domain := strings.ToLower(q.Name)
		domain = domain[:len(domain)-1]

		// Use read lock - allows concurrent DNS queries while blocking filter updates
		// Must hold lock before reading any shared maps (including len())
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

			// msg.Answer = append(msg.Answer, &dns.A{
			// 	Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			// 	A:   net.ParseIP(ip),
			// })

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

		// Forward request WITHOUT holding the mutex - this is the key fix!
		// External DNS queries can take time and should not block other requests
		resp, err := forwardDNSRequest(r)
		if err != nil {
			log.Println("[DNS] Error forwarding DNS request:", err)
			return
		}

		for _, answer := range resp.Answer {
			m.Answer = append(m.Answer, answer)
		}
	}
	w.WriteMsg(m)
}

func forwardDNSRequest(r *dns.Msg) (*dns.Msg, error) {
	c := new(dns.Client)
	resp, _, err := c.Exchange(r, externalResolver)
	if err != nil {
		return nil, err
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
