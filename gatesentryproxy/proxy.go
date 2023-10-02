package gatesentryproxy

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	// "gatesentryalpha"
	// "flag"
	"encoding/base64"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
	// "github.com/bogdanovich/dns_resolver"
	// "strconv"
)

var IProxy *GSProxy
var MaxContentScanSize int64
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
var hopByHop = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

type GSProxyPassthru struct {
	UserData  interface{}
	DontTouch bool
	User      string
}

type GSResponder struct {
	Changed bool
}

type GSHandler struct {
	Id     string
	Handle func(*[]byte, *GSResponder, *GSProxyPassthru)
}

type GSUserCached struct {
	User string
	Pass string
}

type GSProxy struct {
	Handlers   map[string][]*GSHandler
	UsersCache map[string]GSUserCached
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

// func (h *GSHandler) Handle( content string, rs *responder ) string {

// }

func NewGSProxy() *GSProxy {
	p := GSProxy{}
	IProxy = &p
	IProxy.UsersCache = map[string]GSUserCached{}

	// p.Handlers = map[string]*GSProxy{};
	// p.Handlers = map[string][]*GSHandler;
	return &p
}

func (p *GSProxy) RegisterHandler(id string, f func(*[]byte, *GSResponder, *GSProxyPassthru)) {
	// fmt.Println(p)
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
	// fmt.Println( (p.Handlers ) )

	// if ( p.Handlers[id] == nil ) {
	// 	p.Handlers[id] = []*GSHandler{}
	// 	p.Handlers[id] = append(p.Handlers[id], h)
	// }else{
	// 	p.Handlers[id] = append(p.Handlers[id], h)
	// }
	// p.Handlers[id] = h;
	// p.Handlers[id]=[]
}

func (p *GSProxy) RunHandler(handlerid string, contentType string, content *[]byte, gpt *GSProxyPassthru) bool {
	// log.Printf("Running Handler for " + handlerid )
	// fmt.Println( (p.Handlers ) )
	rs := GSResponder{false}
	// fmt.Println( len(p.Handlers[handlerid]) )
	// log.Printf("Handler = " + p.Handlers[handlerid][0].Id);
	if p.Handlers[handlerid] != nil {
		for i := 0; i < len(p.Handlers[handlerid]); i++ {
			p.Handlers[handlerid][i].Handle(content, &rs, gpt)
		}
		if rs.Changed {
			return true
		}
	}
	return false
	// for i := 0; i < len(p.Handlers); i++ {
	// 	p.Handlers[i].
	// }

}

func InitProxy() {
	MaxContentScanSize = 1e8
}

// lanAddress returns whether addr is in one of the LAN address ranges.
func lanAddress(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch ip4[0] {
		case 10, 127:
			return true
		case 172:
			return ip4[1]&0xf0 == 16
		case 192:
			return ip4[1] == 168
		}
		return false
	}

	// IPv6
	switch {
	case ip[0]&0xfe == 0xfc:
		return true
	case ip[0] == 0xfe && (ip[1]&0xfc) == 0x80:
		return true
	case ip.Equal(ip6Loopback):
		return true
	}

	return false
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

func ProxyCredentials(r *http.Request) (user, pass string, ok bool) {
	auth := r.Header.Get("Proxy-Authorization")

	if val, okP := IProxy.UsersCache[auth]; okP {
		return val.User, val.Pass, true
	}

	if auth == "" || !strings.HasPrefix(auth, "Basic ") {
		return "", "", false
	}
	// sTORE CREDS IN CACHE HERE
	nuser, npass, nok := decodeBase64Credentials(strings.TrimPrefix(auth, "Basic "))
	gsu := GSUserCached{User: nuser, Pass: npass}
	IProxy.UsersCache[auth] = gsu
	return nuser, npass, nok
}

type DataPassThru struct {
	io.Writer
	// total int64 // Total # of bytes transferred
	Contenttype string
	Passthru    *GSProxyPassthru
}

func (pt *DataPassThru) Write(p []byte) (int, error) {
	n, err := pt.Writer.Write(p)
	// pt.total += int64(n)
	// log.Println(err.Error())
	if err == nil {
		// fmt.Println("Read", n, "bytes for a total of", pt.total)
		bs := []byte(strconv.Itoa(n))
		// log.Println(string(bs));
		IProxy.RunHandler("contentlength", pt.Contenttype, &bs, pt.Passthru)
	}

	return n, err
}

// func checkIfaddressIsIp(){
// 	ip := net.ParseIP(addr)
// 	if ( ip == nil ){
// 		return false;
// 	}
// 	return true;
// }

func (h ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	passthru := NewGSProxyPassthru()
	// log.Printf("-Received a request-");
	// log.Printf("METHOD = "+r.Method)
	// fmt.Println("URL = "+r.Host+r.URL.String())
	log.Printf("URL = " + r.Host + r.URL.String())

	hostaddress := strings.Split(r.URL.Host, ":")[0]
	isHostLanAddress := lanAddress(hostaddress)

	if len(r.URL.String()) > 10000 {
		http.Error(w, "URL too long", http.StatusRequestURITooLong)
		return
	}

	// log.Printf("Making a client")
	client := r.RemoteAddr
	host, _, err := net.SplitHostPort(client)
	if err == nil {
		client = host
	}

	prereqmessage := []byte(r.Header.Get("Proxy-Authorization"))
	prerequestblock := IProxy.RunHandler("prerequest", "", &prereqmessage, passthru)
	// fmt.Println(prerequestblock)
	if prerequestblock {

		// _=hostaddress

		if isHostLanAddress {
			log.Println("Host IS LAN address = " + hostaddress)
			// isHostLanAddress = true;
		} else {
			showBlockPage(w, r, nil, prereqmessage)
			return
		}

		// }
	}

	// Reconstruct the URL if it is incomplete (i.e. on a transparent proxy).
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
			// Delay a while since some programs really hammer us with this kind of request.
			time.Sleep(time.Second)
			http.Error(w, "No host in request URL, and no Host header.", http.StatusBadRequest)
			return
		}
	}

	authUser := ""

	user := client
	pass := ""

	if authUser != "" {
		user = authUser
	}

	authEnabled := true
	t := []byte(r.Header.Get("Proxy-Authorization"))
	authEnabled = IProxy.RunHandler("authenabled", "", &t, passthru)

	// log.Println("Auth status = " )
	// fmt.Println( authEnabled)
	if authEnabled {
		ok := false
		user, pass, ok = ProxyCredentials(r)
		if ok {
			// Verify Credentials here
			authUser = user
			temp := []byte(r.Header.Get("Proxy-Authorization"))
			isauth := IProxy.RunHandler("isauthuser", "", &temp, passthru)
			if !isauth {
				user = ""
			}
			log.Println("Unable to verify user")
			_ = pass
		}

		if h.user != "" {
			authUser = h.user
			user = h.user
		}
	}

	/**
	* Check if user has valid access
	 */

	passthru.User = user

	if authEnabled {
		if user == "" {
			// case "require-auth":
			w.Header().Set("Proxy-Authenticate", "Basic realm="+"gsrealm")
			http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
			log.Printf("Missing required proxy authentication from %v to %v", r.RemoteAddr, r.URL)
			return
		}
	}

	temp := []byte("")
	if authEnabled {
		if user != "" {
			temp = []byte("")
			isuseractive := IProxy.RunHandler("isaccessactive", "", &temp, passthru)
			if isHostLanAddress {
				// go forward
			} else if !isuseractive {
				showBlockPage(w, r, nil, temp)
				return
			}
		}
	}

	temp = []byte("")
	timeblocked := IProxy.RunHandler("timeallowed", "", &temp, passthru)
	if timeblocked {
		showBlockPage(w, r, nil, temp)
		return
	}

	_ = user
	if r.Method == "CONNECT" {
		// log.Printf("R")
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

	action := ""

	if r.Method == "CONNECT" {
		// If the result is unclear, go ahead and start to bump the connection.
		// The ACLs will be checked one more time anyway.
		action = "ssl-bump"
	}

	temp = []byte(r.URL.String())
	modified := IProxy.RunHandler("blockinternet", "", &temp, passthru)
	if modified {
		action = "block"
	} else {
		modified = IProxy.RunHandler("url", "", &temp, passthru)
		if modified {
			action = "block"
		} else {
			temp = []byte(r.URL.Host)
			modified = IProxy.RunHandler("mitm", "", &temp, passthru)

			if isHostLanAddress {
				modified = true
			}
			// hostaddress := strings.Split(r.URL.Host, ":")[0]
			// _=hostaddress
			// if ( !lanAddress(hostaddress) ){

			// }
			if modified {
				action = ""
				temp = []byte(r.URL.String())
				IProxy.RunHandler("log", "", &temp, passthru)
			} else {

				temp = []byte(r.URL.String())
				modified = IProxy.RunHandler("except_urls", "", &temp, passthru)
				if modified {
					action = ""
				}
				temp = []byte(r.URL.String())
				IProxy.RunHandler("log", "", &temp, passthru)
			}
		}
	}

	// if ( (action == "" || action == "ssl-bump") && user == "" ){
	// 	action = "require-auth";
	// }

	switch action {

	case "ssl-bump":
		conn, err := newHijackedConn(w)
		if err != nil {
			log.Println("Error hijacking connection for CONNECT request to %s: %v", r.URL.Host, err)
			return
		}
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		// conf = nil // Allow it to be garbage-collected, since we won't use it any more.
		SSLBump(conn, r.URL.Host, user, authUser, r, passthru)
		return
	case "block":
		showBlockPage(w, r, nil, temp)
		return
		break
	}

	if r.Method == "CONNECT" {
		conn, err := newHijackedConn(w)
		if err != nil {
			log.Println("Error hijacking connection for CONNECT request to %s: %v", r.URL.Host, err)
			return
		}
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		// logAccess(r, nil, 0, false, user, tally, scores, thisRule, "", ignored)
		// conf = nil // Allow it to be garbage-collected, since we won't use it any more.
		log.Printf("Running a CONNECTDIRECT")
		// int64 to string
		// IF HOST CONTAINS scontent and fbcdn.net
		// THEN BLOCK
		var uploaded, downloaded, wasBlocked = ConnectDirect(conn, r.URL.Host, nil, passthru)

		if wasBlocked {
			showBlockPage(w, r, nil, []byte("Blocked"))
			return
		}

		// uploadedStr := strconv.FormatInt(uploaded, 10)
		// downloadedStr := strconv.FormatInt(downloaded, 10)

		uploadedStr := GetHumanDataSize(uploaded)
		downloadedStr := GetHumanDataSize(downloaded)
		fmt.Println("[Traffic] Host = " + r.URL.Host + " Uploaded = " + uploadedStr + " Downloaded = " + downloadedStr)

		return
	}

	if r.Header.Get("Upgrade") == "websocket" {
		// logAccess(r, nil, 0, false, user, tally, scores, thisRule, "", ignored)
		http.Error(w, "Web sockets currently not supported", http.StatusBadRequest)
		// h.makeWebsocketConnection(w, r)
		return
	}

	if len(r.Header["X-Forwarded-For"]) >= 10 {
		http.Error(w, "Proxy forwarding loop", http.StatusBadRequest)
		log.Printf("Proxy forwarding loop from %s to %v", r.Header.Get("X-Forwarded-For"), r.URL)
		return
	}

	gzipOK := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && !lanAddress(client)
	r.Header.Del("Accept-Encoding")
	_ = gzipOK
	// conf.changeQuery(r.URL)

	var rt http.RoundTripper
	if h.rt == nil {
		rt = httpTransport
	} else {
		rt = h.rt
	}

	// Some HTTP/2 servers don't like having a body on a GET request, even if
	// it is empty.
	if r.ContentLength == 0 {
		r.Body.Close()
		r.Body = nil
	}

	removeHopByHopHeaders(r.Header)
	resp, err := rt.RoundTrip(r)

	if err != nil {
		// http.Error(w, err.Error(), http.StatusServiceUnavailable)
		log.Printf("error fetching %s: %s", r.URL, err)
		t := []byte(err.Error())
		IProxy.RunHandler("proxyerror", "", &t, passthru)
		showBlockPage(w, r, nil, t)
		return
		// logAccess(r, nil, 0, false, user, tally, scores, thisRule, "", ignored)
		// return
	}
	defer resp.Body.Close()

	action = ""
	// action = "allow";
	temp = []byte(r.URL.String())
	IProxy.RunHandler("log", "", &temp, passthru)

	contentType := resp.Header.Get("Content-Type")
	// fmt.Println(resp.Header)
	contentTyperesponse := []byte(contentType)
	contentTypeStatusBlocked := IProxy.RunHandler("contenttypeblocked", "", &contentTyperesponse, passthru)
	if contentTypeStatusBlocked {
		action = "block"
	}
	switch action {
	case "block":
		showBlockPage(w, r, nil, contentTyperesponse)
		return
		break
	case "allow":
		var dest io.Writer = w
		shouldGZIP := false
		if gzipOK && (resp.ContentLength == -1 || resp.ContentLength > 1024) {
			ct, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
			if err == nil {
				switch ct {
				case "application/javascript", "application/x-javascript", "application/json":
					shouldGZIP = true
				default:
					shouldGZIP = strings.HasPrefix(ct, "text/")
				}
			}
		}
		if shouldGZIP {
			resp.Header.Set("Content-Encoding", "gzip")
			gzw := gzip.NewWriter(w)
			defer gzw.Close()
			dest = gzw
		} else if resp.ContentLength > 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		}
		// fmt.Println(resp.Body)
		destwithcounter := &DataPassThru{
			Writer:      dest,
			Contenttype: contentType,
			Passthru:    passthru,
		}

		// io.Copy(destwithcounter, resp.Body)
		// n, err := io.Copy(dest, resp.Body)
		n, err := io.Copy(destwithcounter, resp.Body)
		if err != nil {
			log.Printf("error while copying response (URL: %s): %s", r.URL, err)
		}
		copyResponseHeader(w, resp)
		_ = n
		// logAccess(r, resp, int(n), false, user, tally, scores, thisRule, "", ignored)
		return
		break

	}
	cContentType := strings.ToLower(contentType)
	if strings.Contains(cContentType, ";") {
		t := strings.Split(cContentType, ";")
		cContentType = t[0]
	}
	log.Println("Content type is = ", cContentType)
	if cContentType == "video/webm" ||
		cContentType == "video/mp4" ||
		cContentType == "video/x-ms-wmv" ||
		cContentType == "audio/mpeg" ||
		cContentType == "video/x-msvideo" ||
		cContentType == "video/jpeg" ||
		cContentType == "image/png" ||
		cContentType == "image/gif" ||
		cContentType == "image/jpeg" ||
		cContentType == "image/webp" ||
		cContentType == "image/svg+xml" ||
		cContentType == "image/x-icon" ||
		cContentType == "text/css" ||
		cContentType == "font/woff2" ||
		cContentType == "application/x-font-woff" ||
		cContentType == "application/zip" ||
		cContentType == "application/x-msdownload" ||
		cContentType == "application/octet-stream" ||
		cContentType == "application/x-javascript" ||
		cContentType == "application/javascript" {
		log.Println("Not filtering, sending directly to client")
		// var dest io.Writer = w
		destwithcounter := &DataPassThru{
			Writer:      w,
			Contenttype: contentType,
			Passthru:    passthru,
		}
		copyResponseHeader(w, resp)
		io.Copy(destwithcounter, resp.Body)
		// io.Copy(dest, resp.Body)
		// destwithcounter := &DataPassThru{
		// 	Writer: w,
		// 	Contenttype: contentType,
		// 	Passthru: passthru,
		// }
		// destwithcounter.Write(content)
		return
	}
	lr := &io.LimitedReader{
		R: resp.Body,
		N: int64(MaxContentScanSize),
	}
	content, err := ioutil.ReadAll(lr)

	if err != nil {
		log.Printf("error while reading response body (URL: %s): %s", r.URL, err)
	}
	log.Printf("Reading Body")
	// contentSize := len(content);

	// log.Println("Content size is = ", contentSize)
	// log.Println(string(content))

	// if ( contentSize > 0 ){

	// }
	if lr.N == 0 {
		log.Println("response body too long to filter:", r.URL)
		var dest io.Writer = w
		if gzipOK {
			log.Println("Response encoding set to gzip")
			resp.Header.Set("Content-Encoding", "gzip")
			gzw := gzip.NewWriter(w)
			// copyResponseHeader(w, resp)
			// gzw.Write([]byte(content) )
			defer gzw.Close()
			// return;
			dest = gzw
		} else if resp.ContentLength > 0 {
			log.Println("Content length > 0")
			w.Header().Set("Content-Length", strconv.FormatInt(resp.ContentLength, 10))
		}

		// io.Copy(w, resp.Body)
		// return;
		destwithcounter := &DataPassThru{
			Writer:      w,
			Contenttype: contentType,
			Passthru:    passthru,
		}

		// _=destwithcounter
		// w.Header().Set("Content-Type", "text/html")
		// copyResponseHeader(w, resp)
		// w.Write([]byte("Hello World") )
		// return;
		// w.Header().Set("Content-Encoding", "gzip")
		copyResponseHeader(w, resp)
		// destwithcounter.Write(content)
		// w.Header().Set("Content-Encoding", "gzip")

		// _, err := io.Copy(dest, resp.Body)
		_ = dest
		_, err := io.Copy(destwithcounter, resp.Body)
		resp.Header.Set("Content-Encoding", "gzip")

		if err != nil {
			log.Printf("error while copying response (URL: %s): %s", r.URL, err)
		}
		// logAccess(r, resp, int(n)+len(content), false, user, tally, scores, ACLActionRule{Action: "allow", Needed: []string{"too-long-to-filter"}}, "", ignored)
		// return
	}

	modified = false
	pageTitle := ""

	_, cs, _ := charset.DetermineEncoding(content, contentType)
	var doc *html.Node
	_ = modified
	_ = pageTitle
	_ = cs
	_ = doc

	// go func(){

	// }

	// log.Printf("Content type is = " + contentType + " length is = "+ strconv.Itoa(len(content)));
	if strings.Contains(contentType, "html") || len(contentType) == 0 {
		log.Printf("Content type is html")
		modified = IProxy.RunHandler("content", contentType, (&content), passthru)
		mms := "false"
		if modified {
			mms = "true"
		}
		_ = mms
		// log.Printf("---The request was " + mms + "---")
		// content = []byte("HI!")
		// if conf.LogTitle {
		// doc, err = parseHTML(content, cs)
		// 	if err != nil {
		// 		log.Printf("Error parsing HTML from %s: %s", r.URL, err)
		// 	} else {
		// 		t := titleSelector.MatchFirst(doc)
		// 		if t != nil {
		// 			if titleText := t.FirstChild; titleText != nil && titleText.Type == html.TextNode {
		// 				pageTitle = titleText.Data
		// 			}
		// 		}
		// 	}
		// }

		// modified = conf.pruneContent(r.URL, &content, cs, acls, &doc)
		if modified {
			cs = "utf-8"
		}
		if modified {
			resp.Header.Set("Content-Type", "text/html; charset=utf-8")
		}
	}

	if gzipOK && len(content) > 1000 {
		// log.Println("ENCODING IS GZIP");
		resp.Header.Set("Content-Encoding", "gzip")
		copyResponseHeader(w, resp)
		gzw := gzip.NewWriter(w)
		var dest io.Writer
		dest = gzw
		destwithcounter := &DataPassThru{Writer: dest, Contenttype: contentType, Passthru: passthru}
		destwithcounter.Write(content)
		gzw.Close()
	} else {
		// log.Printf("No content encoding for = " + r.URL.String());
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		copyResponseHeader(w, resp)
		destwithcounter := &DataPassThru{Writer: w, Contenttype: contentType, Passthru: passthru}
		destwithcounter.Write(content)
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
	toRemove := hopByHop
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

func newHijackedConn(w http.ResponseWriter) (*hijackedConn, error) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("connection doesn't support hijacking")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	err = bufrw.Flush()
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &hijackedConn{
		Conn:   conn,
		Reader: bufrw.Reader,
	}, nil
}
