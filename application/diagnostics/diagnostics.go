// Package diagnostics reports the health of the local GateSentry gateway.
package diagnostics

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"time"

	gatesentryDiscovery "bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryDns "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryFilters "bitbucket.org/abdullah_irfan/gatesentryf/filters"
	gatesentryPolicy "bitbucket.org/abdullah_irfan/gatesentryf/policy"
	gatesentryStorage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	"github.com/miekg/dns"
)

const (
	StatusOK        = "ok"
	StatusFailed    = "failed"
	StatusUnknown   = "unknown"
	blocklistMaxAge = 36 * time.Hour
)

type Check struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message"`
	Recovery string `json:"recovery,omitempty"`
}
type Report struct {
	Version   int       `json:"version"`
	CheckedAt time.Time `json:"checked_at"`
	Overall   string    `json:"overall"`
	Checks    []Check   `json:"checks"`
}

// Deps contains live state and test seams. A nil value produces an explicit
// unknown check rather than silently claiming that a subsystem is healthy.
type Deps struct {
	Now           func() time.Time
	Settings      *gatesentryStorage.MapStore
	DnsInfo       *gatesentryTypes.DnsServerInfo
	Filters       *[]gatesentryFilters.GSFilter
	Devices       *gatesentryDiscovery.DeviceStore
	Policy        *gatesentryPolicy.Service
	GetDevices    func() *gatesentryDiscovery.DeviceStore
	GetPolicy     func() *gatesentryPolicy.Service
	ListenAddr    string
	ListenPort    string
	Upstream      string
	Listener      func(context.Context, string, string) error
	UpstreamCheck func(context.Context, string) error
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now().UTC()
	}
	return time.Now().UTC()
}

func Run(d Deps) Report {
	r := Report{Version: 1, CheckedAt: d.now()}
	r.Checks = []Check{listenerCheck(d), upstreamCheck(d), blocklistCheck(d), certificateCheck(d), storageCheck(d), discoveryCheck(d), policyCheck(d)}
	r.Overall = StatusOK
	for _, c := range r.Checks {
		if c.Status == StatusFailed {
			r.Overall = StatusFailed
			break
		}
		if c.Status == StatusUnknown && r.Overall == StatusOK {
			r.Overall = StatusUnknown
		}
	}
	return r
}

func result(name, status, message, recovery string) Check {
	return Check{Name: name, Status: status, Message: message, Recovery: recovery}
}
func unknown(name, message, recovery string) Check {
	return result(name, StatusUnknown, message, recovery)
}

func listenerCheck(d Deps) Check {
	if d.ListenAddr == "" {
		d.ListenAddr = gatesentryDns.GetListenAddr()
	}
	if d.ListenPort == "" {
		d.ListenPort = gatesentryDns.GetListenPort()
	}
	if d.Listener == nil {
		d.Listener = defaultListenerCheck
	}
	if err := d.Listener(context.Background(), d.ListenAddr, d.ListenPort); err != nil {
		return result("listeners", StatusFailed, fmt.Sprintf("DNS listener %s:%s is not reachable: %v", d.ListenAddr, d.ListenPort, err), "Check the configured DNS address and port, then restart GateSentry or stop the process holding the port.")
	}
	return result("listeners", StatusOK, fmt.Sprintf("DNS listener is reachable at %s:%s.", d.ListenAddr, d.ListenPort), "")
}

func defaultListenerCheck(ctx context.Context, addr, port string) error {
	q := new(dns.Msg)
	q.SetQuestion("1.0.0.127.in-addr.arpa.", dns.TypePTR)
	client := &dns.Client{Timeout: 2 * time.Second}
	_, _, err := client.ExchangeContext(ctx, q, net.JoinHostPort(loopbackHost(addr), port))
	return err
}

func loopbackHost(addr string) string {
	if addr == "" || addr == "0.0.0.0" || addr == "::" || addr == "[::]" {
		return "127.0.0.1"
	}
	return addr
}

func upstreamCheck(d Deps) Check {
	if d.Upstream == "" {
		d.Upstream = gatesentryDns.GetExternalResolver()
	}
	if d.UpstreamCheck == nil {
		d.UpstreamCheck = defaultUpstreamCheck
	}
	if err := d.UpstreamCheck(context.Background(), d.Upstream); err != nil {
		return result("upstream", StatusFailed, fmt.Sprintf("Upstream resolver %s did not answer: %v", d.Upstream, err), "Set a reachable DNS resolver in settings and verify outbound UDP/TCP DNS access.")
	}
	return result("upstream", StatusOK, fmt.Sprintf("Upstream resolver %s answered a health query.", d.Upstream), "")
}

func defaultUpstreamCheck(ctx context.Context, resolver string) error {
	if resolver == "" {
		return fmt.Errorf("resolver is not configured")
	}
	q := new(dns.Msg)
	q.SetQuestion(".", dns.TypeNS)
	client := &dns.Client{Timeout: 3 * time.Second}
	response, _, err := client.ExchangeContext(ctx, q, resolver)
	if err != nil {
		return err
	}
	if response == nil {
		return fmt.Errorf("empty DNS response")
	}
	if response.Rcode == dns.RcodeServerFailure {
		return fmt.Errorf("resolver returned SERVFAIL")
	}
	return nil
}

func blocklistCheck(d Deps) Check {
	if d.DnsInfo == nil {
		return unknown("blocklist", "Blocklist state is unavailable.", "Start the DNS service and refresh its blocklists.")
	}
	if d.DnsInfo.LastUpdated <= 0 || d.DnsInfo.NumberDomainsBlocked == 0 {
		return result("blocklist", StatusFailed, "No current blocked domains are loaded.", "Verify blocklist URLs and outbound access, then refresh the blocklists.")
	}
	age := d.now().Sub(time.Unix(int64(d.DnsInfo.LastUpdated), 0))
	if age > blocklistMaxAge {
		return result("blocklist", StatusFailed, fmt.Sprintf("The blocklist was last refreshed %s ago and contains %d domains.", age.Round(time.Minute), d.DnsInfo.NumberDomainsBlocked), "Refresh the blocklists and check the configured source URLs.")
	}
	return result("blocklist", StatusOK, fmt.Sprintf("%d blocked domains loaded; refreshed %s ago.", d.DnsInfo.NumberDomainsBlocked, age.Round(time.Minute)), "")
}

func certificateCheck(d Deps) Check {
	if d.Settings == nil {
		return unknown("certificate", "Certificate storage is unavailable.", "Make sure the settings store is mounted and readable.")
	}
	raw, err := d.Settings.GetE("capem")
	if err != nil || raw == "" {
		return result("certificate", StatusFailed, "The gateway certificate is unavailable.", "Restore the certificate or regenerate it from the certificate settings.")
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return result("certificate", StatusFailed, "The gateway certificate is not valid PEM.", "Replace the certificate with a valid PEM encoded certificate.")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return result("certificate", StatusFailed, "The gateway certificate could not be parsed.", "Replace the certificate with a valid X.509 certificate.")
	}
	if !d.now().Before(cert.NotAfter) {
		return result("certificate", StatusFailed, fmt.Sprintf("The gateway certificate expired on %s.", cert.NotAfter.UTC().Format(time.RFC3339)), "Renew the gateway certificate and install the replacement.")
	}
	return result("certificate", StatusOK, fmt.Sprintf("The gateway certificate expires on %s.", cert.NotAfter.UTC().Format(time.RFC3339)), "")
}

func storageCheck(d Deps) Check {
	if d.Settings == nil {
		return unknown("storage", "Settings storage is unavailable.", "Check the data directory and its permissions.")
	}
	const key = "__gatesentry_diagnostics_probe"
	values, err := d.Settings.Snapshot()
	if err != nil {
		return result("storage", StatusFailed, fmt.Sprintf("Storage could not be read: %v", err), "Check the data directory, filesystem capacity, and permissions.")
	}
	old, existed := values[key]
	if err := d.Settings.Update(key, d.now().Format(time.RFC3339Nano)); err != nil {
		return result("storage", StatusFailed, fmt.Sprintf("Storage could not be written: %v", err), "Check the data directory, filesystem capacity, and permissions.")
	}
	if err := d.Settings.UpdateMap(func(values map[string]string) error {
		if existed {
			values[key] = old
		} else {
			delete(values, key)
		}
		return nil
	}); err != nil {
		return result("storage", StatusFailed, fmt.Sprintf("Storage write succeeded but cleanup failed: %v", err), "Check filesystem health and remove the temporary diagnostics probe after recovery.")
	}
	return result("storage", StatusOK, "Settings storage accepted a temporary write and cleanup.", "")
}

func discoveryCheck(d Deps) Check {
	devices := d.Devices
	if d.GetDevices != nil {
		devices = d.GetDevices()
	}
	if devices == nil {
		return unknown("discovery", "Device discovery state is unavailable.", "Start DNS discovery and inspect its configuration.")
	}
	return result("discovery", StatusOK, fmt.Sprintf("Discovery is available with %d observed devices.", devices.DeviceCount()), "")
}
func policyCheck(d Deps) Check {
	policy := d.Policy
	if d.GetPolicy != nil {
		policy = d.GetPolicy()
	}
	if policy == nil {
		return unknown("policy", "Policy state is unavailable; coverage cannot be confirmed.", "Start the DNS policy service and reload policy storage.")
	}
	s := policy.Snapshot()
	return result("policy", StatusOK, fmt.Sprintf("Policy revision %d is active; %d groups and %d assignments are loaded.", s.Version, len(s.Groups), len(s.Assignments)), "")
}

// RedactSettings returns only explicitly safe operational settings. It never
// serializes credentials, keys, certificates, logs, URLs, or arbitrary values.
func RedactSettings(values map[string]string) map[string]string {
	allowed := []string{"dns_resolver", "enable_dns_server", "enable_https_filtering", "timezone", "settings_schema_version"}
	out := make(map[string]string)
	for _, key := range allowed {
		if value, ok := values[key]; ok {
			out[key] = value
		}
	}
	return out
}

func FilterSummary(filters *[]gatesentryFilters.GSFilter) []map[string]interface{} {
	out := []map[string]interface{}{}
	if filters == nil {
		return out
	}
	for _, f := range *filters {
		out = append(out, map[string]interface{}{"id": f.Id, "name": f.FilterName, "handles": f.Handles, "entries": len(f.FileContents)})
	}
	return out
}
