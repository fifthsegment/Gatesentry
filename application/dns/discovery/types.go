package discovery

import (
	"net"
	"time"

	"github.com/miekg/dns"
)

// DiscoverySource indicates how a device was discovered.
type DiscoverySource string

const (
	SourceDDNS      DiscoverySource = "ddns"      // RFC 2136 Dynamic DNS UPDATE
	SourceLease     DiscoverySource = "lease"     // DHCP lease file reader
	SourceMDNS      DiscoverySource = "mdns"      // mDNS/Bonjour browser
	SourcePassive   DiscoverySource = "passive"   // Passive DNS query observation
	SourceManual    DiscoverySource = "manual"    // User-entered via UI
	SourceTailscale DiscoverySource = "tailscale" // Linked tailscaled peer
)

// TailscaleIdentity links one stable tailscaled node to a canonical device.
// NodeID, names, and last-known Tailscale addresses are durable link metadata;
// reachability is runtime-only. WoLMACs are informational and never LAN identity.
type TailscaleIdentity struct {
	NodeID    string    `json:"node_id"`
	Name      string    `json:"name,omitempty"`
	DNSName   string    `json:"dns_name,omitempty"`
	Addresses []string  `json:"addresses,omitempty"`
	Online    bool      `json:"online"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
	WoLMACs   []string  `json:"wol_macs,omitempty"`
}

// Device represents a physical device on the local network.
type Device struct {
	// ID is a stable unique identifier (UUID v4).
	ID string `json:"id"`

	// DisplayName is the user-visible name.
	// If ManualName is set, it takes precedence.
	// Otherwise, derived from the best available hostname.
	DisplayName string `json:"display_name"`

	// DNSName is the sanitized hostname used in DNS records.
	// Lowercase, alphanumeric + hyphens only (RFC 952/1123).
	// Example: "viviennes-ipad"
	DNSName string `json:"dns_name"`

	// --- Identity: how we recognize this device across IP changes ---

	// Hostnames observed via DHCP Option 12.
	// Most recent first. The first entry is the "primary" hostname.
	Hostnames []string `json:"hostnames,omitempty"`

	// MDNSNames observed via Bonjour/mDNS service discovery.
	MDNSNames []string `json:"mdns_names,omitempty"`

	// MACs observed for this device. May change with MAC randomization.
	// Stored as lowercase colon-separated (e.g., "aa:bb:cc:dd:ee:ff").
	MACs []string `json:"macs,omitempty"`

	// TailscaleNodes are explicitly linked overlay identities. Tailscale does
	// not carry LAN MAC addresses, so these remain a separate namespace.
	TailscaleNodes []TailscaleIdentity `json:"tailscale_nodes,omitempty"`

	// --- Current network addresses ---

	// IPv4 is the current IPv4 address (empty string if unknown).
	IPv4 string `json:"ipv4,omitempty"`

	// IPv6 is the current IPv6 address — GUA or ULA preferred over link-local.
	IPv6 string `json:"ipv6,omitempty"`

	// --- Discovery metadata ---

	// Source indicates the primary discovery method.
	Source DiscoverySource `json:"source"`

	// Sources tracks all methods that have contributed information.
	Sources []DiscoverySource `json:"sources,omitempty"`

	// FirstSeen is when the device was first observed.
	FirstSeen time.Time `json:"first_seen"`

	// LastSeen is when the device was last observed by any method.
	LastSeen time.Time `json:"last_seen"`

	// LANLastSeen tracks direct LAN/passive ownership separately from overlay
	// activity. It is runtime-only: persisted assignments contain no current
	// address claims, and Tailscale activity must not keep a stale LAN claim
	// fresh for policy attribution.
	LANLastSeen time.Time `json:"-"`

	// Online indicates whether the device has been seen within the
	// configured online threshold (default: 5 minutes).
	Online bool `json:"online"`

	// --- User-managed fields ---

	// ManualName is a user-assigned friendly name that overrides auto-derived names.
	ManualName string `json:"manual_name,omitempty"`

	// Owner identifies who the device belongs to (e.g., "Vivienne", "Dad").
	// NOTE: This maps to Rule.Users in the existing rule engine. When per-device
	// filtering is implemented, the DNS handler will use FindDeviceByIP() to
	// resolve query source → device → owner → filtering policy.
	Owner string `json:"owner,omitempty"`

	// Category assigns this device to a filtering group (e.g., "kids", "adults", "iot").
	// NOTE: Intentionally a single string for now. May evolve to Groups []string
	// in a future branch when per-group parental control policies are implemented.
	// The device store itself has no filtering logic — policy decisions belong in
	// the DNS handler or a dedicated policy engine.
	Category string `json:"category,omitempty"`

	// Persistent indicates this device entry should survive restarts
	// even without re-discovery. True for manual entries.
	Persistent bool `json:"persistent"`
}

// GetDisplayName returns the most stable available label for this device.
// A user-assigned name always wins. Automatic labels prefer MAC identity,
// followed by a LAN hostname, another observed name, and finally the current
// LAN address.
func (d *Device) GetDisplayName() string {
	if d.ManualName != "" {
		return d.ManualName
	}
	if len(d.MACs) > 0 {
		return d.MACs[0]
	}
	if len(d.Hostnames) > 0 {
		return d.Hostnames[0]
	}
	if d.DNSName != "" {
		return d.DNSName
	}
	for _, identity := range d.TailscaleNodes {
		if identity.Name != "" {
			return identity.Name
		}
		if identity.DNSName != "" {
			return identity.DNSName
		}
	}
	if len(d.MDNSNames) > 0 {
		return d.MDNSNames[0]
	}
	if d.IPv4 != "" {
		return d.IPv4
	}
	if d.IPv6 != "" {
		return d.IPv6
	}
	return "Unknown"
}

// HasIP reports whether ip is one of the device's current LAN or linked
// Tailscale addresses.
func (d *Device) HasIP(ip string) bool {
	if ip == "" {
		return false
	}
	if d.IPv4 == ip || d.IPv6 == ip {
		return true
	}
	return d.HasTailscaleIP(ip)
}

// HasTailscaleIP reports whether ip is a last-known alias of an explicitly
// linked stable node. These aliases remain useful identity after reachability
// observations expire; address conflicts are handled by the store claim index.
func (d *Device) HasTailscaleIP(ip string) bool {
	ip = normalizeIP(ip)
	if ip == "" {
		return false
	}
	for _, identity := range d.TailscaleNodes {
		for _, address := range identity.Addresses {
			if normalizeIP(address) == ip {
				return true
			}
		}
	}
	return false
}

// HasSource returns true if the device was discovered by the given source.
func (d *Device) HasSource(source DiscoverySource) bool {
	for _, s := range d.Sources {
		if s == source {
			return true
		}
	}
	return false
}

// AddSource adds a discovery source if not already present.
func (d *Device) AddSource(source DiscoverySource) {
	if !d.HasSource(source) {
		d.Sources = append(d.Sources, source)
	}
}

// DnsRecord represents a single DNS resource record derived from
// the device inventory. These are generated, not manually managed.
type DnsRecord struct {
	// Name is the fully-qualified domain name (without trailing dot).
	// Example: "macmini.local"
	Name string `json:"name"`

	// Type is the DNS record type (dns.TypeA, dns.TypeAAAA, dns.TypePTR).
	Type uint16 `json:"type"`

	// Value is the record data.
	// For A/AAAA: the IP address string.
	// For PTR: the target hostname.
	Value string `json:"value"`

	// TTL in seconds. Default 60 for dynamic records, 300 for manual.
	TTL uint32 `json:"ttl"`

	// DeviceID links this record back to its source Device.
	DeviceID string `json:"device_id"`

	// Source indicates how the record was created.
	Source DiscoverySource `json:"source"`
}

// ToRR converts a DnsRecord to a miekg/dns resource record suitable
// for including in a DNS response message.
func (r *DnsRecord) ToRR() dns.RR {
	fqdn := dns.Fqdn(r.Name)
	switch r.Type {
	case dns.TypeA:
		return &dns.A{
			Hdr: dns.RR_Header{
				Name:   fqdn,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    r.TTL,
			},
			A: net.ParseIP(r.Value),
		}
	case dns.TypeAAAA:
		return &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   fqdn,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    r.TTL,
			},
			AAAA: net.ParseIP(r.Value),
		}
	case dns.TypePTR:
		return &dns.PTR{
			Hdr: dns.RR_Header{
				Name:   fqdn,
				Rrtype: dns.TypePTR,
				Class:  dns.ClassINET,
				Ttl:    r.TTL,
			},
			Ptr: dns.Fqdn(r.Value),
		}
	default:
		return nil
	}
}

// TypeString returns a human-readable record type name.
func (r *DnsRecord) TypeString() string {
	switch r.Type {
	case dns.TypeA:
		return "A"
	case dns.TypeAAAA:
		return "AAAA"
	case dns.TypePTR:
		return "PTR"
	default:
		return dns.TypeToString[r.Type]
	}
}

// DefaultTTL for auto-discovered records.
const DefaultTTL uint32 = 60

// ManualTTL for manually-entered records.
const ManualTTL uint32 = 300

// OnlineThreshold is how recently a device must have been seen
// to be considered "online".
const OnlineThreshold = 5 * time.Minute
