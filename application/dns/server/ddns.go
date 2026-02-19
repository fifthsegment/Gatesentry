package gatesentryDnsServer

import (
	"log"
	"net"
	"strings"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	"github.com/miekg/dns"
)

// --- Package-level DDNS configuration ---
// These are set in StartDNSServer() from settings.

var (
	// ddnsEnabled controls whether DDNS UPDATE messages are accepted.
	// Default: true (DDNS works out of the box for DHCP servers on the same machine).
	ddnsEnabled = true

	// ddnsTSIGRequired controls whether TSIG authentication is mandatory.
	// Default: false (simple setups don't need TSIG; enable for security).
	ddnsTSIGRequired = false
)

// SetDDNSEnabled enables or disables acceptance of DDNS UPDATE messages at runtime.
func SetDDNSEnabled(enabled bool) {
	ddnsEnabled = enabled
	if enabled {
		log.Println("[DDNS] Dynamic DNS updates enabled")
	} else {
		log.Println("[DDNS] Dynamic DNS updates disabled")
	}
}

// SetDDNSTSIGRequired enables or disables mandatory TSIG authentication for DDNS.
func SetDDNSTSIGRequired(required bool) {
	ddnsTSIGRequired = required
	if required {
		log.Println("[DDNS] TSIG authentication required for updates")
	} else {
		log.Println("[DDNS] TSIG authentication not required (updates accepted without TSIG)")
	}
}

// UpdateTSIGKey updates the TSIG secret map on both DNS servers (UDP and TCP)
// at runtime. If keyName or keySecret is empty, TSIG verification is removed.
func UpdateTSIGKey(keyName, keySecret string) {
	if keyName == "" || keySecret == "" {
		// Remove TSIG configuration
		if server != nil {
			server.TsigSecret = nil
		}
		if tcpServer != nil {
			tcpServer.TsigSecret = nil
		}
		log.Println("[DDNS] TSIG key removed")
		return
	}
	if !strings.HasSuffix(keyName, ".") {
		keyName += "."
	}
	secrets := map[string]string{keyName: keySecret}
	if server != nil {
		server.TsigSecret = secrets
	}
	if tcpServer != nil {
		tcpServer.TsigSecret = secrets
	}
	log.Printf("[DDNS] TSIG key updated: %s", strings.TrimSuffix(keyName, "."))
}

// ddnsMsgAcceptFunc extends the default miekg/dns message acceptance to also
// accept DNS UPDATE (opcode 5) messages. The default MsgAcceptFunc rejects
// UPDATE because the Ns section can contain many RRs, but we need it for DDNS.
func ddnsMsgAcceptFunc(dh dns.Header) dns.MsgAcceptAction {
	opcode := int(dh.Bits>>11) & 0xF
	if opcode == dns.OpcodeUpdate {
		return dns.MsgAccept
	}
	return dns.DefaultMsgAcceptFunc(dh)
}

// handleDDNSUpdate processes an RFC 2136 Dynamic DNS UPDATE message.
// It validates TSIG if configured, checks the zone, parses the UPDATE
// section using RFC 2136 §2.5 semantics, and applies add/delete operations
// to the device store.
//
// RFC 2136 §2.5 update section semantics:
//   - Class IN  + TTL > 0         → Add RR to an RRset
//   - Class ANY + TTL = 0 + no RD → Delete all RRsets for a name
//   - Class NONE + TTL = 0        → Delete specific RR from an RRset
//
// Reference: Python DDNS implementation in DDNS/ project
func handleDDNSUpdate(w dns.ResponseWriter, r *dns.Msg) {
	// Extract client IP for logging
	clientIP := ""
	if addr := w.RemoteAddr(); addr != nil {
		clientIP = addr.String()
		if host, _, err := net.SplitHostPort(clientIP); err == nil {
			clientIP = host
		}
	}

	// Extract zone name for logging (best-effort, before validation)
	requestZone := ""
	if len(r.Question) > 0 {
		requestZone = strings.ToLower(strings.TrimSuffix(r.Question[0].Name, "."))
	}

	// 1. Check if DDNS is enabled
	if !ddnsEnabled {
		log.Println("[DDNS] UPDATE rejected: DDNS is disabled")
		if logger != nil {
			logger.LogDNS(requestZone, clientIP, "ddns-rejected")
		}
		sendDDNSResponse(w, r, dns.RcodeRefused)
		return
	}

	// 2. Validate TSIG authentication
	if ddnsTSIGRequired {
		tsig := r.IsTsig()
		if tsig == nil {
			log.Println("[DDNS] UPDATE rejected: TSIG required but not present")
			if logger != nil {
				logger.LogDNS(requestZone, clientIP, "ddns-rejected")
			}
			sendDDNSResponse(w, r, dns.RcodeRefused)
			return
		}
		if w.TsigStatus() != nil {
			log.Printf("[DDNS] UPDATE rejected: TSIG verification failed: %v", w.TsigStatus())
			if logger != nil {
				logger.LogDNS(requestZone, clientIP, "ddns-rejected")
			}
			sendDDNSResponse(w, r, dns.RcodeRefused)
			return
		}
	} else if tsig := r.IsTsig(); tsig != nil {
		// TSIG not required but present — still validate it
		if w.TsigStatus() != nil {
			log.Printf("[DDNS] UPDATE rejected: TSIG present but invalid: %v", w.TsigStatus())
			if logger != nil {
				logger.LogDNS(requestZone, clientIP, "ddns-rejected")
			}
			sendDDNSResponse(w, r, dns.RcodeRefused)
			return
		}
	}

	// 3. Validate zone section
	if len(r.Question) == 0 {
		log.Println("[DDNS] UPDATE rejected: empty zone section")
		if logger != nil {
			logger.LogDNS("", clientIP, "ddns-rejected")
		}
		sendDDNSResponse(w, r, dns.RcodeFormatError)
		return
	}
	updateZone := strings.ToLower(strings.TrimSuffix(r.Question[0].Name, "."))

	// 3a. Reverse zones (in-addr.arpa / ip6.arpa): accept and succeed immediately.
	// GateSentry auto-generates PTR records from forward A/AAAA records stored in
	// the device store (see rebuildIndexes), so we don't need to process reverse
	// zone UPDATEs. Returning success prevents DHCP servers (e.g., pfSense) from
	// logging errors and endlessly retrying.
	if strings.HasSuffix(updateZone, ".in-addr.arpa") || strings.HasSuffix(updateZone, ".ip6.arpa") {
		log.Printf("[DDNS] UPDATE accepted (reverse zone, no-op): zone=%s (from %s)",
			updateZone, clientIP)
		if logger != nil {
			logger.LogDNS(updateZone, clientIP, "ddns-ptr")
		}
		sendDDNSResponse(w, r, dns.RcodeSuccess)
		return
	}

	if !isAuthorizedZone(updateZone) {
		log.Printf("[DDNS] UPDATE rejected: zone %q not authorized", updateZone)
		if logger != nil {
			logger.LogDNS(updateZone, clientIP, "ddns-rejected")
		}
		sendDDNSResponse(w, r, dns.RcodeNotZone)
		return
	}

	// 4. Parse UPDATE section (msg.Ns — authority section repurposed for UPDATE)
	adds, deletes := parseDDNSUpdates(r.Ns, updateZone)

	// 5. Apply: deletions first, then additions (RFC 2136 §3.4.2)
	appliedDeletes := 0
	for _, del := range deletes {
		applyDDNSDelete(del)
		appliedDeletes++
		if logger != nil {
			logger.LogDNS(del.name, clientIP, "ddns-delete")
		}
	}

	appliedAdds := 0
	for _, add := range adds {
		applyDDNSAdd(add, updateZone)
		appliedAdds++
		if logger != nil {
			logger.LogDNS(add.name, clientIP, "ddns-add")
		}
	}

	// 6. Clean up non-persistent devices with no remaining addresses.
	// This handles the case where a DELETE removed all addresses and no
	// subsequent ADD replaced them. Devices that received new addresses
	// from ADDs are left intact.
	cleanupOrphanedDevices()

	log.Printf("[DDNS] UPDATE applied: zone=%s adds=%d deletes=%d (from %s)",
		updateZone, appliedAdds, appliedDeletes, w.RemoteAddr())

	sendDDNSResponse(w, r, dns.RcodeSuccess)
}

// ddnsUpdate represents a parsed RFC 2136 update operation.
type ddnsUpdate struct {
	name   string // FQDN without trailing dot, lowercase
	rrtype uint16 // dns.TypeA, dns.TypeAAAA, dns.TypeANY, etc.
	class  uint16 // dns.ClassINET=add, dns.ClassANY=delete-all, dns.ClassNONE=delete-specific
	ttl    uint32
	value  string // IP address for A/AAAA, hostname for PTR (empty for delete-all)
}

// parseDDNSUpdates extracts add and delete operations from the UPDATE section.
//
// RFC 2136 §2.5 class semantics:
//   - Class IN (1)     → Add to an RRset
//   - Class ANY (255)  → Delete an RRset (TTL=0, no RDATA) or all RRsets (TypeANY)
//   - Class NONE (254) → Delete a specific RR from an RRset
func parseDDNSUpdates(rrs []dns.RR, zone string) (adds []ddnsUpdate, deletes []ddnsUpdate) {
	for _, rr := range rrs {
		hdr := rr.Header()
		name := strings.ToLower(strings.TrimSuffix(hdr.Name, "."))

		update := ddnsUpdate{
			name:   name,
			rrtype: hdr.Rrtype,
			class:  hdr.Class,
			ttl:    hdr.Ttl,
		}

		// Extract value from typed RR (present for adds and specific deletes).
		// *dns.ANY has no RDATA — used for ClassANY delete operations.
		switch v := rr.(type) {
		case *dns.A:
			if v.A != nil {
				update.value = v.A.String()
			}
		case *dns.AAAA:
			if v.AAAA != nil {
				update.value = v.AAAA.String()
			}
		case *dns.PTR:
			update.value = strings.TrimSuffix(v.Ptr, ".")
		}

		switch hdr.Class {
		case dns.ClassINET:
			// Add operation — requires a value
			if update.value != "" {
				adds = append(adds, update)
			}
		case dns.ClassANY:
			// Delete all RRsets for name (TypeANY) or specific type
			deletes = append(deletes, update)
		case dns.ClassNONE:
			// Delete specific RR
			deletes = append(deletes, update)
		default:
			log.Printf("[DDNS] Ignoring RR with unexpected class %d: %s", hdr.Class, name)
		}
	}
	return
}

// applyDDNSAdd processes an ADD operation from a DDNS UPDATE.
// Creates or updates a device in the store with the given record.
func applyDDNSAdd(update ddnsUpdate, zone string) {
	if deviceStore == nil {
		return
	}

	hostname := extractHostname(update.name, zone)
	if hostname == "" {
		log.Printf("[DDNS] ADD ignored: cannot extract hostname from %q in zone %q",
			update.name, zone)
		return
	}

	device := &discovery.Device{
		Hostnames: []string{hostname},
		Source:    discovery.SourceDDNS,
		Sources:   []discovery.DiscoverySource{discovery.SourceDDNS},
	}

	switch update.rrtype {
	case dns.TypeA:
		device.IPv4 = update.value
	case dns.TypeAAAA:
		device.IPv6 = update.value
	default:
		log.Printf("[DDNS] ADD: unsupported RR type %s for %s",
			dns.TypeToString[update.rrtype], update.name)
		return
	}

	// Match existing device by hostname or IP to merge
	existing := deviceStore.FindDeviceByHostname(hostname)
	if existing == nil && device.IPv4 != "" {
		existing = deviceStore.FindDeviceByIP(device.IPv4)
	}
	if existing == nil && device.IPv6 != "" {
		existing = deviceStore.FindDeviceByIP(device.IPv6)
	}
	if existing != nil {
		device.ID = existing.ID
	}

	// ARP lookup for MAC enrichment
	if device.IPv4 != "" {
		if mac := discovery.LookupARPEntry(device.IPv4); mac != "" {
			device.MACs = []string{mac}
		}
	}

	deviceID := deviceStore.UpsertDevice(device)
	if existing == nil {
		log.Printf("[DDNS] New device: %s → %s (ID: %s)", update.name, update.value, deviceID)
	} else {
		log.Printf("[DDNS] Updated device: %s → %s (ID: %s)", update.name, update.value, deviceID)
	}
}

// applyDDNSDelete processes a DELETE operation from a DDNS UPDATE.
//
// Uses ClearDeviceAddress to directly remove IPs without going through
// UpsertDevice's merge logic (which would preserve empty IPs). The device
// itself is kept alive so that subsequent ADDs in the same UPDATE message
// can find it by hostname. Orphaned devices are cleaned up after all
// operations are applied.
func applyDDNSDelete(update ddnsUpdate) {
	if deviceStore == nil {
		return
	}

	// Look up existing records for this name
	var records []discovery.DnsRecord
	if update.rrtype == dns.TypeANY {
		records = deviceStore.LookupAll(update.name)
	} else {
		records = deviceStore.LookupName(update.name, update.rrtype)
	}

	if len(records) == 0 {
		// Name not found — silently succeed per RFC 2136
		return
	}

	switch update.class {
	case dns.ClassANY:
		// Delete all records for this name (or specific type)
		deviceID := records[0].DeviceID
		clearIPv4 := update.rrtype == dns.TypeANY || update.rrtype == dns.TypeA
		clearIPv6 := update.rrtype == dns.TypeANY || update.rrtype == dns.TypeAAAA
		deviceStore.ClearDeviceAddress(deviceID, clearIPv4, clearIPv6)
		log.Printf("[DDNS] Cleared records for: %s (ID: %s)", update.name, deviceID)

	case dns.ClassNONE:
		// Delete specific RR matching the value
		for _, rec := range records {
			if rec.Value == update.value {
				clearIPv4 := update.rrtype == dns.TypeA
				clearIPv6 := update.rrtype == dns.TypeAAAA
				deviceStore.ClearDeviceAddress(rec.DeviceID, clearIPv4, clearIPv6)
				log.Printf("[DDNS] Deleted %s %s for: %s (ID: %s)",
					dns.TypeToString[update.rrtype], update.value, update.name, rec.DeviceID)
			}
		}
	}
}

// cleanupOrphanedDevices removes non-persistent devices that have no remaining
// IP addresses. Called after all DDNS delete/add operations are applied.
func cleanupOrphanedDevices() {
	if deviceStore == nil {
		return
	}
	for _, d := range deviceStore.GetAllDevices() {
		if d.IPv4 == "" && d.IPv6 == "" && !d.Persistent {
			deviceStore.RemoveDevice(d.ID)
			log.Printf("[DDNS] Cleaned up addressless device: %s (ID: %s)", d.DisplayName, d.ID)
		}
	}
}

// isAuthorizedZone checks if the given zone matches any configured zone.
func isAuthorizedZone(zone string) bool {
	if deviceStore == nil {
		return false
	}
	for _, z := range deviceStore.Zones() {
		if strings.EqualFold(zone, z) {
			return true
		}
	}
	return false
}

// extractHostname strips the zone suffix from an FQDN to get the bare hostname.
//
// Examples:
//
//	extractHostname("macmini.local", "local") → "macmini"
//	extractHostname("printer.jvj28.com", "jvj28.com") → "printer"
//	extractHostname("sub.host.local", "local") → "sub.host"
//	extractHostname("local", "local") → "" (zone itself is not a hostname)
func extractHostname(fqdn string, zone string) string {
	fqdn = strings.ToLower(fqdn)
	zone = strings.ToLower(zone)
	suffix := "." + zone
	if strings.HasSuffix(fqdn, suffix) {
		host := fqdn[:len(fqdn)-len(suffix)]
		if host != "" {
			return host
		}
	}
	return ""
}

// sendDDNSResponse sends a DNS response for a DDNS UPDATE message.
// If the request had a valid TSIG, the response is signed with the same key.
func sendDDNSResponse(w dns.ResponseWriter, r *dns.Msg, rcode int) {
	m := new(dns.Msg)
	m.SetRcode(r, rcode)

	// Sign response with TSIG if the request had TSIG
	if tsig := r.IsTsig(); tsig != nil {
		m.SetTsig(tsig.Hdr.Name, tsig.Algorithm, 300, time.Now().Unix())
	}

	w.WriteMsg(m)
}
