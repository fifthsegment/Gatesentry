package gatesentryproxy

import (
	"context"
	"io"
	"log"
	"net"
	"time"

	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"bytes"
	"reflect"

	"golang.org/x/net/http2"

	gsClientHello "bitbucket.org/abdullah_irfan/gatesentryproxy/clienthello"
)

var BlockObsoleteSSL = true
var ExtraRootCerts *x509.CertPool
var CertCache CertificateCache
var TLSCert tls.Certificate

var CertFile string
var KeyFile string
var ParsedTLSCert *x509.Certificate
var ServeMux *http.ServeMux
var errCouldNotVerify = errors.New("could not verify server certificate")

// unverifiedClientConfig is a TLS configuration that doesn't verify server
// certificates.
var unverifiedClientConfig = &tls.Config{
	InsecureSkipVerify: false,
}

var insecureHTTPTransport = &http.Transport{
	TLSClientConfig:       unverifiedClientConfig,
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           safeDialContext,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	DisableCompression:    true, // Phase 3: don't auto-decompress; proxy handles it per-path
}

var http2Transport = &http2.Transport{
	DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
		return tls.DialWithDialer(dialer, network, addr, cfg)
	},
}

var insecureHTTP2Transport = &http2.Transport{
	TLSClientConfig: unverifiedClientConfig,
	DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
		return tls.DialWithDialer(dialer, network, addr, cfg)
	},
}

// A hardValidationTransport wraps another (insecure) RoundTripper and checks
// the server certificates various ways, including against an earlier
// connection's certificates. If any of the checks pass, the certificate is
// accepted.
type hardValidationTransport struct {
	rt http.RoundTripper

	originalCertificates []*x509.Certificate
	originalServerName   string

	// originalCertPool is a CertPool containing the certificates from originalCertificates
	originalCertPool *x509.CertPool

	// expectedErrDefault is the error that was received when validating the
	// original certificate against the system default CAs.
	expectedErrDefault error

	// expectedErrOriginal is the error that was received when validating
	// the original certificate against its own certificate chain.
	// It should normally be nil, but not always.
	expectedErrOriginal error
}

func Init(cert string, key string) {
	CertFile = cert
	KeyFile = key
	// ServeMux = http.NewServeMux()
	// ServerCertPool()
	loadCertificate()
}

func InitWithDataCerts(certPEMBlock, keyPEMBlock []byte) {
	// ServeMux = http.NewServeMux()
	loadCertificateWithData(certPEMBlock, keyPEMBlock)
}

func GSLogSSL(user string, serverAddr string, serverName string, err error, cachedCert bool) {
	entry := fmt.Sprintf("[SSL] [User: %s] [Address: %s] [Sname: %s]", user, serverAddr, serverName)
	log.Println(entry)
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

func HandleSSLBump(r *http.Request, w http.ResponseWriter, user string, authUser string, passthru *GSProxyPassthru, gsproxy *GSProxy) {
	conn, err := newHijackedConn(w)
	if err != nil {
		log.Printf("Error hijacking connection for CONNECT request to %s: %v", r.URL.Host, err)
		errorData := &GSProxyErrorData{
			Error: "Error hijacking connection for CONNECT request to " + r.URL.Host + ": " + err.Error(),
		}
		IProxy.ProxyErrorHandler(errorData)
		sendBlockMessageBytes(w, r, nil, errorData.FilterResponse, nil)
		return
	}
	fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	SSLBump(conn, r.URL.Host, user, authUser, r, passthru, gsproxy, nil)
}

func HandleSSLConnectDirect(r *http.Request, w http.ResponseWriter, user string, passthru *GSProxyPassthru) {
	conn, err := newHijackedConn(w)
	if err != nil {
		log.Printf("Error hijacking connection for CONNECT request to %s: %v", r.URL.Host, err)
		return
	}
	fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	// logAccess(r, nil, 0, false, user, tally, scores, thisRule, "", ignored)
	// conf = nil // Allow it to be garbage-collected, since we won't use it any more.
	ConnectDirect(conn, r.URL.Host, nil, passthru)
}

// ConnectDirect connects to serverAddr and copies data between it and conn.
// extraData is sent to the server first.
func ConnectDirect(conn net.Conn, serverAddr string, extraData []byte, gpt *GSProxyPassthru) (uploaded, downloaded int64) {
	// activeConnections.Add(1)
	// defer activeConnections.Done()
	log.Println("Running a CONNECTDIRECT TCP to " + serverAddr)
	serverConn, err := safeDialContext(context.Background(), "tcp", serverAddr)

	if err != nil {
		log.Printf("error with pass-through of SSL connection to %s: %s", serverAddr, err)
		conn.Close()
		return
	}

	if extraData != nil {
		// There may also be data waiting in the socket's input buffer;
		// read it before we send the data on, so that the first packet of
		// the connection doesn't get split in two.
		conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		bufPtr := GetSmallBuffer()
		buf := *bufPtr
		n, _ := conn.Read(buf[:2000])
		PutSmallBuffer(bufPtr)
		conn.SetReadDeadline(time.Time{})
		if n > 0 {
			extraData = append(extraData, buf[:n]...)
		}
		serverConn.Write(extraData)
	}

	ulChan := make(chan int64)
	go func() {
		log.Printf("Non-MITM connection : Writing data to connection")
		destwithcounter := &DataPassThru{Writer: conn, Contenttype: "", Passthru: gpt}
		n, _ := io.Copy(destwithcounter, serverConn)
		time.Sleep(time.Second)
		conn.Close()
		ulChan <- n + int64(len(extraData))
	}()

	// go func() {
	// 	log.Printf("Non-MITM connection : Writing data to connection")
	// 	destwithcounter := &DataPassThru{Writer: conn, Contenttype: "", Passthru: gpt}

	// 	// Create a counter for tracking downloaded bytes
	// 	counter := &ByteCounter{}

	// 	// Do a limited read to check the size without copying data
	// 	limitedReader := io.LimitedReader{R: serverConn, N: 500 * 1024}
	// 	_, err := io.Copy(io.MultiWriter(destwithcounter, counter), &limitedReader)
	// 	gsInfo := GatesentrySSLHostnameWithDataSize{
	// 		Hostname: serverAddr,
	// 		Datasize: counter.Count(),
	// 	}
	// 	gsInfoByte, _ := json.Marshal(gsInfo)
	// 	IProxy.RunHandler("ssl_contentlength_domain", serverAddr, &gsInfoByte, gpt)

	// 	// Check if downloaded bytes exceed 500KB (in bytes)
	// 	fmt.Println("Downloaded bytes = " + strconv.FormatInt(counter.Count(), 10))
	// 	if strings.Contains(serverAddr, ".fbcdn.net") && counter.Count() >= 500*1023 && err == nil {

	// 		fmt.Println("Downloaded bytes exceed 500KB . Host = " + serverAddr)
	// 		// If more than 500KB, write an empty response
	// 		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
	// 	} else {
	// 		fmt.Println("Bytes = " + strconv.FormatInt(counter.Count(), 10) + " . Host = " + serverAddr)
	// 		// If less than or equal to 500KB, copy the data
	// 		_, _ = io.Copy(destwithcounter, serverConn)
	//
	// 	}

	// 	time.Sleep(time.Second)
	// 	conn.Close()

	// 	// Send the downloaded byte count
	// 	downloadedSizeChan <- counter.Count() + int64(len(extraData))
	// }()

	downloaded, _ = io.Copy(serverConn, conn)
	serverConn.Close()
	uploaded = <-ulChan
	return uploaded, downloaded
}

// SSLBump performs a man-in-the-middle attack on conn, to filter the HTTPS
// traffic. serverAddr is the address (host:port) of the server the client was
// trying to connect to. user is the username to use for logging; authUser is
// the authenticated user, if any; r is the CONNECT request, if any.
// If clientHelloData is provided (non-nil), it will be used instead of reading
// from the connection (used in transparent proxy mode where ClientHello was already read).
func SSLBump(conn net.Conn, serverAddr, user, authUser string, r *http.Request, gpt *GSProxyPassthru, gsproxy *GSProxy, clientHelloData []byte) {
	if DebugLogging {
		log.Printf("[SSL] Performing a SSL Bump")
	}
	defer func() {
		if err := recover(); err != nil {
			bufPtr := GetSmallBuffer()
			buf := *bufPtr
			n := runtime.Stack(buf, false)
			log.Printf("SSLBump: panic serving connection to %s: %v\n%s", serverAddr, err, buf[:n])
			PutSmallBuffer(bufPtr)
			conn.Close()
		}
	}()
	//PrintMemUsage()
	obsoleteVersion := false
	// Read the client hello so that we can find out the name of the server (not
	// just the address).
	var clientHello []byte
	var err error
	if clientHelloData != nil {
		clientHello = clientHelloData
	} else {
		clientHello, err = gsClientHello.ReadClientHello(conn)
	}

	if err != nil {
		GSLogSSL(user, serverAddr, "", fmt.Errorf("error reading client hello: %v", err), false)

		if _, ok := err.(net.Error); ok {
			conn.Close()
			return
		} else if err == gsClientHello.ErrObsoleteSSLVersion {
			obsoleteVersion = true
			if BlockObsoleteSSL {
				conn.Close()
				return
			}
		} else {
			// conf = nil
			ConnectDirect(conn, serverAddr, clientHello, gpt)
			return
		}
	}

	host, port, err := net.SplitHostPort(serverAddr)
	if err != nil {
		host = serverAddr
		port = "443"
	}
	_ = port
	serverName := ""
	if !obsoleteVersion {
		if sn, ok := clientHelloServerName(clientHello); ok {
			serverName = sn
		}
	}
	if serverName == "" {
		serverName = host
		if ip := net.ParseIP(serverName); ip != nil {
			// All we have is an IP address, not a name from a CONNECT request.
			// See if we can do better by reverse DNS.
			names, err := net.LookupAddr(serverName)
			if err == nil && len(names) > 0 {
				serverName = strings.TrimSuffix(names[0], ".")
			}
		}
	}
	//
	// CACHE CERT HERE
	//
	cert, rt := CertCache.Get(serverName, serverAddr)
	cachedCert := rt != nil

	if !cachedCert {
		serverConn, err := tls.Dial("tcp", serverAddr, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: false,
			NextProtos:         []string{"h2", "http/1.1"},
		})
		if err != nil {
			GSLogSSL(user, serverAddr, serverName, err, true)
			// conf = nil
			ConnectDirect(conn, serverAddr, clientHello, gpt)
			return
		}

		state := serverConn.ConnectionState()
		serverConn.Close()
		serverCert := state.PeerCertificates[0]

		valid := validCert(serverCert, state.PeerCertificates[1:])

		cert, err = signCertificate(serverCert, !valid)
		if err != nil {
			serverConn.Close()
			GSLogSSL(user, serverAddr, serverName, fmt.Errorf("error generating certificate: %v", err), false)
			ConnectDirect(conn, serverAddr, clientHello, gpt)
			return
		}
		// requestUrlBytes_log := []byte(serverAddr)
		gpt.ProxyActionToLog = ProxyActionSSLBump
		// gsproxy.RunHandler("log", "", &requestUrlBytes_log, gpt)
		gsproxy.LogHandler(GSLogData{
			User:   user,
			Action: ProxyActionSSLBump,
			Url:    serverAddr,
		})

		_, err = serverCert.Verify(x509.VerifyOptions{
			Intermediates: certPoolWith(state.PeerCertificates[1:]),
			DNSName:       serverName,
		})
		validWithDefaultRoots := err == nil

		if state.NegotiatedProtocol == "h2" && state.NegotiatedProtocolIsMutual {
			if validWithDefaultRoots {
				rt = http2Transport
			} else {
				rt = newHardValidationTransport(insecureHTTP2Transport, serverName, state.PeerCertificates)
			}
		} else {
			if validWithDefaultRoots {
				rt = httpTransport
			} else {
				rt = newHardValidationTransport(insecureHTTPTransport, serverName, state.PeerCertificates)
			}
		}
		CertCache.Put(serverName, serverAddr, cert, rt)
	}
	log.Println("[SSL] Setting up HTTP Server")
	// Create TLS config for the handshake (not for the server)
	tlsConfig := &tls.Config{
		NextProtos:   []string{"h2", "http/1.1"},
		Certificates: []tls.Certificate{cert, TLSCert},
	}
	tlsConn := tls.Server(&insertingConn{conn, clientHello}, tlsConfig)
	err = tlsConn.Handshake()
	if err != nil {
		GSLogSSL(user, serverAddr, serverName, fmt.Errorf("error in handshake with client: %v", err), cachedCert)
		conn.Close()
		return
	}

	clientState := tlsConn.ConnectionState()
	negotiatedProto := clientState.NegotiatedProtocol

	handler := ProxyHandler{
		TLS:         true,
		connectPort: port,
		user:        authUser,
		rt:          rt,
		Iproxy:      gsproxy,
	}

	GSLogSSL(user, serverAddr, serverName, nil, cachedCert)

	// Set up HTTP server with HTTP/2 support
	server := http.Server{
		Handler: handler,
	}

	if err := http2.ConfigureServer(&server, nil); err != nil {
		log.Printf("[SSL] Error configuring HTTP/2 server: %v", err)
	}

	// Handle HTTP/2 directly if negotiated via ALPN
	if negotiatedProto == "h2" {
		h2s := &http2.Server{}
		h2s.ServeConn(tlsConn, &http2.ServeConnOpts{
			Handler: handler,
			Context: context.Background(),
		})
	} else {
		// Use standard HTTP/1.1 server with the TLS connection
		listener := &singleListener{conn: tlsConn}
		server.Serve(listener)
	}
}

func newHardValidationTransport(rt http.RoundTripper, serverName string, certificates []*x509.Certificate) *hardValidationTransport {
	t := &hardValidationTransport{
		rt:                   rt,
		originalCertificates: certificates,
		originalServerName:   serverName,
		originalCertPool:     x509.NewCertPool(),
	}

	for _, cert := range certificates {
		t.originalCertPool.AddCert(cert)
	}

	_, t.expectedErrDefault = certificates[0].Verify(x509.VerifyOptions{
		Intermediates: t.originalCertPool,
		DNSName:       serverName,
	})

	_, t.expectedErrOriginal = certificates[0].Verify(x509.VerifyOptions{
		Roots:   t.originalCertPool,
		DNSName: serverName,
	})

	return t
}

func sameType(a, b interface{}) bool {
	return reflect.TypeOf(a) == reflect.TypeOf(b)
}

func (t *hardValidationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	log.Println("[SSL] RoundTrip")
	resp, err := t.rt.RoundTrip(req)
	if err != nil {
		log.Println("[SSL] Roundtrip error ")
		return resp, err
	}
	log.Println("[SSL] Checking for pubic key")
	// Check for public key equality first, since it's cheap.
	if bytes.Equal(resp.TLS.PeerCertificates[0].RawSubjectPublicKeyInfo, t.originalCertificates[0].RawSubjectPublicKeyInfo) {
		return resp, nil
	}

	serverCert := resp.TLS.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, ic := range resp.TLS.PeerCertificates[1:] {
		intermediates.AddCert(ic)
	}

	_, err = serverCert.Verify(x509.VerifyOptions{
		Intermediates: intermediates,
		DNSName:       req.Host,
	})
	if err == nil || sameType(err, t.expectedErrDefault) {
		return resp, nil
	}

	_, err = serverCert.Verify(x509.VerifyOptions{
		Intermediates: intermediates,
		DNSName:       req.Host,
		Roots:         t.originalCertPool,
	})
	if err == nil || sameType(err, t.expectedErrOriginal) {
		return resp, nil
	}

	if req.Host != t.originalServerName {
		_, err := serverCert.Verify(x509.VerifyOptions{
			Intermediates: intermediates,
			DNSName:       t.originalServerName,
		})
		if err == nil || sameType(err, t.expectedErrDefault) {
			return resp, nil
		}

		_, err = serverCert.Verify(x509.VerifyOptions{
			Intermediates: intermediates,
			DNSName:       t.originalServerName,
			Roots:         t.originalCertPool,
		})
		if err == nil || sameType(err, t.expectedErrOriginal) {
			return resp, nil
		}
	}

	resp.Body.Close()
	return resp, errCouldNotVerify
}

func clientHelloServerName(data []byte) (name string, ok bool) {
	var hello = gsClientHello.ClientHello{}
	err := hello.Unmarshall(data)
	return hello.SNI, err == nil
}

// A insertingConn is a net.Conn that inserts extra data at the start of the
// incoming data stream.
type insertingConn struct {
	net.Conn
	extraData []byte
}

func (c *insertingConn) Read(p []byte) (n int, err error) {
	if len(c.extraData) == 0 {
		return c.Conn.Read(p)
	}

	n = copy(p, c.extraData)
	c.extraData = c.extraData[n:]
	return
}
