package gatesentryproxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	// "gatesentryalpha"
	// "flag"
	"encoding/base64"
	"encoding/json"

	"github.com/h2non/filetype"
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
	prerequestblock := IProxy.RunHandler("prerequest", "", &prereqmessage, passthru)
	// fmt.Println(prerequestblock)
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

	authEnabled := true
	t := []byte(r.Header.Get("Proxy-Authorization"))
	authEnabled = IProxy.RunHandler("authenabled", "", &t, passthru)

	user, _, authUser := HandleAuthAndAssignUser(r, passthru, h, authEnabled, client)

	if authEnabled {
		if user == "" {
			w.Header().Set("Proxy-Authenticate", "Basic realm="+"gsrealm")
			http.Error(w, "Proxy authentication required", http.StatusProxyAuthRequired)
			log.Printf("Missing required proxy authentication from %v to %v", r.RemoteAddr, r.URL)
			return
		} else {
			isuseractive := IProxy.RunHandler("isaccessactive", "", &EMPTY_BYTES, passthru)
			if !isuseractive && !isHostLanAddress {
				showBlockPage(w, r, nil, EMPTY_BYTES)
				return
			}
		}
	}

	timeblocked := IProxy.RunHandler("timeallowed", "", &EMPTY_BYTES, passthru)
	if timeblocked {
		showBlockPage(w, r, nil, EMPTY_BYTES)
		return
	}

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

	action := ACTION_NONE

	// if r.Method == "CONNECT" {
	// 	action = ACTION_SSL_BUMP
	// }

	requestUrlBytes := []byte(r.URL.String())
	internetBlocked := IProxy.RunHandler("blockinternet", "", &requestUrlBytes, passthru)

	if internetBlocked {
		showBlockPage(w, r, nil, BLOCKED_INTERNET_BYTES)
		return
	}

	urlBlocked := IProxy.RunHandler("url", "", &requestUrlBytes, passthru)
	if urlBlocked {
		showBlockPage(w, r, nil, BLOCKED_URL_BYTES)
		return
	}

	if r.Method == "CONNECT" {
		// If the result is unclear, go ahead and start to bump the connection.
		// The ACLs will be checked one more time anyway.
		action = ACTION_SSL_BUMP
	}

	// urlBytes := []byte(r.URL.String())
	blockedInternet := IProxy.RunHandler("blockinternet", "", &requestUrlBytes, passthru)
	if blockedInternet {
		showBlockPage(w, r, nil, BLOCKED_INTERNET_BYTES)
		return
	}
	blockedUrl := IProxy.RunHandler("url", "", &requestUrlBytes, passthru)
	if blockedUrl {
		showBlockPage(w, r, nil, BLOCKED_URL_BYTES)
		return
	}

	urlHostBytes := []byte(r.URL.Host)

	IProxy.RunHandler("mitm", "", &urlHostBytes, passthru)

	if isHostLanAddress {
		action = ACTION_NONE
		// modified = true
	}

	// IProxy.RunHandler("log", "", &requestUrlBytes, passthru)

	isExceptionUrl := IProxy.RunHandler("except_urls", "", &requestUrlBytes, passthru)

	if isExceptionUrl {
		action = ACTION_NONE
	}

	// requestHostBytes := []byte(r.URL.Host)
	// shouldMitm := IProxy.RunHandler("mitm", "", &requestHostBytes, passthru)

	// if shouldMitm && !isHostLanAddress {
	// 	action = ACTION_SSL_BUMP
	// }
	// if isHostLanAddress {
	// 	modified = true
	// }
	// if modified {
	// 	action = ""
	// 	IProxy.RunHandler("log", "", &requestUrlBytes, passthru)
	// } else {
	// 	modified = IProxy.RunHandler("except_urls", "", &requestUrlBytes, passthru)
	// 	if modified {
	// 		action = ""
	// 	}
	// }
	IProxy.RunHandler("log", "", &requestUrlBytes, passthru)
	// log.Print("For url = "+r.URL.Host+"Action is = ", action, " shouldmitm = ", shouldMitm, " isHostLanAddress = ", isHostLanAddress)
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
		log.Printf("error fetching %s: %s", r.URL, err)
		errorBytes := []byte(err.Error())
		IProxy.RunHandler("proxyerror", "", &errorBytes, passthru)
		showBlockPage(w, r, nil, t)
		return
	}
	defer resp.Body.Close()

	action = ACTION_NONE
	// action = "allow";

	contentType := resp.Header.Get("Content-Type")
	log.Println("Content type is = ", contentType, " for ", r.URL.String())
	contentTyperesponse := []byte(contentType)
	contentTypeStatusBlocked := IProxy.RunHandler("contenttypeblocked", "", &contentTyperesponse, passthru)

	if contentTypeStatusBlocked {
		action = ACTION_BLOCK_REQUEST
	}

	if action == ACTION_BLOCK_REQUEST {
		showBlockPage(w, r, nil, contentTyperesponse)
		return
	}

	cContentType := strings.ToLower(contentType)
	if strings.Contains(cContentType, ";") {
		t := strings.Split(cContentType, ";")
		cContentType = t[0]
	}

	// Create a buffer to hold a copy of the data
	var buf bytes.Buffer

	// Use io.TeeReader to read from resp.Body and simultaneously write to buf
	tee := io.TeeReader(resp.Body, &buf)

	// Read the entire response from the TeeReader into a byte slice
	localCopyData, err := io.ReadAll(tee)

	log.Println("LocalCopyData size is = ", len(localCopyData))
	if err != nil {
		// Handle error
		return
	}

	kind, _ := filetype.Match(localCopyData)
	if kind != filetype.Unknown {
		log.Printf("File type: %s. MIME: %s\n", kind.Extension, kind.MIME.Value)
		contentType = kind.MIME.Value
	}

	if cContentType == "video/webm" ||
		cContentType == "video/mp4" ||
		cContentType == "video/x-ms-wmv" ||
		cContentType == "audio/mpeg" ||
		cContentType == "video/x-msvideo" ||
		cContentType == "video/jpeg" ||
		cContentType == "image/png" ||
		cContentType == "image/avif" ||
		cContentType == "image/gif" ||
		cContentType == "image/jpeg" ||
		cContentType == "image/jpg" ||
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

		var scanThis = ContentScannerInput{
			Content:     localCopyData,
			ContentType: contentType,
			Url:         r.URL.String(),
		}

		// convert above struct to bytes
		scanThisBytes, err := json.Marshal(scanThis)
		if err != nil {
			log.Println(err)
		}

		isBlocked := IProxy.RunHandler("contentscanner", "", &scanThisBytes, passthru)
		if isBlocked {
			emptyImage, _ := createEmptyImage(50, 50, "jpeg")
			emptyReader := bytes.NewReader(emptyImage)
			io.Copy(destwithcounter, emptyReader)
		} else {
			io.Copy(destwithcounter, &buf)
		}
		log.Println("IO Copy done for url = ", r.URL.String())
		return
	}
	lr := &io.LimitedReader{
		R: bytes.NewReader(localCopyData),
		N: int64(MaxContentScanSize),
	}
	content, err := io.ReadAll(lr)

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

	// modified = false
	pageTitle := ""

	_, cs, _ := charset.DetermineEncoding(content, contentType)
	var doc *html.Node
	// _ = modified
	_ = pageTitle
	_ = cs
	_ = doc

	// go func(){

	// }

	// log.Printf("Content type is = " + contentType + " length is = "+ strconv.Itoa(len(content)));
	if strings.Contains(contentType, "html") || len(contentType) == 0 {
		log.Printf("Content type is html")
		modified := IProxy.RunHandler("content", contentType, (&content), passthru)
		mms := "false"
		if modified {
			mms = "true"
		}
		_ = mms

		// find </html> and insert script before it in the content variable
		content = []byte(strings.Replace(string(content), "</html>", `<script>
		console.log('Gatesentry filtered');
		!function(n,t){"object"==typeof exports&&"undefined"!=typeof module?module.exports=t():"function"==typeof define&&define.amd?define(t):(n="undefined"!=typeof globalThis?globalThis:n||self).LazyLoad=t()}(this,(function(){"use strict";function n(){return n=Object.assign||function(n){for(var t=1;t<arguments.length;t++){var e=arguments[t];for(var i in e)Object.prototype.hasOwnProperty.call(e,i)&&(n[i]=e[i])}return n},n.apply(this,arguments)}var t="undefined"!=typeof window,e=t&&!("onscroll"in window)||"undefined"!=typeof navigator&&/(gle|ing|ro)bot|crawl|spider/i.test(navigator.userAgent),i=t&&"IntersectionObserver"in window,o=t&&"classList"in document.createElement("p"),a=t&&window.devicePixelRatio>1,r={elements_selector:".lazy",container:e||t?document:null,threshold:300,thresholds:null,data_src:"src",data_srcset:"srcset",data_sizes:"sizes",data_bg:"bg",data_bg_hidpi:"bg-hidpi",data_bg_multi:"bg-multi",data_bg_multi_hidpi:"bg-multi-hidpi",data_bg_set:"bg-set",data_poster:"poster",class_applied:"applied",class_loading:"loading",class_loaded:"loaded",class_error:"error",class_entered:"entered",class_exited:"exited",unobserve_completed:!0,unobserve_entered:!1,cancel_on_exit:!0,callback_enter:null,callback_exit:null,callback_applied:null,callback_loading:null,callback_loaded:null,callback_error:null,callback_finish:null,callback_cancel:null,use_native:!1,restore_on_error:!1},c=function(t){return n({},r,t)},l=function(n,t){var e,i="LazyLoad::Initialized",o=new n(t);try{e=new CustomEvent(i,{detail:{instance:o}})}catch(n){(e=document.createEvent("CustomEvent")).initCustomEvent(i,!1,!1,{instance:o})}window.dispatchEvent(e)},u="src",s="srcset",d="sizes",f="poster",_="llOriginalAttrs",g="data",v="loading",b="loaded",m="applied",p="error",h="native",E="data-",I="ll-status",y=function(n,t){return n.getAttribute(E+t)},k=function(n){return y(n,I)},w=function(n,t){return function(n,t,e){var i="data-ll-status";null!==e?n.setAttribute(i,e):n.removeAttribute(i)}(n,0,t)},A=function(n){return w(n,null)},L=function(n){return null===k(n)},O=function(n){return k(n)===h},x=[v,b,m,p],C=function(n,t,e,i){n&&(void 0===i?void 0===e?n(t):n(t,e):n(t,e,i))},N=function(n,t){o?n.classList.add(t):n.className+=(n.className?" ":"")+t},M=function(n,t){o?n.classList.remove(t):n.className=n.className.replace(new RegExp("(^|\\s+)"+t+"(\\s+|$)")," ").replace(/^\s+/,"").replace(/\s+$/,"")},z=function(n){return n.llTempImage},T=function(n,t){if(t){var e=t._observer;e&&e.unobserve(n)}},R=function(n,t){n&&(n.loadingCount+=t)},G=function(n,t){n&&(n.toLoadCount=t)},j=function(n){for(var t,e=[],i=0;t=n.children[i];i+=1)"SOURCE"===t.tagName&&e.push(t);return e},D=function(n,t){var e=n.parentNode;e&&"PICTURE"===e.tagName&&j(e).forEach(t)},H=function(n,t){j(n).forEach(t)},V=[u],F=[u,f],B=[u,s,d],J=[g],P=function(n){return!!n[_]},S=function(n){return n[_]},U=function(n){return delete n[_]},$=function(n,t){if(!P(n)){var e={};t.forEach((function(t){e[t]=n.getAttribute(t)})),n[_]=e}},q=function(n,t){if(P(n)){var e=S(n);t.forEach((function(t){!function(n,t,e){e?n.setAttribute(t,e):n.removeAttribute(t)}(n,t,e[t])}))}},K=function(n,t,e){N(n,t.class_applied),w(n,m),e&&(t.unobserve_completed&&T(n,t),C(t.callback_applied,n,e))},Q=function(n,t,e){N(n,t.class_loading),w(n,v),e&&(R(e,1),C(t.callback_loading,n,e))},W=function(n,t,e){e&&n.setAttribute(t,e)},X=function(n,t){W(n,d,y(n,t.data_sizes)),W(n,s,y(n,t.data_srcset)),W(n,u,y(n,t.data_src))},Y={IMG:function(n,t){D(n,(function(n){$(n,B),X(n,t)})),$(n,B),X(n,t)},IFRAME:function(n,t){$(n,V),W(n,u,y(n,t.data_src))},VIDEO:function(n,t){H(n,(function(n){$(n,V),W(n,u,y(n,t.data_src))})),$(n,F),W(n,f,y(n,t.data_poster)),W(n,u,y(n,t.data_src)),n.load()},OBJECT:function(n,t){$(n,J),W(n,g,y(n,t.data_src))}},Z=["IMG","IFRAME","VIDEO","OBJECT"],nn=function(n,t){!t||function(n){return n.loadingCount>0}(t)||function(n){return n.toLoadCount>0}(t)||C(n.callback_finish,t)},tn=function(n,t,e){n.addEventListener(t,e),n.llEvLisnrs[t]=e},en=function(n,t,e){n.removeEventListener(t,e)},on=function(n){return!!n.llEvLisnrs},an=function(n){if(on(n)){var t=n.llEvLisnrs;for(var e in t){var i=t[e];en(n,e,i)}delete n.llEvLisnrs}},rn=function(n,t,e){!function(n){delete n.llTempImage}(n),R(e,-1),function(n){n&&(n.toLoadCount-=1)}(e),M(n,t.class_loading),t.unobserve_completed&&T(n,e)},cn=function(n,t,e){var i=z(n)||n;on(i)||function(n,t,e){on(n)||(n.llEvLisnrs={});var i="VIDEO"===n.tagName?"loadeddata":"load";tn(n,i,t),tn(n,"error",e)}(i,(function(o){!function(n,t,e,i){var o=O(t);rn(t,e,i),N(t,e.class_loaded),w(t,b),C(e.callback_loaded,t,i),o||nn(e,i)}(0,n,t,e),an(i)}),(function(o){!function(n,t,e,i){var o=O(t);rn(t,e,i),N(t,e.class_error),w(t,p),C(e.callback_error,t,i),e.restore_on_error&&q(t,B),o||nn(e,i)}(0,n,t,e),an(i)}))},ln=function(n,t,e){!function(n){return Z.indexOf(n.tagName)>-1}(n)?function(n,t,e){!function(n){n.llTempImage=document.createElement("IMG")}(n),cn(n,t,e),function(n){P(n)||(n[_]={backgroundImage:n.style.backgroundImage})}(n),function(n,t,e){var i=y(n,t.data_bg),o=y(n,t.data_bg_hidpi),r=a&&o?o:i;r&&(n.style.backgroundImage='url("'.concat(r,'")'),z(n).setAttribute(u,r),Q(n,t,e))}(n,t,e),function(n,t,e){var i=y(n,t.data_bg_multi),o=y(n,t.data_bg_multi_hidpi),r=a&&o?o:i;r&&(n.style.backgroundImage=r,K(n,t,e))}(n,t,e),function(n,t,e){var i=y(n,t.data_bg_set);if(i){var o=i.split("|"),a=o.map((function(n){return"image-set(".concat(n,")")}));n.style.backgroundImage=a.join(),""===n.style.backgroundImage&&(a=o.map((function(n){return"-webkit-image-set(".concat(n,")")})),n.style.backgroundImage=a.join()),K(n,t,e)}}(n,t,e)}(n,t,e):function(n,t,e){cn(n,t,e),function(n,t,e){var i=Y[n.tagName];i&&(i(n,t),Q(n,t,e))}(n,t,e)}(n,t,e)},un=function(n){n.removeAttribute(u),n.removeAttribute(s),n.removeAttribute(d)},sn=function(n){D(n,(function(n){q(n,B)})),q(n,B)},dn={IMG:sn,IFRAME:function(n){q(n,V)},VIDEO:function(n){H(n,(function(n){q(n,V)})),q(n,F),n.load()},OBJECT:function(n){q(n,J)}},fn=function(n,t){(function(n){var t=dn[n.tagName];t?t(n):function(n){if(P(n)){var t=S(n);n.style.backgroundImage=t.backgroundImage}}(n)})(n),function(n,t){L(n)||O(n)||(M(n,t.class_entered),M(n,t.class_exited),M(n,t.class_applied),M(n,t.class_loading),M(n,t.class_loaded),M(n,t.class_error))}(n,t),A(n),U(n)},_n=["IMG","IFRAME","VIDEO"],gn=function(n){return n.use_native&&"loading"in HTMLImageElement.prototype},vn=function(n,t,e){n.forEach((function(n){return function(n){return n.isIntersecting||n.intersectionRatio>0}(n)?function(n,t,e,i){var o=function(n){return x.indexOf(k(n))>=0}(n);w(n,"entered"),N(n,e.class_entered),M(n,e.class_exited),function(n,t,e){t.unobserve_entered&&T(n,e)}(n,e,i),C(e.callback_enter,n,t,i),o||ln(n,e,i)}(n.target,n,t,e):function(n,t,e,i){L(n)||(N(n,e.class_exited),function(n,t,e,i){e.cancel_on_exit&&function(n){return k(n)===v}(n)&&"IMG"===n.tagName&&(an(n),function(n){D(n,(function(n){un(n)})),un(n)}(n),sn(n),M(n,e.class_loading),R(i,-1),A(n),C(e.callback_cancel,n,t,i))}(n,t,e,i),C(e.callback_exit,n,t,i))}(n.target,n,t,e)}))},bn=function(n){return Array.prototype.slice.call(n)},mn=function(n){return n.container.querySelectorAll(n.elements_selector)},pn=function(n){return function(n){return k(n)===p}(n)},hn=function(n,t){return function(n){return bn(n).filter(L)}(n||mn(t))},En=function(n,e){var o=c(n);this._settings=o,this.loadingCount=0,function(n,t){i&&!gn(n)&&(t._observer=new IntersectionObserver((function(e){vn(e,n,t)}),function(n){return{root:n.container===document?null:n.container,rootMargin:n.thresholds||n.threshold+"px"}}(n)))}(o,this),function(n,e){t&&(e._onlineHandler=function(){!function(n,t){var e;(e=mn(n),bn(e).filter(pn)).forEach((function(t){M(t,n.class_error),A(t)})),t.update()}(n,e)},window.addEventListener("online",e._onlineHandler))}(o,this),this.update(e)};return En.prototype={update:function(n){var t,o,a=this._settings,r=hn(n,a);G(this,r.length),!e&&i?gn(a)?function(n,t,e){n.forEach((function(n){-1!==_n.indexOf(n.tagName)&&function(n,t,e){n.setAttribute("loading","lazy"),cn(n,t,e),function(n,t){var e=Y[n.tagName];e&&e(n,t)}(n,t),w(n,h)}(n,t,e)})),G(e,0)}(r,a,this):(o=r,function(n){n.disconnect()}(t=this._observer),function(n,t){t.forEach((function(t){n.observe(t)}))}(t,o)):this.loadAll(r)},destroy:function(){this._observer&&this._observer.disconnect(),t&&window.removeEventListener("online",this._onlineHandler),mn(this._settings).forEach((function(n){U(n)})),delete this._observer,delete this._settings,delete this._onlineHandler,delete this.loadingCount,delete this.toLoadCount},loadAll:function(n){var t=this,e=this._settings;hn(n,e).forEach((function(n){T(n,t),ln(n,e,t)}))},restoreAll:function(){var n=this._settings;mn(n).forEach((function(t){fn(t,n)}))}},En.load=function(n,t){var e=c(t);ln(n,e)},En.resetStatus=function(n){A(n)},t&&function(n,t){if(t)if(t.length)for(var e,i=0;e=t[i];i+=1)l(n,e);else l(n,t)}(En,window.lazyLoadOptions),En}));

		document.addEventListener("DOMContentLoaded", function() {
			document.querySelectorAll("img").forEach(img => { 
				img.classList.add("lazy"); 
			});
		  });
		</script>
		</html>`, 1))

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
