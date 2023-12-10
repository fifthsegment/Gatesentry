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

	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"
	"github.com/h2non/filetype"
)

var IProxy *GSProxy
var MaxContentScanSize int64 = 1e8
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
	MaxContentScanSize = 1e8
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
	buf := make([]byte, enc.DecodedLen(len(auth)))
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
	Bytes []byte
	// total int64 // Total # of bytes transferred
	Contenttype string
	Passthru    *GSProxyPassthru
}

func (pt *DataPassThru) Write(p []byte) (int, error) {
	n, err := pt.Writer.Write(p)
	pt.Bytes = append(pt.Bytes, p...)
	if err == nil {
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
	passthru := NewGSProxyPassthru()

	hostaddress := strings.Split(r.URL.Host, ":")[0]
	isHostLanAddress := isLanAddress(hostaddress)

	if len(r.URL.String()) > 10000 {
		http.Error(w, "URL too long", http.StatusRequestURITooLong)
		return
	}

	client := r.RemoteAddr
	host, _, err := net.SplitHostPort(client)
	if err == nil {
		client = host
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

			log.Println("User auth status = ", userAuthStatusString, " For user = ", user)
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
		passthru.ProxyActionToLog = ProxyActionBlockedUrl
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedUrl})
		sendBlockMessageBytes(w, r, nil, urlFilterData.FilterResponse, nil)
		return
	}

	fileExt := getFileExtensionFromUrl(r.URL.String())
	fileMime := getMimeByExtension(fileExt)
	contentTypeScan := &GSContentTypeFilterData{Url: r.URL.String(), ContentType: fileMime}
	IProxy.ContentTypeHandler(contentTypeScan)

	log.Println("Url File extension = ", fileExt, " mime ", fileMime)

	if contentTypeScan.FilterResponseAction == ProxyActionBlockedFileType {
		passthru.ProxyActionToLog = ProxyActionBlockedUrl
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedUrl})
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

	// urlHostBytes := []byte(r.URL.Host)
	// shouldMitm, _ := IProxy.RunHandler("mitm", "", &urlHostBytes, passthru)
	shouldMitm := IProxy.DoMitm(r.URL.Host)
	log.Println("Should MITM = ", shouldMitm, " currentAction = "+action, " for ", r.URL.String())

	if isHostLanAddress {
		action = ACTION_NONE
		// modified = true
	}

	if shouldMitm == false {
		action = ACTION_NONE
	}

	// isExceptionUrl, _ := IProxy.RunHandler("except_urls", "", &requestUrlBytes, passthru)
	isExceptionUrl := IProxy.IsExceptionUrl(r.URL.String())
	if isExceptionUrl {
		action = ACTION_NONE
	}

	if action == ACTION_SSL_BUMP {
		HandleSSLBump(r, w, user, authUser, passthru, IProxy)
		return
	}

	if r.Method == "CONNECT" {
		// requestUrlBytes_log := []byte(r.URL.String())
		passthru.ProxyActionToLog = ProxyActionSSLDirect
		// IProxy.RunHandler("log", "", &requestUrlBytes_log, passthru)
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionSSLDirect})
		HandleSSLConnectDirect(r, w, user, passthru)
		return
	}

	if r.Header.Get("Upgrade") == "websocket" {
		HandleWebsocketConnection(r, w)
		return
	}

	if len(r.Header["X-Forwarded-For"]) >= 10 {
		http.Error(w, "Proxy forwarding loop", http.StatusBadRequest)
		log.Printf("Proxy forwarding loop from %s to %v", r.Header.Get("X-Forwarded-For"), r.URL)
		return
	}

	gzipOK := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && !isLanAddress(client)
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

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, ";") {
		t := strings.Split(contentType, ";")
		if len(t) > 0 {
			contentType = t[0]
		}
	}
	log.Println("Content type is = ", contentType, " for ", r.URL.String())
	// contentTypeBytes := []byte(contentType)

	// contentTypeStatusBlocked, _ := IProxy.RunHandler("contenttypeblocked", "", &contentTypeBytes, passthru)
	contentTypeData := GSContentTypeFilterData{Url: r.URL.String(), ContentType: contentType}
	IProxy.ContentTypeHandler(&contentTypeData)

	if contentTypeData.FilterResponseAction == ProxyActionBlockedFileType {
		// requestUrlBytes_log := []byte(r.URL.String())
		passthru.ProxyActionToLog = ProxyActionBlockedFileType
		// IProxy.RunHandler("log", "", &requestUrlBytes_log, passthru)
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: ProxyActionBlockedFileType})
		sendBlockMessageBytes(w, r, nil, BLOCKED_CONTENT_TYPE, &contentType)
		return
	}

	var buf bytes.Buffer
	limitedReader := &io.LimitedReader{R: resp.Body, N: int64(MaxContentScanSize)}
	teeReader := io.TeeReader(limitedReader, &buf)

	localCopyData, err := io.ReadAll(teeReader)

	if err != nil {
		log.Printf("error while reading response body (URL: %s): %s", r.URL, err)
	}

	if limitedReader.N == 0 {
		log.Println("response body too long to filter:", r.URL)
		if gzipOK {
			resp.Header.Set("Content-Encoding", "gzip")
			gzw := gzip.NewWriter(w)
			defer gzw.Close()
		} else if resp.ContentLength > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		}

		destwithcounter := &DataPassThru{
			Writer:      w,
			Contenttype: contentType,
			Passthru:    passthru,
		}

		copyResponseHeader(w, resp)

		_, err := io.Copy(destwithcounter, resp.Body)
		resp.Header.Set("Content-Encoding", "gzip")

		if err != nil {
			log.Printf("error while copying response (URL: %s): %s", r.URL, err)
			errorData := &GSProxyErrorData{Error: err.Error()}
			IProxy.ProxyErrorHandler(errorData)
			sendBlockMessageBytes(w, r, nil, errorData.FilterResponse, nil)
			return
		}
	}

	kind, _ := filetype.Match(localCopyData)
	if kind != filetype.Unknown {
		log.Printf("File type: %s. MIME: %s\n", kind.Extension, kind.MIME.Value)
		contentType = kind.MIME.Value
	}
	responseSentMedia, proxyActionTaken := ScanMedia(localCopyData, contentType, r, w, resp, buf, passthru)
	if responseSentMedia == true {
		passthru.ProxyActionToLog = proxyActionTaken
		// IProxy.RunHandler("log", "", &requestUrlBytes, passthru)
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: proxyActionTaken})
		return
	}

	responseSentText, proxyActionTaken := ScanText(localCopyData, contentType, r, w, resp, buf, passthru)
	if responseSentText == true {
		passthru.ProxyActionToLog = proxyActionTaken
		// IProxy.RunHandler("log", "", &requestUrlBytes, passthru)
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: proxyActionTaken})
		return
	}

	ruleTest := &GatesentryTypes.GSRuleFilterParam{
		Url:         r.URL.String(),
		ContentType: contentType,
		User:        user,
		Action:      ProxyActionFilterNone,
	}
	IProxy.RuleHandler(ruleTest)

	if ruleTest.Action == ProxyActionBlockedRule {
		IProxy.LogHandler(GSLogData{Url: r.URL.String(), User: user, Action: proxyActionTaken})
		sendBlockMessageBytes(w, r, nil, BLOCKED_CONTENT_TEXT, &contentType)
	}

	if gzipOK && len(localCopyData) > 1000 {
		resp.Header.Set("Content-Encoding", "gzip")
		copyResponseHeader(w, resp)
		gzw := gzip.NewWriter(w)
		var dest io.Writer
		dest = gzw
		destwithcounter := &DataPassThru{Writer: dest, Contenttype: contentType, Passthru: passthru}
		destwithcounter.Write(localCopyData)
		gzw.Close()
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(len(localCopyData)))
		copyResponseHeader(w, resp)
		destwithcounter := &DataPassThru{Writer: w, Contenttype: contentType, Passthru: passthru}
		destwithcounter.Write(localCopyData)
	}
}

func sendInsecureBlockBytes(w http.ResponseWriter, r *http.Request, resp *http.Response, content []byte, contentType *string) {
	w.WriteHeader(http.StatusOK)
	// string ends with

	if contentType != nil && isImage(*contentType) {
		reasonForBlockArray := append([]string{"", "Image blocked by Gatesentry", "Reason(s) for blocking", "1. The content type is blocked"})
		emptyImage, _ := createEmptyImage(500, 500, "jpeg", reasonForBlockArray)
		w.Header().Set("Content-Type", "image/jpeg; charset=utf-8")
		w.Write(emptyImage)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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

// copyResponseHeader writes resp's header and status code to w.
func copyResponseHeader(w http.ResponseWriter, resp *http.Response) {
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
