package gatesentryDnsServer

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	gatesentryDnsHttpServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/http"
	gatesentryDnsScheduler "bitbucket.org/abdullah_irfan/gatesentryf/dns/scheduler"
	gatesentryDnsUtils "bitbucket.org/abdullah_irfan/gatesentryf/dns/utils"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	"github.com/miekg/dns"
)

type QueryLog struct {
	Domain string
	Time   time.Time
}

var (
	externalResolver = "8.8.8.8:53"
	mutex            sync.Mutex // Mutex to control access to blockedDomains
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

var server *dns.Server
var serverRunning bool = false

func StartDNSServer(basePath string, ilogger *gatesentryLogger.Log, blockedLists []string, settings *gatesentry2storage.MapStore) {

	if server != nil || serverRunning == true {
		fmt.Println("DNS server is already running")
		return
	}

	logger = ilogger
	logsPath = basePath + logsPath
	go gatesentryDnsHttpServer.StartHTTPServer()
	// InitializeLogs()
	// go gatesentryDnsFilter.InitializeBlockedDomains(&blockedDomains, &blockedLists)
	go gatesentryDnsScheduler.RunScheduler(
		&blockedDomains,
		&blockedLists,
		&internalRecords,
		&exceptionDomains,
		&mutex,
		settings,
	)
	serverRunning = true
	// go PrintQueryLogsPeriodically()
	// Listen for incoming DNS requests on port 53
	server = &dns.Server{Addr: "0.0.0.0:53", Net: "udp"}
	server.Handler = dns.HandlerFunc(handleDNSRequest)

	fmt.Println("DNS forwarder listening on :53 . Binded on : ", localIp)
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
		// os.Exit(1)
		return
	}

}

func StopDNSServer() {
	// if server == nil || serverRunning == false {
	if server == nil || serverRunning == false {
		fmt.Println("DNS server is already stopped")

		return
	}

	gatesentryDnsHttpServer.StopHTTPServer()
	serverRunning = false
	server = nil
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	mutex.Lock()
	defer mutex.Unlock()

	// send an error if the server is not running
	if serverRunning == false {
		fmt.Println("DNS server is not running")
		w.Close()
		w.Hijack()
		return
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		domain := strings.ToLower(q.Name)
		log.Println("[DNS] Domain requested:", domain)
		domain = domain[:len(domain)-1]
		// LogQuery(domain)
		if _, exists := exceptionDomains[domain]; exists {
			log.Println("Domain is exception : ", domain)
			logger.LogDNS(domain, "dns", "exception")

		} else if ip, exists := internalRecords[domain]; exists {
			log.Println("Domain is internal : ", domain, " - ", ip)
			response := new(dns.Msg)
			response.SetRcode(r, dns.RcodeSuccess)
			response.Answer = append(m.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 3600},
				A:   net.ParseIP(ip),
			})

			logger.LogDNS(domain, "dns", "internal")
			w.WriteMsg(response)
			return
		} else if blockedDomains[domain] {
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
	resp, _, err := c.Exchange(r, externalResolver) // Use Google's public DNS as an example
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
