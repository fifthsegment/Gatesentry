package gatesentryproxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/h2non/filetype"
)

var IProxy *GSProxy
var MaxContentScanSize int64 = 2e6 // Path C (HTML-only) scan buffer; tunable via GS_MAX_SCAN_SIZE_MB
var DebugLogging = false           // Disable verbose logging for performance

// AdminPort is the GateSentry admin UI port. Proxy requests targeting this port
// on any loopback/local address are blocked to prevent SSRF to admin endpoints.
var AdminPort = "8080"

// GateSentryDNSPort is the port GateSentry's DNS server listens on.
// Read from GATESENTRY_DNS_PORT env var at init; defaults to "10053".
var GateSentryDNSPort = "10053"

// Phase 3: Response pipeline path constants.
// The proxy classifies each response by Content-Type and routes it through
// one of three pipelines with different buffering strategies.
const (
	pipelineStream = iota // Path A: stream passthrough (JS, CSS, fonts, JSON, binary, downloads)
	pipelinePeek          // Path B: peek 4KB + stream (images, video, audio)
	pipelineBuffer        // Path C: buffer & scan (text/html, xhtml, unknown)
)

// PeekSize is the number of bytes read for filetype detection in Path B.
const PeekSize = 4096

// errSSRFBlocked is returned when the proxy blocks an outbound connection
// to a loopback or link-local address (SSRF protection).
var errSSRFBlocked = errors.New("connection to loopback or link-local address blocked (SSRF protection)")

var ip6Loopback = net.ParseIP("::1")

var dialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

func init() {
	// Read DNS port from environment (set by run.sh / docker-compose)
	if port := os.Getenv("GATESENTRY_DNS_PORT"); port != "" {
		GateSentryDNSPort = port
	}
	if port := os.Getenv("GS_ADMIN_PORT"); port != "" {
		AdminPort = port
	}
	if sizeMB := os.Getenv("GS_MAX_SCAN_SIZE_MB"); sizeMB != "" {
		if mb, err := strconv.ParseInt(sizeMB, 10, 64); err == nil && mb > 0 {
			MaxContentScanSize = mb * 1024 * 1024
			log.Printf("[Phase3] MaxContentScanSize set to %dMB via GS_MAX_SCAN_SIZE_MB", mb)
		}
	}

	// Wire the dialer's resolver to GateSentry's own DNS server so that
	// every hostname the proxy resolves goes through GateSentry filtering.
	setGateSentryResolver()
	log.Printf("[Phase2] Proxy DNS resolver wired to 127.0.0.1:%s", GateSentryDNSPort)
}

// setGateSentryResolver configures the dialer to resolve DNS through
// GateSentry's own DNS server at 127.0.0.1:GateSentryDNSPort.
func setGateSentryResolver() {
	dialer.Resolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "udp", "127.0.0.1:"+GateSentryDNSPort)
		},
	}
}

// SetDNSResolver switches the proxy's DNS resolution strategy.
// When the GateSentry DNS server is running, the proxy should use it so that
// blocked-domain filtering applies to proxied requests. When the DNS server is
// stopped, the proxy falls back to the configured upstream resolver so that
// proxied requests can still resolve hostnames.
func SetDNSResolver(useGateSentryDNS bool, upstreamResolver string) {
	if useGateSentryDNS {
		setGateSentryResolver()
		log.Printf("[Proxy] DNS resolver switched to GateSentry DNS (127.0.0.1:%s)", GateSentryDNSPort)
	} else {
		// Normalize upstream resolver — ensure it has a port
		if _, _, err := net.SplitHostPort(upstreamResolver); err != nil {
			upstreamResolver = net.JoinHostPort(upstreamResolver, "53")
		}
		resolver := upstreamResolver // capture for closure
		dialer.Resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", resolver)
			},
		}
		log.Printf("[Proxy] DNS resolver switched to upstream (%s) — GateSentry DNS is disabled", resolver)
	}
}

// safeDialContext prevents SSRF attacks targeting GateSentry's own admin UI.
// When a hostname resolves to a loopback or link-local address AND targets
// the admin port, the connection is blocked. All other connections are allowed
// because GateSentry's DNS is the resolver — if it resolved a domain, the
// proxy should trust that resolution.
//
// The DNS resolver is wired to GateSentry DNS (init), so all hostname
// resolution goes through GateSentry filtering before reaching here.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	// Block requests to the admin UI port on loopback/link-local addresses.
	// This catches DNS rebinding attacks where evil.com → 127.0.0.1:8080.
	if port == AdminPort {
		// If it's already an IP literal targeting admin port on loopback, block.
		if ip := net.ParseIP(host); ip != nil {
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				log.Printf("[SECURITY] SSRF blocked: connection to admin port %s on %s", port, host)
				return nil, errSSRFBlocked
			}
		} else {
			// It's a hostname — resolve and check.
			ips, err := dialer.Resolver.LookupHost(ctx, host)
			if err == nil {
				for _, ipStr := range ips {
					if ip := net.ParseIP(ipStr); ip != nil {
						if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
							log.Printf("[SECURITY] SSRF blocked: %q resolved to %s targeting admin port %s", host, ipStr, port)
							return nil, errSSRFBlocked
						}
					}
				}
			}
		}
	}

	return dialer.DialContext(ctx, network, addr)
}

var httpTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           safeDialContext,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DisableCompression:    true, // Phase 3: don't auto-decompress; proxy handles it per-path
}

func NewGSProxyPassthru() *GSProxyPassthru {
	p := GSProxyPassthru{}
	p.ProxyActionToLog = ProxyActionFilterNone
	return &p
}

func NewGSHandler(handlerid string, f func(*[]byte, *GSResponder, *GSProxyPassthru)) *GSHandler {
	// h := GSHandler{Id: handlerid, Handle: f}
	// h.Handle = f;
	// return &h
	return nil
}

func NewGSProxy() *GSProxy {
	proxy := GSProxy{}
	IProxy = &proxy
	// UsersCache is sync.Map — zero value is ready to use, no init needed

	// Start periodic eviction of expired user cache entries
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now().Unix()
			IProxy.UsersCache.Range(func(key, value interface{}) bool {
				if cached, ok := value.(GSUserCached); ok {
					if now-cached.CachedAt > 300 { // 5 minutes
						IProxy.UsersCache.Delete(key)
					}
				}
				return true
			})
		}
	}()

	return &proxy
}

func (p *GSProxy) RegisterHandler(id string, f func(*[]byte, *GSResponder, *GSProxyPassthru)) {
	h := NewGSHandler(id, f)
	if p.Handlers == nil {
		p.Handlers = map[string][]*GSHandler{}
	}
	log.Printf("Registering Handler for " + id)
	mm, ok := p.Handlers[id]
	if !ok {
		mm = ([]*GSHandler{})
		p.Handlers[id] = mm
	}
	p.Handlers[id] = append(p.Handlers[id], h)
}

func (p *GSProxy) RegisterAuthHandler(f func(authheader string) bool) {
	log.Println("Registering Auth Handler")
	p.AuthHandler = f
}

func (p *GSProxy) RunHandler(handlerid string, content *GSContentFilterData) {
	if p.Handlers[handlerid] != nil {
		for i := 0; i < len(p.Handlers[handlerid]); i++ {
			p.Handlers[handlerid][i].Handle(content)
		}
	}
}

func (p *GSProxy) RunAuthHandler(authheader string) bool {
	if p.AuthHandler != nil {
		return p.AuthHandler(authheader)
	}
	return false
}

func InitProxy() {
	CreateBlockedImageBytes()
	MaxContentScanSize = 1e7 // 10MB for low-spec hardware
}

type ProxyHandler struct {
	// TLS is whether this is an HTTPS connection.
	TLS bool

	// connectPort is the server port that was specified in a CONNECT request.
	connectPort string

	// user is a user that has already been authenticated.
	user string

	// rt is the RoundTripper that will be used to fulfill the requests.
	// If it is nil, a default Transport will be used.
	rt http.RoundTripper

	Iproxy *GSProxy
}

func decodeBase64Credentials(auth string) (user, pass string, ok bool) {
	auth = strings.TrimSpace(auth)
	enc := base64.StdEncoding

	// Use buffer pool for small allocations
	bufPtr := GetSmallBuffer()
	defer PutSmallBuffer(bufPtr)
	buf := *bufPtr

	n, err := enc.Decode(buf, []byte(auth))
	if err != nil {
		return "", "", false
	}
	auth = string(buf[:n])

	colon := strings.Index(auth, ":")
	if colon == -1 {
		return "", "", false
	}

	return auth[:colon], auth[colon+1:], true
}

type DataPassThru struct {
	io.Writer
	// total int64 // Total # of bytes transferred
	Contenttype string
	Passthru    *GSProxyPassthru
}

func (pt *DataPassThru) Write(p []byte) (int, error) {
	n, err := pt.Writer.Write(p)
	if err == nil {
		Metrics.BytesWritten.Add(int64(n))
		IProxy.ContentSizeHandler(
			GSContentSizeFilterData{
				Url:         "",
				ContentType: pt.Contenttype,
				ContentSize: int64(n),
				User:        pt.Passthru.User,
			},
		)
	}
	return n, err
}

func (h ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestStart := time.Now()
	Metrics.RequestsTotal.Add(1)
	Metrics.ActiveRequests.Add(1)
	defer func() {
		Metrics.ActiveRequests.Add(-1)
		Metrics.RequestDuration.Observe(time.Since(requestStart))
	}()

	passthru := NewGSProxyPassthru()

	client := r.RemoteAddr
	host, _, err := net.SplitHostPort(client)
	if err == nil {
		client = host
	}

	if TransparentProxyEnabled && IsTransparentProxyRequest(r) {
		if DebugLogging {
			log.Printf("[Transparent] Detected transparent proxy request from %s to %s", client, r.Host)
		}

		originalDst := r.Host
		if originalDst == "" {
			log.Printf("[Transparent] No Host header in transparent request from %s", client)
			http.Error(w, "No Host header", http.StatusBadRequest)
			return
		}

		if !strings.Contains(originalDst, ":") {
			originalDst = net.JoinHostPort(originalDst, "80")
		}

		r.URL.Scheme = "http"
		r.URL.Host = originalDst
	}

	hostaddress := strings.Split(r.URL.Host, ":")[0]
	isHostLanAddress := isLanAddress(hostaddress)

	// Phase 2: Block proxy requests targeting GateSentry's own admin UI.
	// The PAC file should route these DIRECT, so any request arriving here
	// for the admin port is suspicious (potential SSRF).
	if requestPort := extractPort(r.URL.Host); requestPort == AdminPort {
		if hostaddress == "" || isLanAddress(hostaddress) || hostaddress == "localhost" {
			Metrics.BlocksSSRF.Add(1)
			log.Printf("[SECURITY] Blocked proxy request to admin UI: %s from %s", r.URL.Host, client)
			http.Error(w, "Forbidden — proxy access to admin interface denied", http.StatusForbidden)
			return
		}
	}

	if len(r.URL.String()) > 10000 {
		http.Error(w, "URL too long", http.StatusRequestURITooLong)
		return
	}

	if r.URL.Scheme == "" {
		if h.TLS {
			r.URL.Scheme = "https"
		} else {
			r.URL.Scheme = "http"
		}
	}
	if r.URL.Host == "" {
		if r.Host != "" {
			r.URL.Host = r.Host
		} else {
			log.Printf("Request from %s has no host in URL: %v", client, r.URL)
			time.Sleep(time.Second)
			http.Error(w, "No host in request URL, and no Host header.", http.StatusBadRequest)
			return
		}
	}

	authEnabled := true
	authEnabled = IProxy.IsAuthEnabled()
	user, _, authUser := HandleAuthAndAssignUser(r, passthru, h, authEnabled, client)
	if authEnabled {
		if user == "" || user == "127.0.0.1" {
			Metrics.AuthFailures.Add(1)
			w.Header().Set("Proxy-Authenticate", "Basic realm="+"gsrealm")
			http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
			log.Printf("Missing required proxy authentication from %v to %v", r.RemoteAddr, r.URL)
			return
		} else {
			// _, userAuthStatus := IProxy.RunHandler("isaccessactive", "", &EMPTY_BYTES, passthru)
			userAccessFilterData := GSUserAccessFilterData{User: user}
			IProxy.UserAccessHandler(&userAccessFilterData)
			userAuthStatusString := userAccessFilterData.FilterResponseAction

			if DebugLogging {
				log.Println("User auth status = ", userAuthStatusString, " For user = ", user)
			}
			if userAuthStatusString == ProxyActionUserNotFound {
				Metrics.AuthFailures.Add(1)
				w.Header().Set("Proxy-Authenticate", "Basic realm="+"gsrealm")
				http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
				log.Printf("Missing required proxy authentication from %v to %v", r.RemoteAddr, r.URL)
				return
			}
			if userAuthStatusString != ProxyActionUserActive && !isHostLanAddress {
				Metrics.BlocksUser.Add(1)
				sendBlockMessageBytes(w, r, nil, userAccessFilterData.FilterResponse, nil)
				return
			}
		}
	}

	action := ACTION_NONE

	// requestUrlBytes := []byte(r.URL.String())
	// isBlockedInternet, _ := IProxy.RunHandler(FILTER_USER_ACCESS_DISABLED, "", &requestUrlBytes, passthru)
	// userAccess := GSUserAccessFilterData{User: user}
	// IProxy.UserAccessHandler(&userAccess)
	// if userAccess.FilterResponseAction == (ProxyActionBlockedInternetForUser) {
	// 	// requestUrlBytes_log := []byte(r.URL.String())
	// 	passthru.ProxyActionToLog = ProxyActionBlockedInternetForUser
	// 	// IProxy.RunHandler("log", "", &requestUrlBytes_log, passthru)
	// 	IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedInternetForUser})
	// 	showBlockPage(w, r, nil, userAccess.FilterResponse)
	// 	return
	// }

	// timeblocked, _ := IProxy.RunHandler(FILTER_TIME, "", &EMPTY_BYTES, passthru)
	timefilterData := GSTimeAccessFilterData{Url: r.URL.String(), User: user}
	IProxy.TimeAccessHandler(&timefilterData)
	if timefilterData.FilterResponseAction == string(ProxyActionBlockedTime) {
		Metrics.BlocksTime.Add(1)
		passthru.ProxyActionToLog = ProxyActionBlockedTime
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedTime})
		sendBlockMessageBytes(w, r, nil, timefilterData.FilterResponse, nil)
		return
	}

	if r.Method == "CONNECT" {
		hostport := r.URL.Host
		host, port, err := net.SplitHostPort(hostport)
		if err, ok := err.(*net.AddrError); ok && err.Err == "too many colons in address" {
			colon := strings.LastIndex(hostport, ":")
			host, port = hostport[:colon], hostport[colon+1:]
			if ip := net.ParseIP(host); ip != nil {
				r.URL.Host = net.JoinHostPort(host, port)
			}
		}
	}

	urlFilterData := GSUrlFilterData{Url: r.URL.String(), User: user}

	// isBlockedUrl, _ := IProxy.RunHandler(FILTER_ACCESS_URL, "", &requestUrlBytes, passthru)
	IProxy.UrlAccessHandler(&urlFilterData)

	if urlFilterData.FilterResponseAction == ProxyActionBlockedUrl {
		Metrics.BlocksURL.Add(1)
		passthru.ProxyActionToLog = ProxyActionBlockedUrl
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedUrl})
		sendBlockMessageBytes(w, r, nil, urlFilterData.FilterResponse, nil)
		return
	}

	if r.Method == "CONNECT" {
		action = ACTION_SSL_BUMP
	}

	requestHost, _, _ := net.SplitHostPort(r.URL.Host)
	if requestHost == "" {
		requestHost = r.URL.Host
	}

	shouldBlock, ruleMatch, ruleShouldMITM := CheckProxyRules(requestHost, user)

	// For block rules with post-response match criteria (URL patterns,
	// content-type criteria, or keyword filtering), we cannot short-circuit
	// the block here. We must proxy the request first so the response handler
	// can evaluate those criteria in steps 5-8 of the rule pipeline.
	//
	// Exception: HTTPS (CONNECT) without MITM — the response handler won't
	// run for passthrough tunnels, so we must block at the domain level.
	if shouldBlock {
		canDeferBlock := false
		if ruleMatch != nil {
			matchVal := reflect.ValueOf(ruleMatch)
			if matchVal.Kind() == reflect.Struct {
				hasPostCriteria := false
				urlRegex := matchVal.FieldByName("BlockURLRegexes")
				if urlRegex.IsValid() && urlRegex.Kind() == reflect.Slice && urlRegex.Len() > 0 {
					hasPostCriteria = true
				}
				contentTypes := matchVal.FieldByName("BlockContentTypes")
				if contentTypes.IsValid() && contentTypes.Kind() == reflect.Slice && contentTypes.Len() > 0 {
					hasPostCriteria = true
				}
				kwEnabled := matchVal.FieldByName("KeywordFilterEnabled")
				if kwEnabled.IsValid() && kwEnabled.Kind() == reflect.Bool && kwEnabled.Bool() {
					hasPostCriteria = true
				}

				// Can only defer the block if the response handler will run:
				// - HTTP requests always go through the response handler
				// - HTTPS (CONNECT) only goes through the response handler if MITM is active
				isHTTPS := r.Method == "CONNECT"
				if hasPostCriteria && (!isHTTPS || ruleShouldMITM) {
					canDeferBlock = true
					if DebugLogging {
						log.Printf("[Rule] Block rule has post-response criteria — deferring block for %s", r.URL)
					}
				}
			}
		}

		if !canDeferBlock {
			Metrics.BlocksRule.Add(1)
			log.Printf("[Proxy] Blocking request to %s by rule", r.URL.String())
			passthru.ProxyActionToLog = ProxyActionBlockedUrl
			ruleName := extractRuleName(ruleMatch)
			LogProxyActionWithRule(r.URL.String(), user, ProxyActionBlockedUrl, ruleName)
			var blockContent []byte
			if IProxy.RuleBlockPageHandler != nil {
				blockContent = IProxy.RuleBlockPageHandler(requestHost, ruleName)
			}
			if blockContent == nil {
				blockContent = []byte("Blocked by proxy rule")
			}
			sendBlockMessageBytes(w, r, nil, blockContent, nil)
			return
		}
	}

	ruleMatched := ruleMatch != nil
	if ruleMatch != nil {
		passthru.UserData = ruleMatch
	}

	shouldMitm := IProxy.DoMitm(r.URL.Host)

	if ruleMatched {
		shouldMitm = ruleShouldMITM
	}

	if DebugLogging {
		log.Println("Should MITM = ", shouldMitm, " currentAction = "+action, " for ", r.URL.String())
	}

	if isHostLanAddress {
		// LAN addresses bypass MITM by default, but explicit rules can override.
		if !ruleMatched {
			action = ACTION_NONE
		}
	}

	if shouldMitm == false {
		action = ACTION_NONE
	}

	if !ruleMatched {
		isExceptionUrl := IProxy.IsExceptionUrl(r.URL.String())
		if isExceptionUrl {
			action = ACTION_NONE
		}
	}

	if action == ACTION_SSL_BUMP {
		Metrics.ConnectTotal.Add(1)
		Metrics.MITMTotal.Add(1)
		HandleSSLBump(r, w, user, authUser, passthru, IProxy)
		return
	}

	if r.Method == "CONNECT" {
		Metrics.ConnectTotal.Add(1)
		Metrics.DirectTotal.Add(1)
		// requestUrlBytes_log := []byte(r.URL.String())
		passthru.ProxyActionToLog = ProxyActionSSLDirect
		// IProxy.RunHandler("log", "", &requestUrlBytes_log, passthru)
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionSSLDirect})
		HandleSSLConnectDirect(r, w, user, passthru)
		return
	}

	// Block TRACE method — RFC 9110 §9.3.8. TRACE reflects the request
	// (including Cookie/Authorization) in the response body, enabling
	// Cross-Site Tracing (XST) credential theft. Standard proxy practice
	// is to refuse TRACE rather than forward it.
	if r.Method == "TRACE" {
		http.Error(w, "TRACE method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Upgrade") == "websocket" {
		Metrics.WebSocketTotal.Add(1)
		Metrics.ActiveWebSocket.Add(1)
		defer Metrics.ActiveWebSocket.Add(-1)
		HandleWebsocketConnection(r, w)
		return
	}

	// Count actual XFF entries across all header lines. RFC 7239 allows
	// comma-separated IPs in a single X-Forwarded-For header, so we must
	// split and count rather than just counting header lines.
	xffCount := 0
	for _, line := range r.Header["X-Forwarded-For"] {
		xffCount += len(strings.Split(line, ","))
	}
	if xffCount >= 10 {
		http.Error(w, "Proxy forwarding loop", http.StatusBadRequest)
		log.Printf("Proxy forwarding loop from %s to %v (%d XFF entries)", r.Header.Get("X-Forwarded-For"), r.URL, xffCount)
		return
	}

	// Loop detection: check for our private loop-detection header and Via.
	// We use X-GateSentry-Loop (a private header that upstream servers ignore)
	// instead of adding Via to outgoing requests, because Via triggers nginx's
	// gzip_proxied off default and kills compression for millions of servers.
	if r.Header.Get("X-GateSentry-Loop") == ViaIdentifier {
		log.Printf("[SECURITY] Proxy loop detected via X-GateSentry-Loop from %s to %v", client, r.URL)
		http.Error(w, "Proxy loop detected", http.StatusLoopDetected)
		return
	}
	if viaHeader := r.Header.Get("Via"); viaHeader != "" {
		if strings.Contains(strings.ToLower(viaHeader), strings.ToLower(ViaIdentifier)) {
			log.Printf("[SECURITY] Proxy loop detected via Via header from %s to %v", client, r.URL)
			http.Error(w, "Proxy loop detected", http.StatusLoopDetected)
			return
		}
	}

	// Phase 3: Preserve Accept-Encoding for upstream but normalize to gzip-only
	// (the only compression we can decompress for content scanning).
	// With DisableCompression: true on the transport, the raw Content-Encoding
	// from upstream passes through for Path A (stream passthrough).
	clientAcceptsGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	if r.Header.Get("Accept-Encoding") != "" {
		if clientAcceptsGzip {
			r.Header.Set("Accept-Encoding", "gzip")
		} else {
			r.Header.Del("Accept-Encoding")
		}
	}

	var rt http.RoundTripper
	if h.rt == nil {
		rt = httpTransport
	} else {
		rt = h.rt
	}

	if r.ContentLength == 0 {
		r.Body.Close()
		r.Body = nil
	}

	removeHopByHopHeaders(r.Header)

	// Add a private loop-detection header to outgoing requests.
	// We deliberately do NOT set the standard Via header on outgoing requests
	// because nginx's default gzip_proxied=off refuses to compress responses
	// when Via is present — killing gzip for millions of default-configured
	// servers. The Via header is still added to responses back to the client
	// (in copyResponseHeader) for RFC 7230 §5.7.1 compliance.
	r.Header.Set("X-GateSentry-Loop", ViaIdentifier)
	// Strip any existing Via header from the client — it's not ours to forward
	// and it would also trigger the same nginx gzip issue at upstream.
	r.Header.Del("Via")

	Metrics.HTTPTotal.Add(1)
	upstreamStart := time.Now()
	resp, err := rt.RoundTrip(r)
	Metrics.UpstreamDuration.Observe(time.Since(upstreamStart))
	if err != nil {
		Metrics.ErrorsUpstream.Add(1)
		log.Printf("error fetching %s: %s", r.URL, err)
		errorData := &GSProxyErrorData{Error: err.Error()}
		IProxy.ProxyErrorHandler(errorData)
		// Transport errors (upstream unreachable, malformed response, TLS failure)
		// must always return 502 Bad Gateway per HTTP semantics.
		// The ProxyErrorHandler is notified for logging/metrics but does not
		// control the HTTP status code for transport-level failures.
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Phase 1: Sanitize response headers before any processing.
	// Rejects responses with conflicting Content-Length, negative Content-Length,
	// response splitting (\r\n in header values), and strips null bytes.
	if reason := sanitizeResponseHeaders(resp); reason != "" {
		log.Printf("[SECURITY] Rejecting response from %s: %s", r.URL, reason)
		errorData := &GSProxyErrorData{Error: "Upstream response rejected: " + reason}
		IProxy.ProxyErrorHandler(errorData)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Step 5: Check URL regex match criteria from the matched rule.
	// If patterns exist but NONE match the request URL, the rule is skipped
	// (the request is allowed through as if the rule didn't match).
	// If patterns exist and one matches, continue to step 6/7/8.
	if passthru.UserData != nil {
		matchVal := reflect.ValueOf(passthru.UserData)
		if matchVal.Kind() == reflect.Struct {
			urlRegexField := matchVal.FieldByName("BlockURLRegexes")

			if urlRegexField.IsValid() && urlRegexField.Kind() == reflect.Slice && urlRegexField.Len() > 0 {
				requestURL := r.URL.String()
				matched := false
				for i := 0; i < urlRegexField.Len(); i++ {
					patternVal := urlRegexField.Index(i)
					if patternVal.Kind() == reflect.String {
						pattern := patternVal.String()
						ok, err := regexp.MatchString(pattern, requestURL)
						if DebugLogging {
							log.Printf("URL regex match: pattern %s on URL %s: %v (err: %v)", pattern, requestURL, ok, err)
						}
						if err == nil && ok {
							matched = true
							break
						}
					}
				}

				if !matched {
					// URL patterns exist but none matched — rule doesn't apply.
					// Allow the request through (skip this rule).
					if DebugLogging {
						log.Printf("[Rule] URL patterns didn't match %s — skipping rule", requestURL)
					}
					// Fall through to normal response delivery below
					passthru.UserData = nil // Clear the rule match so steps 6-8 don't fire
				}
			}
		}
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, ";") {
		t := strings.Split(contentType, ";")
		if len(t) > 0 {
			contentType = t[0]
		}
	}
	if DebugLogging {
		log.Println("Content type is = ", contentType, " for ", r.URL.String())
	}

	// Step 6: Check per-rule BlockContentTypes — if the rule has content-type
	// criteria, match the response Content-Type against the list.
	// If patterns exist but NONE match, the rule doesn't apply (skip it).
	// If a pattern matches, continue to step 7/8.
	if contentType != "" && passthru.UserData != nil {
		matchVal := reflect.ValueOf(passthru.UserData)
		if matchVal.Kind() == reflect.Struct {
			ctField := matchVal.FieldByName("BlockContentTypes")
			if ctField.IsValid() && ctField.Kind() == reflect.Slice && ctField.Len() > 0 {
				ctMatched := false
				for i := 0; i < ctField.Len(); i++ {
					blocked := strings.ToLower(strings.TrimSpace(ctField.Index(i).String()))
					if blocked != "" && strings.Contains(contentType, blocked) {
						ctMatched = true
						break
					}
				}
				if !ctMatched {
					// Content-type criteria exist but none matched — rule doesn't apply.
					if DebugLogging {
						log.Printf("[Rule] Content-type %q didn't match any blocked types — skipping rule for %s", contentType, r.URL)
					}
					passthru.UserData = nil // Clear the rule match so steps 7-8 don't fire
				}
			}
		}
	}

	// Step 8 (pre-keyword): Apply rule action if all match criteria passed.
	// If we still have a matched rule at this point (UserData not cleared by
	// steps 5-6), and the rule's action is "block", block the request now.
	// Keyword scanning (step 7) happens in the buffer pipeline and can also
	// force a block regardless of rule action.
	if passthru.UserData != nil {
		matchVal := reflect.ValueOf(passthru.UserData)
		if matchVal.Kind() == reflect.Struct {
			actionField := matchVal.FieldByName("ShouldBlock")
			if actionField.IsValid() && actionField.Kind() == reflect.Bool && actionField.Bool() {
				Metrics.BlocksRule.Add(1)
				// All match criteria satisfied and action is "block"
				requestURL := r.URL.String()
				passthru.ProxyActionToLog = ProxyActionBlockedUrl
				ruleName := extractRuleName(ruleMatch)
				IProxy.LogHandler(GSLogData{Url: requestURL, User: user, Action: ProxyActionBlockedUrl, RuleName: ruleName})

				var blockContent []byte
				if IProxy.RuleBlockPageHandler != nil {
					host, _, _ := net.SplitHostPort(r.URL.Host)
					if host == "" {
						host = r.URL.Host
					}
					blockContent = IProxy.RuleBlockPageHandler(host, ruleName)
				}
				if blockContent == nil {
					blockContent = []byte("Blocked by proxy rule")
				}

				if contentType != "" && isImage(contentType) {
					sendInsecureBlockBytes(w, r, resp, blockContent, &contentType)
				} else {
					sendBlockMessageBytes(w, r, nil, blockContent, nil)
				}
				return
			}
		}
	}

	// Phase 3: Three-path response pipeline.
	// Classify by Content-Type and route to the appropriate processing path.
	// HEAD requests always use Path A — no body to scan, pass upstream headers through.
	pipeline := classifyContentType(contentType)
	if r.Method == "HEAD" {
		pipeline = pipelineStream
	}
	if DebugLogging {
		pathNames := [...]string{"Stream", "Peek", "Buffer"}
		log.Printf("[Phase3] %s → Path %s (%s)", r.URL, pathNames[pipeline], contentType)
	}

	switch pipeline {
	case pipelineStream:
		Metrics.PipelineStream.Add(1)
		// PATH A: Stream Passthrough — JS, CSS, fonts, JSON, binary, downloads.
		//
		// If upstream already sent Content-Encoding (e.g. gzip), pass it through
		// unchanged (true end-to-end compression).
		//
		// If upstream did NOT compress (common: nginx default gzip_proxied=off
		// refuses to compress when it sees our Via header), the proxy compresses
		// compressible text types (JS, CSS, JSON, XML, SVG, etc.) itself.
		// This avoids a massive performance penalty for the millions of nginx
		// sites running default config.
		upstreamCompressed := resp.Header.Get("Content-Encoding") != ""
		proxyCompress := !upstreamCompressed && clientAcceptsGzip &&
			!isLanAddress(client) && isCompressibleType(contentType) &&
			r.Method != "HEAD" && resp.ContentLength > 256

		if proxyCompress {
			// We'll gzip-compress on the fly — remove Content-Length (size changes)
			// and set Content-Encoding before writing headers.
			resp.Header.Set("Content-Encoding", "gzip")
			resp.Header.Del("Content-Length")
		} else if resp.ContentLength >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		}
		copyResponseHeader(w, resp)

		// HEAD responses have no body — skip the copy to avoid blocking
		// on a connection that will never send data.
		if r.Method == "HEAD" {
			break
		}

		if proxyCompress {
			gzw, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
			dest := &DataPassThru{Writer: gzw, Contenttype: contentType, Passthru: passthru}
			if flusher, ok := w.(http.Flusher); ok {
				streamWithFlusher(dest, resp.Body, flusher)
			} else {
				io.Copy(dest, resp.Body)
			}
			gzw.Close()
		} else {
			dest := &DataPassThru{Writer: w, Contenttype: contentType, Passthru: passthru}
			if flusher, ok := w.(http.Flusher); ok {
				streamWithFlusher(dest, resp.Body, flusher)
			} else {
				io.Copy(dest, resp.Body)
			}
		}

	case pipelinePeek:
		Metrics.PipelinePeek.Add(1)
		// PATH B: Peek & Stream — images, video, audio.
		// Read first 4KB for filetype detection and content filter check,
		// then stream the remainder without full-body buffering.
		body, wasDecompressed := decompressResponseBody(resp)
		if wasDecompressed {
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Content-Length") // size changed after decompression
		}

		peekBuf := make([]byte, PeekSize)
		n, peekErr := io.ReadAtLeast(body, peekBuf, 1)
		peekBuf = peekBuf[:n]

		if n == 0 && peekErr != nil {
			// Empty or unreadable body — just forward headers
			copyResponseHeader(w, resp)
			return
		}

		// Detect actual file type from magic bytes
		kind, _ := filetype.Match(peekBuf)
		peekContentType := contentType
		if kind != filetype.Unknown {
			if DebugLogging {
				log.Printf("[Phase3] Peek filetype: %s MIME: %s", kind.Extension, kind.MIME.Value)
			}
			peekContentType = kind.MIME.Value
		}

		// Run content filter for media types
		if filetype.IsImage(peekBuf) || filetype.IsVideo(peekBuf) || filetype.IsAudio(peekBuf) ||
			isImage(peekContentType) || isVideo(peekContentType) {
			contentFilterData := GSContentFilterData{
				Url:         r.URL.String(),
				ContentType: peekContentType,
				Content:     peekBuf,
			}
			IProxy.ContentHandler(&contentFilterData)
			if contentFilterData.FilterResponseAction == ProxyActionBlockedMediaContent {
				Metrics.BlocksMedia.Add(1)
				passthru.ProxyActionToLog = ProxyActionBlockedMediaContent
				IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedMediaContent, RuleName: extractRuleName(passthru.UserData)})
				copyResponseHeader(w, resp)
				dest := &DataPassThru{Writer: w, Contenttype: peekContentType, Passthru: passthru}
				var reasonForBlockArray []string
				if err := json.Unmarshal(contentFilterData.FilterResponse, &reasonForBlockArray); err != nil {
					reasonForBlockArray = []string{"", "Error", err.Error()}
				} else {
					reasonForBlockArray = append([]string{"", "Image blocked by Gatesentry", "Reason(s) for blocking"}, reasonForBlockArray...)
				}
				emptyImage, _ := createEmptyImage(500, 500, "jpeg", reasonForBlockArray)
				dest.Write(emptyImage)
				return
			}
		}

		// Allowed — write headers, peek bytes, then stream the rest
		copyResponseHeader(w, resp)
		dest := &DataPassThru{Writer: w, Contenttype: peekContentType, Passthru: passthru}
		dest.Write(peekBuf)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
			streamWithFlusher(dest, body, flusher)
		} else {
			io.Copy(dest, body)
		}

	case pipelineBuffer:
		Metrics.PipelineBuffer.Add(1)
		// PATH C: Buffer & Scan — text/html and unknown content types.
		// Full body buffering (up to MaxContentScanSize) for text scanning.
		// This preserves the existing scanning behaviour for HTML.

		// Decompress if upstream sent gzip/deflate (since we scan raw text)
		body, wasDecompressed := decompressResponseBody(resp)
		if wasDecompressed {
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Content-Length")
		}

		var buf bytes.Buffer
		limitedReader := &io.LimitedReader{R: body, N: int64(MaxContentScanSize)}
		teeReader := io.TeeReader(limitedReader, &buf)

		localCopyData, err := io.ReadAll(teeReader)
		if err != nil {
			log.Printf("error while reading response body (URL: %s): %s", r.URL, err)
		}

		if limitedReader.N == 0 {
			// Body exceeds MaxContentScanSize — deliver what we have, stream the rest
			log.Println("response body too long to filter:", r.URL)
			copyResponseHeader(w, resp)
			dest := &DataPassThru{Writer: w, Contenttype: contentType, Passthru: passthru}
			dest.Write(localCopyData)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
				streamWithFlusher(dest, body, flusher)
			} else {
				_, copyErr := io.Copy(dest, body)
				if copyErr != nil {
					log.Printf("error while copying response (URL: %s): %s", r.URL, copyErr)
				}
			}
			return
		}

		// Detect actual file type from body bytes (catches mislabeled Content-Type)
		kind, _ := filetype.Match(localCopyData)
		if kind != filetype.Unknown {
			if DebugLogging {
				log.Printf("File type: %s. MIME: %s\n", kind.Extension, kind.MIME.Value)
			}
			contentType = kind.MIME.Value
		}

		// Run media scanner (handles mislabeled Content-Type → actual image)
		responseSentMedia, proxyActionTaken := ScanMedia(localCopyData, contentType, r, w, resp, buf, passthru)
		if responseSentMedia {
			Metrics.BlocksMedia.Add(1)
			passthru.ProxyActionToLog = proxyActionTaken
			IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: proxyActionTaken, RuleName: extractRuleName(passthru.UserData)})
			return
		}

		// Run text/HTML keyword scanner — only when the matched rule enables it.
		// Keyword scanning is a per-rule filter, not a global pipeline step.
		if isKeywordFilterEnabled(passthru) {
			responseSentText, proxyActionTaken := ScanText(localCopyData, contentType, r, w, resp, buf, passthru)
			if responseSentText {
				Metrics.BlocksKeyword.Add(1)
				passthru.ProxyActionToLog = proxyActionTaken
				IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: proxyActionTaken, RuleName: extractRuleName(passthru.UserData)})
				return
			}
		}

		// Deliver the buffered response
		if clientAcceptsGzip && !isLanAddress(client) && len(localCopyData) > 1000 {
			resp.Header.Set("Content-Encoding", "gzip")
			copyResponseHeader(w, resp)
			gzw := gzip.NewWriter(w)
			dest := &DataPassThru{Writer: gzw, Contenttype: contentType, Passthru: passthru}
			dest.Write(localCopyData)
			gzw.Close()
		} else {
			// For HEAD responses, preserve upstream's Content-Length since there's no body to measure.
			// For GET responses, use the actual body length we read.
			if r.Method == "HEAD" && resp.ContentLength >= 0 {
				w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
			} else {
				w.Header().Set("Content-Length", strconv.Itoa(len(localCopyData)))
			}
			copyResponseHeader(w, resp)
			dest := &DataPassThru{Writer: w, Contenttype: contentType, Passthru: passthru}
			dest.Write(localCopyData)
		}
	}
}

// isKeywordFilterEnabled checks whether the rule match in passthru has keyword
// filtering enabled. Returns false if no rule matched or the flag is unset.
func isKeywordFilterEnabled(passthru *GSProxyPassthru) bool {
	if passthru == nil || passthru.UserData == nil {
		return false
	}
	matchVal := reflect.ValueOf(passthru.UserData)
	if matchVal.Kind() == reflect.Struct {
		field := matchVal.FieldByName("KeywordFilterEnabled")
		if field.IsValid() && field.Kind() == reflect.Bool {
			return field.Bool()
		}
	}
	return false
}

func sendInsecureBlockBytes(w http.ResponseWriter, r *http.Request, resp *http.Response, content []byte, contentType *string) {
	// string ends with

	if contentType != nil && isImage(*contentType) {
		reasonForBlockArray := append([]string{"", "Image blocked by Gatesentry", "Reason(s) for blocking", "1. The content type is blocked"})
		emptyImage, _ := createEmptyImage(500, 500, "jpeg", reasonForBlockArray)
		w.Header().Set("Content-Type", "image/jpeg; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		w.Write(emptyImage)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(http.StatusForbidden)
	w.Write(content)
}

func sendBlockMessageBytes(w http.ResponseWriter, r *http.Request, resp *http.Response, content []byte, contentType *string) {
	// check if request is https
	if strings.Contains(r.URL.String(), ":443") {
		log.Println("[Proxy] Sending block page for https request")
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			sendInsecureBlockBytes(w, r, resp, content, contentType)
			return
		}
		defer conn.Close()
		conn.Write([]byte("HTTP/1.1 200 OK\r\n\r\n"))

		// Extract the hostname for certificate generation
		blockHost := r.URL.Host
		if h, _, splitErr := net.SplitHostPort(blockHost); splitErr == nil {
			blockHost = h
		}

		tlsConfig, err := createBlockPageTLSConfig(blockHost)
		if err != nil {
			fmt.Println("[Proxy][Error:showBlockPage] Error creating block page certificate:", err)
			conn.Close()
			return
		}

		tlsConn := tls.Server(conn, tlsConfig)
		err = tlsConn.Handshake()
		if err != nil {
			log.Println("[Proxy][Error:showBlockPage] Handshake failed:", err)
			conn.Close()
			return
		}

		headers := fmt.Sprintf("HTTP/1.1 403 Forbidden\r\nContent-Type: text/html; charset=utf-8\r\nContent-Length: %d\r\nCache-Control: no-store, no-cache, must-revalidate, max-age=0\r\nPragma: no-cache\r\nExpires: 0\r\nConnection: close\r\n\r\n", len(content))
		_, err = tlsConn.Write([]byte(headers))
		if err != nil {
			log.Println("[Proxy][Error:showBlockPage] writing headers to connection", err)
			return
		}
		_, err = tlsConn.Write(content)
		if err != nil {
			conn.Close()
			return
		}

		tlsConn.Close()
	} else {
		sendInsecureBlockBytes(w, r, resp, content, contentType)
	}

}

// CheckProxyRules checks proxy rules for a given host and user.
// Returns: shouldBlock (bool), ruleMatch (interface{}), shouldMITM (bool)
func CheckProxyRules(host string, user string) (bool, interface{}, bool) {
	if IProxy == nil || IProxy.RuleMatchHandler == nil {
		return false, nil, false
	}

	ruleMatch := IProxy.RuleMatchHandler(host, user)
	if ruleMatch == nil {
		return false, nil, false
	}

	matchVal := reflect.ValueOf(ruleMatch)
	if matchVal.Kind() != reflect.Struct {
		return false, nil, false
	}

	matchedField := matchVal.FieldByName("Matched")
	if !matchedField.IsValid() || matchedField.Kind() != reflect.Bool || !matchedField.Bool() {
		return false, nil, false
	}

	shouldBlockField := matchVal.FieldByName("ShouldBlock")
	mitmField := matchVal.FieldByName("ShouldMITM")

	shouldBlock := false
	shouldMITM := false

	if shouldBlockField.IsValid() && shouldBlockField.Kind() == reflect.Bool && shouldBlockField.Bool() {
		shouldBlock = true
	}

	if mitmField.IsValid() && mitmField.Kind() == reflect.Bool {
		shouldMITM = mitmField.Bool()
	}

	return shouldBlock, ruleMatch, shouldMITM
}

// LogProxyAction logs a proxy action with the given URL, user, and action
func LogProxyAction(url string, user string, action ProxyAction) {
	LogProxyActionWithRule(url, user, action, "")
}

// LogProxyActionWithRule logs a proxy action including the name of the matched rule
func LogProxyActionWithRule(url string, user string, action ProxyAction, ruleName string) {
	if IProxy != nil && IProxy.LogHandler != nil {
		IProxy.LogHandler(GSLogData{Url: url, User: user, Action: action, RuleName: ruleName})
	}
}

// extractRuleName extracts the rule name from a RuleMatch interface{} via reflection
func extractRuleName(ruleMatch interface{}) string {
	if ruleMatch == nil {
		return ""
	}
	mv := reflect.ValueOf(ruleMatch)
	if mv.Kind() != reflect.Struct {
		return ""
	}
	ruleField := mv.FieldByName("Rule")
	if !ruleField.IsValid() || ruleField.IsNil() {
		return ""
	}
	nameField := ruleField.Elem().FieldByName("Name")
	if nameField.IsValid() && nameField.Kind() == reflect.String {
		return nameField.String()
	}
	return ""
}

// ViaIdentifier is the token used in Via headers for loop detection.
const ViaIdentifier = "gatesentry"

// sanitizeResponseHeaders validates upstream response headers and returns an
// error description if the response should be rejected. It also sanitises
// individual header values in-place (strips null bytes, detects response
// splitting characters). Called before copyResponseHeader.
func sanitizeResponseHeaders(resp *http.Response) string {
	// 1. Reject conflicting / invalid Content-Length
	clValues := resp.Header.Values("Content-Length")
	if len(clValues) > 1 {
		// RFC 9110 §8.6: multiple differing Content-Length values MUST be rejected
		first := clValues[0]
		for _, v := range clValues[1:] {
			if v != first {
				return "conflicting Content-Length values"
			}
		}
		// All identical — deduplicate to a single value
		resp.Header.Set("Content-Length", first)
	}
	if len(clValues) >= 1 {
		cl, err := strconv.ParseInt(strings.TrimSpace(clValues[0]), 10, 64)
		if err != nil || cl < 0 {
			return "invalid Content-Length value"
		}
	}

	// 2. Scan all header values for response splitting / null byte injection
	for key, values := range resp.Header {
		for i, v := range values {
			if strings.ContainsAny(v, "\r\n") {
				log.Printf("[SECURITY] Response splitting detected in header %q from upstream", key)
				return "response splitting in header: " + key
			}
			// Strip null bytes in-place (prevents C-parser header injection)
			if strings.Contains(v, "\x00") {
				log.Printf("[SECURITY] Null bytes stripped from header %q", key)
				resp.Header[key][i] = strings.ReplaceAll(v, "\x00", "")
			}
		}
	}

	return "" // headers OK
}

// classifyContentType determines which response pipeline path to use based
// on the response Content-Type. This drives the Phase 3 three-path router:
//   - pipelineBuffer (Path C): text/html and xhtml — needs full-body text scanning
//   - pipelinePeek (Path B): media types — peek 4KB for filetype + content filter
//   - pipelineStream (Path A): everything else — zero-copy stream passthrough
func classifyContentType(ct string) int {
	switch {
	case strings.HasPrefix(ct, "text/html"),
		strings.HasPrefix(ct, "application/xhtml+xml"),
		ct == "":
		return pipelineBuffer
	case strings.HasPrefix(ct, "image/"),
		strings.HasPrefix(ct, "video/"),
		strings.HasPrefix(ct, "audio/"):
		return pipelinePeek
	default:
		return pipelineStream
	}
}

// isCompressibleType returns true for content types that benefit from gzip
// compression. If the upstream didn't compress the response (some servers
// don't enable gzip at all), the proxy compresses these types itself as a
// fallback so clients still get compressed responses.
func isCompressibleType(ct string) bool {
	switch {
	case strings.HasPrefix(ct, "text/"),
		strings.HasPrefix(ct, "application/javascript"),
		strings.HasPrefix(ct, "application/x-javascript"),
		strings.HasPrefix(ct, "application/json"),
		strings.HasPrefix(ct, "application/xml"),
		strings.HasPrefix(ct, "application/xhtml+xml"),
		strings.HasPrefix(ct, "application/rss+xml"),
		strings.HasPrefix(ct, "application/atom+xml"),
		strings.HasPrefix(ct, "image/svg+xml"):
		return true
	default:
		return false
	}
}

// streamWithFlusher streams data from src to dst, calling Flush after each
// read chunk for progressive delivery (SSE, chunked streams, drip endpoints).
func streamWithFlusher(dst io.Writer, src io.Reader, flusher http.Flusher) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			flusher.Flush()
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// decompressResponseBody returns a reader that decompresses the response body
// if Content-Encoding is gzip or deflate. The second return value indicates
// whether decompression is active (caller should delete Content-Encoding).
// If the encoding is unsupported or decompression fails, the original body
// is returned unchanged.
func decompressResponseBody(resp *http.Response) (io.Reader, bool) {
	ce := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	switch ce {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			log.Printf("[Phase3] Failed to create gzip reader: %v", err)
			return resp.Body, false
		}
		return gr, true
	case "deflate":
		return flate.NewReader(resp.Body), true
	default:
		return resp.Body, false
	}
}

// copyResponseHeader writes resp's header and status code to w.
// It sanitises headers via sanitizeResponseHeaders, skips hop-by-hop headers
// in the response direction, adds a Via header, and sets X-Content-Type-Options.
func copyResponseHeader(w http.ResponseWriter, resp *http.Response) {
	newHeader := w.Header()

	// Build set of response hop-by-hop headers to skip
	respHopByHop := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Proxy-Connection":    true,
		"TE":                  true,
		"Trailer":             true,
		"Transfer-Encoding":   true,
	}
	if c := resp.Header.Get("Connection"); c != "" {
		for _, key := range strings.Split(c, ",") {
			respHopByHop[http.CanonicalHeaderKey(strings.TrimSpace(key))] = true
		}
	}

	for key, values := range resp.Header {
		if key == "Content-Length" {
			continue
		}
		if respHopByHop[key] {
			continue
		}
		for _, v := range values {
			newHeader.Add(key, v)
		}
	}

	// Add Via header (RFC 7230 §5.7.1)
	existingVia := resp.Header.Get("Via")
	viaValue := fmt.Sprintf("%d.%d %s", resp.ProtoMajor, resp.ProtoMinor, ViaIdentifier)
	if existingVia != "" {
		viaValue = existingVia + ", " + viaValue
	}
	newHeader.Set("Via", viaValue)

	// Defensive header: prevent MIME-type sniffing in browsers
	if newHeader.Get("X-Content-Type-Options") == "" {
		newHeader.Set("X-Content-Type-Options", "nosniff")
	}

	w.WriteHeader(resp.StatusCode)
}

// removeHopByHopHeaders removes header fields listed in
// http://tools.ietf.org/html/draft-ietf-httpbis-p1-messaging-14#section-7.1.3.1
func removeHopByHopHeaders(h http.Header) {
	toRemove := HOP_BY_HOP
	if c := h.Get("Connection"); c != "" {
		for _, key := range strings.Split(c, ",") {
			toRemove = append(toRemove, strings.TrimSpace(key))
		}
	}
	for _, key := range toRemove {
		h.Del(key)
	}
}

// A hijackedConn is a connection that has been hijacked (to fulfill a CONNECT
// request).
type hijackedConn struct {
	net.Conn
	io.Reader
}

func (hc *hijackedConn) Read(b []byte) (int, error) {
	return hc.Reader.Read(b)
}
