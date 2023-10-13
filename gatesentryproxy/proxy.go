package gatesentryproxy

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
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
	return &p
}

func NewGSHandler(handlerid string, f func(*[]byte, *GSResponder, *GSProxyPassthru)) *GSHandler {
	h := GSHandler{Id: handlerid, Handle: f}
	// h.Handle = f;
	return &h
}

func NewGSProxy() *GSProxy {
	p := GSProxy{}
	IProxy = &p
	IProxy.UsersCache = map[string]GSUserCached{}
	return &p
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

func (p *GSProxy) RunHandler(handlerid string, contentType string, content *[]byte, gpt *GSProxyPassthru) (bool, []byte) {
	rs := GSResponder{
		Changed: false,
		Data:    []byte{},
	}

	if p.Handlers[handlerid] != nil {
		for i := 0; i < len(p.Handlers[handlerid]); i++ {
			p.Handlers[handlerid][i].Handle(content, &rs, gpt)
		}
		if rs.Changed {
			return true, rs.Data
		}
	}
	return false, rs.Data
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
		bs := []byte(strconv.Itoa(n))
		IProxy.RunHandler("contentlength", pt.Contenttype, &bs, pt.Passthru)
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

	prereqmessage := []byte(r.Header.Get("Proxy-Authorization"))
	prerequestblock, _ := IProxy.RunHandler("prerequest", "", &prereqmessage, passthru)
	if prerequestblock {
		if isHostLanAddress {
			log.Println("Host IS LAN address = " + hostaddress)
			// isHostLanAddress = true;
		} else {
			showBlockPage(w, r, nil, prereqmessage)
			return
		}
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

	// log.Println("Auth enabled = " + user)
	// user, pass, ok := r.BasicAuth()
	// log.Println("[GPT] User = " + user + " Pass = " + pass + " Ok = " + strconv.FormatBool(ok))
	// if !ok || user != "user" || pass != "pass" {
	// 	log.Println("Unauthorized access from " + client)
	// 	w.Header().Set("WWW-Authenticate", `Basic realm="Please enter your username and password"`)
	// 	w.WriteHeader(401)
	// 	w.Write([]byte("You are unauthorized to access the application.\n"))
	// 	return
	// }

	authEnabled := true
	t := []byte(r.Header.Get("Proxy-Authorization"))
	authEnabled, _ = IProxy.RunHandler("authenabled", "", &t, passthru)
	user, _, authUser := HandleAuthAndAssignUser(r, passthru, h, authEnabled, client)
	if authEnabled {
		if user == "" || user == "127.0.0.1" {
			w.Header().Set("Proxy-Authenticate", "Basic realm="+"gsrealm")
			http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
			log.Printf("Missing required proxy authentication from %v to %v", r.RemoteAddr, r.URL)
			return
		} else {
			_, userAuthStatus := IProxy.RunHandler("isaccessactive", "", &EMPTY_BYTES, passthru)
			userAuthStatusString := string(userAuthStatus)

			log.Println("User auth status = ", userAuthStatusString, " For user = ", user)
			if userAuthStatusString == "NOT_FOUND" {
				w.Header().Set("Proxy-Authenticate", "Basic realm="+"gsrealm")
				http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
				log.Printf("Missing required proxy authentication from %v to %v", r.RemoteAddr, r.URL)
				return
			}
			if userAuthStatusString != "ACTIVE" && !isHostLanAddress {
				showBlockPage(w, r, nil, EMPTY_BYTES)
				return
			}
		}
	}

	timeblocked, _ := IProxy.RunHandler("timeallowed", "", &EMPTY_BYTES, passthru)
	if timeblocked {
		showBlockPage(w, r, nil, EMPTY_BYTES)
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

	action := ACTION_NONE

	requestUrlBytes := []byte(r.URL.String())
	isBlockedInternet, _ := IProxy.RunHandler("blockinternet", "", &requestUrlBytes, passthru)
	if isBlockedInternet {
		showBlockPage(w, r, nil, BLOCKED_INTERNET_BYTES)
		return
	}

	isBlockedUrl, _ := IProxy.RunHandler("url", "", &requestUrlBytes, passthru)
	if isBlockedUrl {
		showBlockPage(w, r, nil, BLOCKED_URL_BYTES)
		return
	}

	if r.Method == "CONNECT" {
		action = ACTION_SSL_BUMP
	}

	urlHostBytes := []byte(r.URL.Host)
	shouldMitm, _ := IProxy.RunHandler("mitm", "", &urlHostBytes, passthru)

	log.Println("Should MITM = ", shouldMitm, " currentAction = "+action, " for ", r.URL.String())

	if isHostLanAddress {
		action = ACTION_NONE
		// modified = true
	}

	if shouldMitm == false {
		action = ACTION_NONE
	}

	isExceptionUrl, _ := IProxy.RunHandler("except_urls", "", &requestUrlBytes, passthru)
	if isExceptionUrl {
		action = ACTION_NONE
	}

	IProxy.RunHandler("log", "", &requestUrlBytes, passthru)

	switch action {
	case ACTION_SSL_BUMP:
		HandleSSLBump(r, w, user, authUser, passthru)
		return
	case ACTION_BLOCK_REQUEST:
		showBlockPage(w, r, nil, EMPTY_BYTES)
		return
	}

	if r.Method == "CONNECT" {
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
		errorBytes := []byte(err.Error())
		IProxy.RunHandler("proxyerror", "", &errorBytes, passthru)
		showBlockPage(w, r, nil, t)
		return
	}
	defer resp.Body.Close()

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, ";") {
		t := strings.Split(contentType, ";")
		contentType = t[0]
	}
	log.Println("Content type is = ", contentType, " for ", r.URL.String())
	contentTypeBytes := []byte(contentType)
	contentTypeStatusBlocked, _ := IProxy.RunHandler("contenttypeblocked", "", &contentTypeBytes, passthru)

	if contentTypeStatusBlocked {
		showBlockPage(w, r, nil, BLOCKED_CONTENT_TYPE)
		return
	}

	// Create a buffer to hold a copy of the data
	var buf bytes.Buffer
	limitedReader := &io.LimitedReader{R: resp.Body, N: int64(MaxContentScanSize)}
	teeReader := io.TeeReader(limitedReader, &buf)

	// Read the entire response from the TeeReader into a byte slice
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
			showBlockPage(w, r, nil, PROXY_ERROR_UNABLE_TO_COPY_DATA)
			return
		}
	}

	kind, _ := filetype.Match(localCopyData)
	if kind != filetype.Unknown {
		log.Printf("File type: %s. MIME: %s\n", kind.Extension, kind.MIME.Value)
		contentType = kind.MIME.Value
	}

	if ScanMedia(localCopyData, contentType, r, w, resp, buf, passthru) == true {
		return
	}

	if ScanHTML(localCopyData, contentType, r, w, resp, buf, passthru) == true {
		return
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
		// log.Printf("No content encoding for = " + r.URL.String());
		w.Header().Set("Content-Length", strconv.Itoa(len(localCopyData)))
		copyResponseHeader(w, resp)
		destwithcounter := &DataPassThru{Writer: w, Contenttype: contentType, Passthru: passthru}
		destwithcounter.Write(localCopyData)
		// w.Write(content)
	}
}

func showBlockPage(w http.ResponseWriter, r *http.Request, resp *http.Response, content []byte) {
	w.WriteHeader(http.StatusForbidden)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(content)
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
