// Package metrics provides Prometheus instrumentation for GateSentry.
//
// It exposes a custom collector that gathers runtime metrics from the DNS
// server, proxy server, DNS cache, SSE subscribers, device store, rule
// manager, and domain list index on each Prometheus scrape.
//
// Design: no background goroutine — all values are read on-demand from atomic
// counters (hot path) and lightweight accessors (cold path) so there is zero
// impact on DNS/proxy request processing.
package metrics

import (
	"log"

	dnsserver "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	gatesentryDomainList "bitbucket.org/abdullah_irfan/gatesentryf/domainlist"
	gatesentry2logger "bitbucket.org/abdullah_irfan/gatesentryf/logger"
	gatesentryWebserverEndpoints "bitbucket.org/abdullah_irfan/gatesentryf/webserver/endpoints"

	gatesentryproxy "bitbucket.org/abdullah_irfan/gatesentryproxy"

	"github.com/prometheus/client_golang/prometheus"
)

// Sources holds the runtime objects from which metrics are gathered.
// Any field may be nil — the collector gracefully skips nil sources.
type Sources struct {
	Logger            *gatesentry2logger.Log
	DomainListManager *gatesentryDomainList.DomainListManager
	RuleManager       gatesentryWebserverEndpoints.RuleManagerInterface
}

// gatesentryCollector implements prometheus.Collector.
type gatesentryCollector struct {
	sources Sources

	// ── DNS query counters ────────────────────────────────────────────
	dnsQueriesTotal *prometheus.Desc
	// ── DNS query latency histograms ──────────────────────────────────
	dnsQueryDuration    *prometheus.Desc
	dnsUpstreamDuration *prometheus.Desc

	// ── DNS cache counters (from atomic Stats) ────────────────────────
	cacheHits      *prometheus.Desc
	cacheMisses    *prometheus.Desc
	cacheInserts   *prometheus.Desc
	cacheEvictions *prometheus.Desc
	cacheExpired   *prometheus.Desc
	// ── DNS cache gauges ──────────────────────────────────────────────
	cacheEntries    *prometheus.Desc
	cacheMaxEntries *prometheus.Desc
	cacheSizeBytes  *prometheus.Desc
	cacheHitRate    *prometheus.Desc

	// ── Proxy request counters ────────────────────────────────────────
	proxyRequestsTotal *prometheus.Desc
	proxyConnectTotal  *prometheus.Desc
	proxyBlocksTotal   *prometheus.Desc
	proxyErrorsTotal   *prometheus.Desc
	proxyAuthFailures  *prometheus.Desc
	proxyPipelineTotal *prometheus.Desc
	// ── Proxy gauges ──────────────────────────────────────────────────
	proxyActiveRequests  *prometheus.Desc
	proxyActiveMITM      *prometheus.Desc
	proxyActiveDirect    *prometheus.Desc
	proxyActiveWebSocket *prometheus.Desc
	proxyBytesWritten    *prometheus.Desc
	proxyCertCacheTotal  *prometheus.Desc
	proxyCertCacheSize   *prometheus.Desc
	proxyUserCacheSize   *prometheus.Desc
	// ── Proxy latency histograms ──────────────────────────────────────
	proxyRequestDuration  *prometheus.Desc
	proxyUpstreamDuration *prometheus.Desc

	// ── SSE subscriber gauges ─────────────────────────────────────────
	sseSubscribers *prometheus.Desc

	// ── Application gauges ────────────────────────────────────────────
	deviceCount       *prometheus.Desc
	ruleCount         *prometheus.Desc
	ruleCountEnabled  *prometheus.Desc
	domainListDomains *prometheus.Desc
}

// NewCollector creates and returns a Prometheus collector for GateSentry metrics.
func NewCollector(src Sources) prometheus.Collector {
	ns := "gatesentry"

	return &gatesentryCollector{
		sources: src,

		// ── DNS queries ──────────────────────────────────────────────
		dnsQueriesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns", "queries_total"),
			"Total DNS queries by result type.",
			[]string{"result"}, nil,
		),
		dnsQueryDuration: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns", "query_duration_seconds"),
			"DNS query processing time distribution.",
			nil, nil,
		),
		dnsUpstreamDuration: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns", "upstream_duration_seconds"),
			"DNS upstream resolver round-trip time distribution.",
			nil, nil,
		),

		// ── DNS cache counters ───────────────────────────────────────
		cacheHits: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "hits_total"),
			"Total DNS cache hits.",
			nil, nil,
		),
		cacheMisses: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "misses_total"),
			"Total DNS cache misses.",
			nil, nil,
		),
		cacheInserts: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "inserts_total"),
			"Total DNS cache inserts.",
			nil, nil,
		),
		cacheEvictions: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "evictions_total"),
			"Total DNS cache evictions due to capacity pressure.",
			nil, nil,
		),
		cacheExpired: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "expired_total"),
			"Total DNS cache entries removed by TTL expiry.",
			nil, nil,
		),

		// ── DNS cache gauges ─────────────────────────────────────────
		cacheEntries: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "entries"),
			"Current number of entries in the DNS cache.",
			nil, nil,
		),
		cacheMaxEntries: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "max_entries"),
			"Maximum capacity of the DNS cache.",
			nil, nil,
		),
		cacheSizeBytes: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "size_bytes"),
			"Estimated memory used by the DNS cache in bytes.",
			nil, nil,
		),
		cacheHitRate: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "dns_cache", "hit_rate_percent"),
			"DNS cache hit rate as a percentage (0-100).",
			nil, nil,
		),

		// ── Proxy requests ───────────────────────────────────────────
		proxyRequestsTotal: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "requests_total"),
			"Total proxy requests by type.",
			[]string{"type"}, nil,
		),
		proxyConnectTotal: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "connect_total"),
			"Total HTTPS CONNECT requests by outcome.",
			[]string{"type"}, nil,
		),
		proxyBlocksTotal: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "blocks_total"),
			"Total blocked requests by reason.",
			[]string{"reason"}, nil,
		),
		proxyErrorsTotal: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "errors_total"),
			"Total proxy errors by type.",
			[]string{"type"}, nil,
		),
		proxyAuthFailures: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "auth_failures_total"),
			"Total proxy authentication failures.",
			nil, nil,
		),
		proxyPipelineTotal: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "pipeline_total"),
			"Total responses routed through each pipeline path.",
			[]string{"path"}, nil,
		),

		// ── Proxy gauges ─────────────────────────────────────────────
		proxyActiveRequests: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "active_requests"),
			"Currently executing proxy requests.",
			nil, nil,
		),
		proxyActiveMITM: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "active_mitm_connections"),
			"Currently active MITM (SSL bump) connections.",
			nil, nil,
		),
		proxyActiveDirect: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "active_direct_connections"),
			"Currently active CONNECT direct (passthrough) tunnels.",
			nil, nil,
		),
		proxyActiveWebSocket: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "active_websocket_connections"),
			"Currently active WebSocket tunnels.",
			nil, nil,
		),
		proxyBytesWritten: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "bytes_written_total"),
			"Total response bytes written to clients.",
			nil, nil,
		),
		proxyCertCacheTotal: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "cert_cache_total"),
			"Total TLS certificate cache operations.",
			[]string{"result"}, nil,
		),
		proxyCertCacheSize: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "cert_cache_entries"),
			"Current number of entries in the TLS certificate cache.",
			nil, nil,
		),
		proxyUserCacheSize: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "user_cache_entries"),
			"Current number of entries in the proxy auth user cache.",
			nil, nil,
		),
		proxyRequestDuration: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "request_duration_seconds"),
			"End-to-end proxy request processing time distribution.",
			nil, nil,
		),
		proxyUpstreamDuration: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "proxy", "upstream_duration_seconds"),
			"Proxy upstream RoundTrip time distribution.",
			nil, nil,
		),

		// ── SSE subscribers ──────────────────────────────────────────
		sseSubscribers: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "sse", "subscribers"),
			"Number of active SSE subscribers by stream type.",
			[]string{"stream"}, nil,
		),

		// ── Application gauges ───────────────────────────────────────
		deviceCount: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "", "devices"),
			"Number of discovered network devices.",
			nil, nil,
		),
		ruleCount: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "", "rules"),
			"Number of configured proxy rules.",
			nil, nil,
		),
		ruleCountEnabled: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "", "rules_enabled"),
			"Number of enabled proxy rules.",
			nil, nil,
		),
		domainListDomains: prometheus.NewDesc(
			prometheus.BuildFQName(ns, "domain_index", "domains_total"),
			"Total unique domains across all loaded domain lists.",
			nil, nil,
		),
	}
}

// Describe sends the super-set of all possible descriptors.
func (c *gatesentryCollector) Describe(ch chan<- *prometheus.Desc) {
	// DNS
	ch <- c.dnsQueriesTotal
	ch <- c.dnsQueryDuration
	ch <- c.dnsUpstreamDuration
	ch <- c.cacheHits
	ch <- c.cacheMisses
	ch <- c.cacheInserts
	ch <- c.cacheEvictions
	ch <- c.cacheExpired
	ch <- c.cacheEntries
	ch <- c.cacheMaxEntries
	ch <- c.cacheSizeBytes
	ch <- c.cacheHitRate
	// Proxy
	ch <- c.proxyRequestsTotal
	ch <- c.proxyConnectTotal
	ch <- c.proxyBlocksTotal
	ch <- c.proxyErrorsTotal
	ch <- c.proxyAuthFailures
	ch <- c.proxyPipelineTotal
	ch <- c.proxyActiveRequests
	ch <- c.proxyActiveMITM
	ch <- c.proxyActiveDirect
	ch <- c.proxyActiveWebSocket
	ch <- c.proxyBytesWritten
	ch <- c.proxyCertCacheTotal
	ch <- c.proxyCertCacheSize
	ch <- c.proxyUserCacheSize
	ch <- c.proxyRequestDuration
	ch <- c.proxyUpstreamDuration
	// SSE + application
	ch <- c.sseSubscribers
	ch <- c.deviceCount
	ch <- c.ruleCount
	ch <- c.ruleCountEnabled
	ch <- c.domainListDomains
}

// Collect is called on each Prometheus scrape.
func (c *gatesentryCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectDNSQueries(ch)
	c.collectDNSCache(ch)
	c.collectProxy(ch)
	c.collectSSE(ch)
	c.collectDevices(ch)
	c.collectRules(ch)
	c.collectDomainIndex(ch)
}

// ---------------------------------------------------------------------------
// DNS metrics
// ---------------------------------------------------------------------------

func (c *gatesentryCollector) collectDNSQueries(ch chan<- prometheus.Metric) {
	m := dnsserver.GetDNSMetrics()
	if m == nil {
		return
	}

	// Labeled counters by result type
	pairs := []struct {
		label string
		val   int64
	}{
		{"blocked", m.QueriesBlocked.Load()},
		{"cached", m.QueriesCached.Load()},
		{"forwarded", m.QueriesForwarded.Load()},
		{"device", m.QueriesDevice.Load()},
		{"exception", m.QueriesException.Load()},
		{"internal", m.QueriesInternal.Load()},
		{"error", m.QueriesError.Load()},
		{"wpad", m.QueriesWPAD.Load()},
		{"ddns", m.QueriesDDNS.Load()},
	}
	for _, p := range pairs {
		ch <- prometheus.MustNewConstMetric(c.dnsQueriesTotal, prometheus.CounterValue, float64(p.val), p.label)
	}

	// Query duration histogram
	emitDNSHistogram(ch, c.dnsQueryDuration, &m.QueryDuration)
	emitDNSHistogram(ch, c.dnsUpstreamDuration, &m.UpstreamDuration)
}

func emitDNSHistogram(ch chan<- prometheus.Metric, desc *prometheus.Desc, h *dnsserver.DNSHistogram) {
	buckets, count, sum := h.CumulativeBuckets()
	if count == 0 {
		return
	}
	ch <- prometheus.MustNewConstHistogram(desc, count, sum, buckets)
}

func (c *gatesentryCollector) collectDNSCache(ch chan<- prometheus.Metric) {
	cache := dnsserver.GetDNSCache()
	if cache == nil {
		return
	}

	snap := cache.Snapshot()

	ch <- prometheus.MustNewConstMetric(c.cacheHits, prometheus.CounterValue, float64(snap.Hits))
	ch <- prometheus.MustNewConstMetric(c.cacheMisses, prometheus.CounterValue, float64(snap.Misses))
	ch <- prometheus.MustNewConstMetric(c.cacheInserts, prometheus.CounterValue, float64(snap.Inserts))
	ch <- prometheus.MustNewConstMetric(c.cacheEvictions, prometheus.CounterValue, float64(snap.Evictions))
	ch <- prometheus.MustNewConstMetric(c.cacheExpired, prometheus.CounterValue, float64(snap.Expired))

	ch <- prometheus.MustNewConstMetric(c.cacheEntries, prometheus.GaugeValue, float64(snap.Entries))
	ch <- prometheus.MustNewConstMetric(c.cacheMaxEntries, prometheus.GaugeValue, float64(snap.MaxEntries))
	ch <- prometheus.MustNewConstMetric(c.cacheSizeBytes, prometheus.GaugeValue, float64(snap.SizeBytes))
	ch <- prometheus.MustNewConstMetric(c.cacheHitRate, prometheus.GaugeValue, snap.HitRate)
}

// ---------------------------------------------------------------------------
// Proxy metrics
// ---------------------------------------------------------------------------

func (c *gatesentryCollector) collectProxy(ch chan<- prometheus.Metric) {
	pm := gatesentryproxy.Metrics
	if pm == nil {
		return
	}

	// Request type counters
	ch <- prometheus.MustNewConstMetric(c.proxyRequestsTotal, prometheus.CounterValue, float64(pm.RequestsTotal.Load()), "all")
	ch <- prometheus.MustNewConstMetric(c.proxyRequestsTotal, prometheus.CounterValue, float64(pm.HTTPTotal.Load()), "http")
	ch <- prometheus.MustNewConstMetric(c.proxyRequestsTotal, prometheus.CounterValue, float64(pm.ConnectTotal.Load()), "connect")
	ch <- prometheus.MustNewConstMetric(c.proxyRequestsTotal, prometheus.CounterValue, float64(pm.WebSocketTotal.Load()), "websocket")

	// CONNECT breakdown
	ch <- prometheus.MustNewConstMetric(c.proxyConnectTotal, prometheus.CounterValue, float64(pm.MITMTotal.Load()), "mitm")
	ch <- prometheus.MustNewConstMetric(c.proxyConnectTotal, prometheus.CounterValue, float64(pm.DirectTotal.Load()), "direct")

	// Block reasons
	blockPairs := []struct {
		label string
		val   int64
	}{
		{"rule", pm.BlocksRule.Load()},
		{"url", pm.BlocksURL.Load()},
		{"time", pm.BlocksTime.Load()},
		{"user", pm.BlocksUser.Load()},
		{"ssrf", pm.BlocksSSRF.Load()},
		{"content_type", pm.BlocksContentType.Load()},
		{"keyword", pm.BlocksKeyword.Load()},
		{"media", pm.BlocksMedia.Load()},
	}
	for _, p := range blockPairs {
		ch <- prometheus.MustNewConstMetric(c.proxyBlocksTotal, prometheus.CounterValue, float64(p.val), p.label)
	}

	// Error types
	errorPairs := []struct {
		label string
		val   int64
	}{
		{"upstream", pm.ErrorsUpstream.Load()},
		{"hijack", pm.ErrorsHijack.Load()},
		{"tls", pm.ErrorsTLS.Load()},
		{"panic", pm.ErrorsPanic.Load()},
	}
	for _, p := range errorPairs {
		ch <- prometheus.MustNewConstMetric(c.proxyErrorsTotal, prometheus.CounterValue, float64(p.val), p.label)
	}

	// Auth failures
	ch <- prometheus.MustNewConstMetric(c.proxyAuthFailures, prometheus.CounterValue, float64(pm.AuthFailures.Load()))

	// Pipeline path counters
	ch <- prometheus.MustNewConstMetric(c.proxyPipelineTotal, prometheus.CounterValue, float64(pm.PipelineStream.Load()), "stream")
	ch <- prometheus.MustNewConstMetric(c.proxyPipelineTotal, prometheus.CounterValue, float64(pm.PipelinePeek.Load()), "peek")
	ch <- prometheus.MustNewConstMetric(c.proxyPipelineTotal, prometheus.CounterValue, float64(pm.PipelineBuffer.Load()), "buffer")

	// Active connection gauges
	ch <- prometheus.MustNewConstMetric(c.proxyActiveRequests, prometheus.GaugeValue, float64(pm.ActiveRequests.Load()))
	ch <- prometheus.MustNewConstMetric(c.proxyActiveMITM, prometheus.GaugeValue, float64(pm.ActiveMITM.Load()))
	ch <- prometheus.MustNewConstMetric(c.proxyActiveDirect, prometheus.GaugeValue, float64(pm.ActiveDirect.Load()))
	ch <- prometheus.MustNewConstMetric(c.proxyActiveWebSocket, prometheus.GaugeValue, float64(pm.ActiveWebSocket.Load()))

	// Bytes written
	ch <- prometheus.MustNewConstMetric(c.proxyBytesWritten, prometheus.CounterValue, float64(pm.BytesWritten.Load()))

	// Cert cache
	ch <- prometheus.MustNewConstMetric(c.proxyCertCacheTotal, prometheus.CounterValue, float64(pm.CertCacheHits.Load()), "hit")
	ch <- prometheus.MustNewConstMetric(c.proxyCertCacheTotal, prometheus.CounterValue, float64(pm.CertCacheMisses.Load()), "miss")
	ch <- prometheus.MustNewConstMetric(c.proxyCertCacheSize, prometheus.GaugeValue, float64(gatesentryproxy.CertCacheSize()))
	ch <- prometheus.MustNewConstMetric(c.proxyUserCacheSize, prometheus.GaugeValue, float64(gatesentryproxy.UserCacheSize()))

	// Latency histograms
	emitProxyHistogram(ch, c.proxyRequestDuration, &pm.RequestDuration)
	emitProxyHistogram(ch, c.proxyUpstreamDuration, &pm.UpstreamDuration)
}

func emitProxyHistogram(ch chan<- prometheus.Metric, desc *prometheus.Desc, h *gatesentryproxy.AtomicHistogram) {
	buckets, count, sum := h.CumulativeBuckets()
	if count == 0 {
		return
	}
	ch <- prometheus.MustNewConstHistogram(desc, count, sum, buckets)
}

// ---------------------------------------------------------------------------
// SSE, devices, rules, domain index (cold path — lightweight accessors)
// ---------------------------------------------------------------------------

func (c *gatesentryCollector) collectSSE(ch chan<- prometheus.Metric) {
	if c.sources.Logger != nil {
		ch <- prometheus.MustNewConstMetric(c.sseSubscribers, prometheus.GaugeValue,
			float64(c.sources.Logger.SubscriberCount()), "log_stream")
	}
	cache := dnsserver.GetDNSCache()
	if cache != nil && cache.Events != nil {
		ch <- prometheus.MustNewConstMetric(c.sseSubscribers, prometheus.GaugeValue,
			float64(cache.Events.SubscriberCount()), "dns_events")
	}
}

func (c *gatesentryCollector) collectDevices(ch chan<- prometheus.Metric) {
	store := dnsserver.GetDeviceStore()
	if store == nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(c.deviceCount, prometheus.GaugeValue, float64(store.DeviceCount()))
}

func (c *gatesentryCollector) collectRules(ch chan<- prometheus.Metric) {
	if c.sources.RuleManager == nil {
		return
	}
	rules, err := c.sources.RuleManager.GetRules()
	if err != nil {
		log.Printf("[metrics] Error counting rules: %v", err)
		return
	}

	total := len(rules)
	enabled := 0
	for _, r := range rules {
		if r.Enabled {
			enabled++
		}
	}
	ch <- prometheus.MustNewConstMetric(c.ruleCount, prometheus.GaugeValue, float64(total))
	ch <- prometheus.MustNewConstMetric(c.ruleCountEnabled, prometheus.GaugeValue, float64(enabled))
}

func (c *gatesentryCollector) collectDomainIndex(ch chan<- prometheus.Metric) {
	if c.sources.DomainListManager == nil || c.sources.DomainListManager.Index == nil {
		return
	}
	ch <- prometheus.MustNewConstMetric(c.domainListDomains, prometheus.GaugeValue,
		float64(c.sources.DomainListManager.Index.TotalDomains()))
}
