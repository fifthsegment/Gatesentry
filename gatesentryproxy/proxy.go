package gatesentryproxy

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/h2non/filetype"
)

var IProxy *GSProxy
var MaxContentScanSize int64 = 1e7 // Reduced from 100MB to 10MB for low-spec hardware
var DebugLogging = false           // Disable verbose logging for performance
var dialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}
var ip6Loopback = net.ParseIP("::1")
var httpTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	Dial:                  dialer.Dial,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
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
	IProxy.UsersCache = map[string]GSUserCached{}
	return &proxy
}

func (p *GSProxy) RegisterHandler(id string, f func(*[]byte, *GSResponder, *GSProxyPassthru)) {
	h := NewGSHandler(id, f)
	if p.Handlers == nil {
		p.Handlers = map[string][]*GSHandler{}
	}
	log.Printf("Registering Handler for %s", id)
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

	// transparent marks a handler serving a connection that reached the
	// gateway by routing rather than by proxy configuration, for logs.
	transparent bool

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

// accountingWriter reports bytes successfully written without retaining a copy
// of the payload. Proxy responses and CONNECT tunnels can be arbitrarily long,
// so accounting must stay constant-memory.
type accountingWriter struct {
	writer      io.Writer
	contentType string
	passthru    *GSProxyPassthru
}

func (w *accountingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 && IProxy != nil && IProxy.ContentSizeHandler != nil {
		user := ""
		if w.passthru != nil {
			user = w.passthru.User
		}
		IProxy.ContentSizeHandler(GSContentSizeFilterData{
			Url:         "",
			ContentType: w.contentType,
			ContentSize: int64(n),
			User:        user,
		})
	}
	return n, err
}

func (h ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
				w.Header().Set("Proxy-Authenticate", "Basic realm="+"gsrealm")
				http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
				log.Printf("Missing required proxy authentication from %v to %v", r.RemoteAddr, r.URL)
				return
			}
			if userAuthStatusString != ProxyActionUserActive && !isHostLanAddress {
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
		passthru.ProxyActionToLog = ProxyActionBlockedTime
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedTime, ClientIP: client, Layer: "explicit_proxy"})
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

	requestHost, _, _ := net.SplitHostPort(r.URL.Host)
	if requestHost == "" {
		requestHost = r.URL.Host
	}
	layer := "explicit_proxy"
	if h.transparent {
		layer = "transparent_proxy"
	}

	// The policy decides first. Inside an inspected connection this runs
	// again for every decrypted request, which is what lets a rule see the
	// URL path and not just the host.
	policy := CheckProxyRules(requestHost, user, client)
	passthru.Policy = policy
	if policy != nil && (policy.Block || policy.BlocksURL(r.URL.String())) {
		passthru.ProxyActionToLog = ProxyActionBlockedUrl
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedUrl, ClientIP: client, Layer: layer, Reason: policy.Reason})
		sendBlockMessageBytes(w, r, nil, policyBlockPage(policy.Reason), nil)
		return
	}
	policyAllows := policy != nil && policy.Allow

	urlFilterData := GSUrlFilterData{Url: r.URL.String(), User: user}
	if !policyAllows {
		IProxy.UrlAccessHandler(&urlFilterData)
	}

	if urlFilterData.FilterResponseAction == ProxyActionBlockedUrl {
		passthru.ProxyActionToLog = ProxyActionBlockedUrl
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedUrl, ClientIP: client, Layer: layer})
		sendBlockMessageBytes(w, r, nil, urlFilterData.FilterResponse, nil)
		return
	}

	fileExt := getFileExtensionFromUrl(r.URL.String())
	fileMime := getMimeByExtension(fileExt)
	contentTypeScan := &GSContentTypeFilterData{Url: r.URL.String(), ContentType: fileMime}
	if !policyAllows {
		IProxy.ContentTypeHandler(contentTypeScan)
	}

	if DebugLogging {
		log.Println("Url File extension = ", fileExt, " mime ", fileMime)
	}

	if contentTypeScan.FilterResponseAction == ProxyActionBlockedFileType {
		passthru.ProxyActionToLog = ProxyActionBlockedUrl
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedUrl, ClientIP: client, Layer: "explicit_proxy"})
		// sendBlockMessageBytes(w, r, nil, urlFilterData.FilterResponse, nil)
		r.URL.Host = "blocked.gatesentryguard.com"
		r.URL.Scheme = "https"
		r.URL.Path = "/file/gatesentryguard/blocked.svg"
		// query := r.URL.Query()
		// query.Add("text", "Blocked")
		// r.URL.RawQuery = query.Encode()
		w.Header().Set("Location", r.URL.String())
		w.WriteHeader(http.StatusMovedPermanently)
		log.Println("Modified url = ", r.URL.String())
		// return
	}

	if r.Method == "CONNECT" {
		action = ACTION_SSL_BUMP
	}

	shouldMitm := IProxy.DoMitm(r.URL.Host)
	if policy != nil && policy.Inspect {
		// URL and response-type rules exist only inside the connection.
		shouldMitm = true
	}

	if DebugLogging {
		log.Println("Should MITM = ", shouldMitm, " currentAction = "+action, " for ", r.URL.String())
	}

	if isHostLanAddress {
		action = ACTION_NONE
		// modified = true
	}

	if shouldMitm == false {
		action = ACTION_NONE
	}

	if policy == nil || !policy.Inspect {
		isExceptionUrl := IProxy.IsExceptionUrl(r.URL.String())
		if isExceptionUrl {
			action = ACTION_NONE
		}
	}

	if action == ACTION_SSL_BUMP {
		HandleSSLBump(r, w, user, authUser, passthru, IProxy)
		return
	}

	if r.Method == "CONNECT" {
		// requestUrlBytes_log := []byte(r.URL.String())
		passthru.ProxyActionToLog = ProxyActionSSLDirect
		// IProxy.RunHandler("log", "", &requestUrlBytes_log, passthru)
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionSSLDirect, ClientIP: client, Layer: layer})
		HandleSSLConnectDirect(r, w, user, passthru)
		return
	}

	if upgrade := strings.TrimSpace(r.Header.Get("Upgrade")); upgrade != "" {
		if strings.EqualFold(upgrade, "websocket") {
			HandleWebsocketConnection(r, w)
		} else {
			http.Error(w, "Unsupported protocol upgrade", http.StatusNotImplemented)
		}
		return
	}

	if len(r.Header["X-Forwarded-For"]) >= 10 {
		http.Error(w, "Proxy forwarding loop", http.StatusBadRequest)
		log.Printf("Proxy forwarding loop from %s to %v", r.Header.Get("X-Forwarded-For"), r.URL)
		return
	}

	gzipOK := acceptsEncoding(r.Header.Get("Accept-Encoding"), "gzip") && !isLanAddress(client)
	r.Header.Del("Accept-Encoding")

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

	resp, err := rt.RoundTrip(r)
	if err != nil {
		log.Printf("error fetching %s: %s", r.URL, err)
		// errorBytes := []byte(err.Error())
		// IProxy.RunHandler("proxyerror", "", &errorBytes, passthru)
		errorData := &GSProxyErrorData{Error: err.Error()}
		IProxy.ProxyErrorHandler(errorData)
		sendBlockMessageBytes(w, r, nil, errorData.FilterResponse, nil)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusSwitchingProtocols {
		http.Error(w, "Unsupported upstream protocol upgrade", http.StatusBadGateway)
		return
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
	// contentTypeBytes := []byte(contentType)

	// Responses forbidden from carrying a message body must not consume or
	// emit one, even if a broken upstream supplied bytes.
	if responseHasNoBody(r.Method, resp.StatusCode) {
		if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
			w.Header().Set("Content-Length", contentLength)
		} else if r.Method == http.MethodHead && resp.ContentLength >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		}
		copyResponseHeader(w, resp)
		return
	}

	// contentTypeStatusBlocked, _ := IProxy.RunHandler("contenttypeblocked", "", &contentTypeBytes, passthru)
	if policy.BlocksContentType(contentType) {
		passthru.ProxyActionToLog = ProxyActionBlockedFileType
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedFileType, ClientIP: client, Layer: layer, Reason: policy.Reason})
		sendBlockMessageBytes(w, r, nil, policyBlockPage(policy.Reason), &contentType)
		return
	}

	contentTypeData := GSContentTypeFilterData{Url: r.URL.String(), ContentType: contentType}
	if !policyAllows {
		IProxy.ContentTypeHandler(&contentTypeData)
	}

	if contentTypeData.FilterResponseAction == ProxyActionBlockedFileType {
		// requestUrlBytes_log := []byte(r.URL.String())
		passthru.ProxyActionToLog = ProxyActionBlockedFileType
		// IProxy.RunHandler("log", "", &requestUrlBytes_log, passthru)
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedFileType, ClientIP: client, Layer: layer})
		sendBlockMessageBytes(w, r, nil, BLOCKED_CONTENT_TYPE, &contentType)
		return
	}

	// Content types that no filter consumes should not pay the bounded-buffer
	// cost. Unknown and generic binary responses remain inspectable because
	// magic-byte media detection can still identify them.
	if !responseContentNeedsInspection(contentType) {
		if err := writeResponseStream(w, resp, contentType, passthru, nil, resp.Body, canDynamicCompress(r, resp, gzipOK)); err != nil {
			log.Printf("error while copying non-inspectable response (URL: %s): %s", r.URL, err)
		}
		return
	}

	// Encoded representations cannot be inspected safely without decoding.
	// Stream them unchanged rather than scanning compressed bytes.
	if resp.Header.Get("Content-Encoding") != "" {
		if err := writeResponseStream(w, resp, contentType, passthru, nil, resp.Body, false); err != nil {
			log.Printf("error while copying encoded response (URL: %s): %s", r.URL, err)
		}
		return
	}

	// A known oversized response cannot be inspected, so stream it directly
	// without allocating the scan buffer at all.
	if resp.ContentLength > MaxContentScanSize {
		log.Println("response body too long to filter:", r.URL)
		if err := writeResponseStream(w, resp, contentType, passthru, nil, resp.Body, canDynamicCompress(r, resp, gzipOK)); err != nil {
			log.Printf("error while copying response (URL: %s): %s", r.URL, err)
		}
		return
	}

	// Read at most the configured scan limit plus one sentinel byte. This
	// distinguishes an exactly-at-limit response from an oversized response
	// without retaining a second copy of the body.
	localCopyData, overLimit, err := readBoundedBody(resp.Body, MaxContentScanSize)
	if err != nil {
		log.Printf("error while reading response body (URL: %s): %s", r.URL, err)
		http.Error(w, "Bad gateway", http.StatusBadGateway)
		return
	}

	if overLimit {
		log.Println("response body too long to filter:", r.URL)
		if err := writeResponseStream(w, resp, contentType, passthru, localCopyData, resp.Body, canDynamicCompress(r, resp, gzipOK)); err != nil {
			log.Printf("error while copying response (URL: %s): %s", r.URL, err)
		}
		return
	}

	kind, _ := filetype.Match(localCopyData)
	if kind != filetype.Unknown {
		if DebugLogging {
			log.Printf("File type: %s. MIME: %s\n", kind.Extension, kind.MIME.Value)
		}
		contentType = kind.MIME.Value
	}
	responseSentMedia, proxyActionTaken := ScanMedia(localCopyData, contentType, r, w, resp, passthru)
	if responseSentMedia {
		passthru.ProxyActionToLog = proxyActionTaken
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: proxyActionTaken, ClientIP: client, Layer: "content"})
		return
	}

	responseSentText, proxyActionTaken := ScanText(localCopyData, contentType, r, w, resp, passthru)
	if responseSentText {
		passthru.ProxyActionToLog = proxyActionTaken
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: proxyActionTaken, ClientIP: client, Layer: "content"})
		return
	}

	if canDynamicCompress(r, resp, gzipOK) && len(localCopyData) > 1000 {
		resp.Header.Set("Content-Encoding", "gzip")
		stripTransformedRepresentationHeaders(resp.Header)
		addVary(resp.Header, "Accept-Encoding")
		copyResponseHeader(w, resp)
		dest := &accountingWriter{writer: w, contentType: contentType, passthru: passthru}
		gzw := gzip.NewWriter(dest)
		_, writeErr := io.Copy(gzw, bytes.NewReader(localCopyData))
		closeErr := gzw.Close()
		if writeErr != nil {
			log.Printf("error while writing response (URL: %s): %s", r.URL, writeErr)
		} else if closeErr != nil {
			log.Printf("error while closing gzip response (URL: %s): %s", r.URL, closeErr)
		}
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(len(localCopyData)))
		copyResponseHeader(w, resp)
		dest := &accountingWriter{writer: w, contentType: contentType, passthru: passthru}
		if _, err := io.Copy(dest, bytes.NewReader(localCopyData)); err != nil {
			log.Printf("error while writing response (URL: %s): %s", r.URL, err)
		}
	}

}

func acceptsEncoding(header, encoding string) bool {
	exactFound, exactAccepted := false, false
	wildcardFound, wildcardAccepted := false, false
	for _, value := range strings.Split(header, ",") {
		parts := strings.Split(value, ";")
		name := strings.TrimSpace(parts[0])
		if !strings.EqualFold(name, encoding) && name != "*" {
			continue
		}
		accepted := true
		for _, parameter := range parts[1:] {
			key, value, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if found && strings.EqualFold(strings.TrimSpace(key), "q") {
				quality, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				accepted = err == nil && quality > 0
			}
		}
		if strings.EqualFold(name, encoding) {
			exactFound, exactAccepted = true, accepted
		} else {
			wildcardFound, wildcardAccepted = true, accepted
		}
	}
	if exactFound {
		return exactAccepted
	}
	return wildcardFound && wildcardAccepted
}

func responseContentNeedsInspection(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	return mediaType == "" || mediaType == "application/octet-stream" || strings.Contains(mediaType, "html") ||
		strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/")
}

func canDynamicCompress(r *http.Request, resp *http.Response, gzipOK bool) bool {
	return gzipOK && resp.StatusCode != http.StatusPartialContent && r.Header.Get("Range") == "" &&
		resp.Header.Get("Content-Range") == "" && resp.Header.Get("Content-Encoding") == ""
}

func stripTransformedRepresentationHeaders(header http.Header) {
	for _, key := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Digest", "Content-Digest", "Content-MD5"} {
		deleteHeaderFold(header, key)
	}
}

func stripReplacementRepresentationHeaders(header http.Header) {
	stripTransformedRepresentationHeaders(header)
	deleteHeaderFold(header, "Last-Modified")
}

func deleteHeaderFold(header http.Header, name string) {
	for key := range header {
		if strings.EqualFold(key, name) {
			delete(header, key)
		}
	}
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, token := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(token), value) || strings.TrimSpace(token) == "*" {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func responseHasNoBody(method string, status int) bool {
	return method == http.MethodHead || status >= 100 && status < 200 || status == http.StatusNoContent || status == http.StatusNotModified
}

// readBoundedBody returns one buffer containing at most limit+1 bytes. The
// extra byte is retained as part of the passthrough prefix when overLimit is
// true, so no byte is dropped or reordered.
func readBoundedBody(src io.Reader, limit int64) (data []byte, overLimit bool, err error) {
	if limit < 0 {
		limit = 0
	}
	data, err = io.ReadAll(io.LimitReader(src, limit+1))
	return data, int64(len(data)) > limit, err
}

// writeResponseStream writes the already-read prefix before the unread body.
// It optionally gzip-compresses the complete ordered stream when the client
// accepts gzip and the upstream response is identity encoded.
func writeResponseStream(w http.ResponseWriter, resp *http.Response, contentType string, passthru *GSProxyPassthru, prefix []byte, body io.Reader, gzipOK bool) error {
	compress := gzipOK && resp.StatusCode != http.StatusPartialContent && resp.Header.Get("Content-Range") == "" && resp.Header.Get("Content-Encoding") == ""
	if compress {
		resp.Header.Set("Content-Encoding", "gzip")
		stripTransformedRepresentationHeaders(resp.Header)
		addVary(resp.Header, "Accept-Encoding")
	} else if resp.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
	}
	copyResponseHeader(w, resp)

	dest := &accountingWriter{writer: w, contentType: contentType, passthru: passthru}
	var output io.Writer = dest
	var gzw *gzip.Writer
	if compress {
		gzw = gzip.NewWriter(dest)
		output = gzw
	}
	_, copyErr := io.Copy(output, io.MultiReader(bytes.NewReader(prefix), body))
	if gzw != nil {
		if closeErr := gzw.Close(); copyErr == nil {
			copyErr = closeErr
		}
	}
	return copyErr
}

func sendInsecureBlockBytes(w http.ResponseWriter, r *http.Request, resp *http.Response, content []byte, contentType *string) {
	w.Header().Del("Content-Encoding")
	if contentType != nil && isImage(*contentType) {
		reasonForBlockArray := []string{"", "Image blocked by Gatesentry", "Reason(s) for blocking", "1. The content type is blocked"}
		emptyImage, _ := createEmptyImage(500, 500, "jpeg", reasonForBlockArray)
		w.Header().Set("Content-Type", "image/jpeg; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(emptyImage)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(emptyImage)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
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

		// clientHello, err := gsClientHello.ReadClientHello(conn)

		tlsConfig, err := createSelfSignedTLSConfig()
		if err != nil {
			fmt.Println("[Proxy][Error:showBlockPage] Error creating self-signed certificate:", err)
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

		_, err = tlsConn.Write([]byte("HTTP/1.1 403 Forbidden\r\n"))
		if err != nil {
			log.Println("[Proxy][Error:showBlockPage] writing to connection", err)
			return
		}
		_, err = tlsConn.Write([]byte("Content-Type: text/html\r\n\r\n"))
		if err != nil {
			log.Println("[Proxy][Error:showBlockPage] Error writing to connection", err)
			conn.Close()
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

// CheckProxyRules asks the policy service for the decision on one request. A
// nil result means no policy decides it.
func CheckProxyRules(host string, user string, clientIP string) *PolicyDecision {
	if IProxy == nil || IProxy.RuleMatchHandler == nil {
		return nil
	}
	return IProxy.RuleMatchHandler(host, user, clientIP)
}

// policyBlockPage is the page shown when a policy denies a request, naming the
// rule so the person in front of the device knows what to ask for.
func policyBlockPage(reason string) []byte {
	if IProxy != nil && IProxy.PolicyBlockPage != nil {
		return IProxy.PolicyBlockPage(reason)
	}
	return []byte("Blocked by policy: " + reason)
}

// LogProxyAction logs a proxy action with the given URL, user, and action
func LogProxyAction(url string, user string, action ProxyAction, clientIP string, layer string) {
	logProxyActionWithReason(url, user, action, clientIP, layer, "")
}

func logProxyActionWithReason(url string, user string, action ProxyAction, clientIP string, layer string, reason string) {
	if IProxy != nil && IProxy.LogHandler != nil {
		IProxy.LogHandler(GSLogData{Url: url, User: user, Action: action, ClientIP: clientIP, Layer: layer, Reason: reason})
	}
}

// copyResponseHeader writes resp's header and status code to w.
func copyResponseHeader(w http.ResponseWriter, resp *http.Response) {
	removeHopByHopHeaders(resp.Header)
	newHeader := w.Header()
	for key, values := range resp.Header {
		if key == "Content-Length" {
			continue
		}
		for _, v := range values {
			newHeader.Add(key, v)
		}
	}

	w.WriteHeader(resp.StatusCode)
}

// removeHopByHopHeaders removes header fields listed in
// http://tools.ietf.org/html/draft-ietf-httpbis-p1-messaging-14#section-7.1.3.1
func removeHopByHopHeaders(h http.Header) {
	toRemove := append([]string(nil), HOP_BY_HOP...)
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
