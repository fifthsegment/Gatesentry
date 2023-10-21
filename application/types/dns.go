package GatesentryTypes

// struct for custom dns entries
type DNSCustomEntry struct {
	IP     string `json:"ip"`
	Domain string `json:"domain"`
}

type DnsServerInfo struct {
	NumberDomainsBlocked int `json:"number_domains_blocked"`
	LastUpdated          int `json:"last_updated"`
	NextUpdate           int `json:"next_update"`
}
