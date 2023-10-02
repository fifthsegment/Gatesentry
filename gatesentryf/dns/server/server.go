package gatesentryDnsServer

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	gatesentryDnsHttpServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/http"
	gatesentryDnsScheduler "bitbucket.org/abdullah_irfan/gatesentryf/dns/scheduler"
	gatesentryDnsUtils "bitbucket.org/abdullah_irfan/gatesentryf/dns/utils"
	gatesentryLogger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
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
	internalRecords  = make(map[string][]dns.RR)
	localIp, _       = gatesentryDnsUtils.GetLocalIP()
	queryLogs        = make(map[string][]QueryLog)
	logMutex         sync.Mutex
	logsFile         *os.File
	fileMutex        sync.Mutex
	logsPath         = "dns_logs.txt"
	logger           *gatesentryLogger.Log
)

func StartDNSServer(basePath string, ilogger *gatesentryLogger.Log, blockedLists []string) {
	logger = ilogger
	logsPath = basePath + logsPath
	go gatesentryDnsHttpServer.StartHTTPServer()
	InitializeLogs()
	// go gatesentryDnsFilter.InitializeBlockedDomains(&blockedDomains, &blockedLists)
	go gatesentryDnsScheduler.RunScheduler(
		&blockedDomains,
		&blockedLists,
		&internalRecords,
		&exceptionDomains,
		&mutex,
	)
	go PrintQueryLogsPeriodically()
	// Listen for incoming DNS requests on port 53
	server := &dns.Server{Addr: "0.0.0.0:53", Net: "udp"}
	server.Handler = dns.HandlerFunc(handleDNSRequest)
	fmt.Println("DNS forwarder listening on :53 . Binded on : ", localIp)
	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
		return
	}
}

func handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	mutex.Lock()
	defer mutex.Unlock()

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true

	for _, q := range r.Question {
		domain := strings.ToLower(q.Name)
		fmt.Println("Domain requested:", domain)
		domain = domain[:len(domain)-1]
		LogQuery(domain)
		logger.LogDNS(domain, "dns")
		if _, exists := exceptionDomains[domain]; exists {
			fmt.Println("Domain is exception : ", domain)
		} else if records, exists := internalRecords[domain]; exists {
			fmt.Println("Domain is internal : ", domain)
			m.Answer = append(m.Answer, records...)
		} else if blockedDomains[domain] {
			fmt.Println("Domain is blocked : ", domain)
			response := new(dns.Msg)
			response.SetRcode(r, dns.RcodeNameError)
			response.Answer = append(response.Answer, &dns.CNAME{
				Hdr:    dns.RR_Header{Name: domain + ".", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 3600},
				Target: "gatesentryfilter.abdullahirfan.com.",
			})

			w.WriteMsg(response)
			return
		}

		resp, err := forwardDNSRequest(r)
		if err != nil {
			fmt.Println("Error forwarding DNS request:", err)
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
