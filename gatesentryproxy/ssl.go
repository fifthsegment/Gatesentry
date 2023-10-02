package gatesentryproxy

import (
	"encoding/json"
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

	// "io/ioutil"
	"bytes"

	// "sync"
	"reflect"

	"golang.org/x/net/http2"

	"strconv"

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
	Dial:                  dialer.Dial,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
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

type ByteCounter struct {
	count int64
}

func (c *ByteCounter) Write(p []byte) (int, error) {
	c.count += int64(len(p))
	return len(p), nil
}

func (c *ByteCounter) Count() int64 {
	return c.count
}

// ConnectDirect connects to serverAddr and copies data between it and conn.
// extraData is sent to the server first.
func ConnectDirect(conn net.Conn, serverAddr string, extraData []byte, gpt *GSProxyPassthru) (uploaded int64, downloaded int64, blocked bool) {
	// activeConnections.Add(1)
	// defer activeConnections.Done()

	serverConn, err := net.Dial("tcp", serverAddr)

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
		buf := make([]byte, 2000)
		n, _ := conn.Read(buf)
		conn.SetReadDeadline(time.Time{})
		if n > 0 {
			extraData = append(extraData, buf[:n]...)
		}
		serverConn.Write(extraData)
	}

	downloadedSizeChan := make(chan int64)

	go func() {
		log.Printf("Non-MITM connection : Writing data to connection")
		destwithcounter := &DataPassThru{Writer: conn, Contenttype: "", Passthru: gpt}

		// Create a counter for tracking downloaded bytes
		counter := &ByteCounter{}

		// Do a limited read to check the size without copying data
		limitedReader := io.LimitedReader{R: serverConn, N: 500 * 1024}
		_, err := io.Copy(io.MultiWriter(destwithcounter, counter), &limitedReader)
		gsInfo := GatesentrySSLHostnameWithDataSize{
			Hostname: serverAddr,
			Datasize: counter.Count(),
		}
		gsInfoByte, _ := json.Marshal(gsInfo)
		IProxy.RunHandler("ssl_contentlength_domain", serverAddr, &gsInfoByte, gpt)

		// Check if downloaded bytes exceed 500KB (in bytes)
		fmt.Println("Downloaded bytes = " + strconv.FormatInt(counter.Count(), 10))
		if strings.Contains(serverAddr, ".fbcdn.net") && counter.Count() >= 500*1023 && err == nil {

			fmt.Println("Downloaded bytes exceed 500KB . Host = " + serverAddr)
			// If more than 500KB, write an empty response
			conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
		} else {
			fmt.Println("Bytes = " + strconv.FormatInt(counter.Count(), 10) + " . Host = " + serverAddr)
			// If less than or equal to 500KB, copy the data
			_, _ = io.Copy(destwithcounter, serverConn)

		}

		time.Sleep(time.Second)
		conn.Close()

		// Send the downloaded byte count
		downloadedSizeChan <- counter.Count() + int64(len(extraData))
	}()

	uploaded, _ = io.Copy(serverConn, conn)

	serverConn.Close()
	downloaded = <-downloadedSizeChan
	return uploaded, downloaded, false
}

// SSLBump performs a man-in-the-middle attack on conn, to filter the HTTPS
// traffic. serverAddr is the address (host:port) of the server the client was
// trying to connect to. user is the username to use for logging; authUser is
// the authenticated user, if any; r is the CONNECT request, if any.
func SSLBump(conn net.Conn, serverAddr, user, authUser string, r *http.Request, gpt *GSProxyPassthru) {
	log.Printf("[SSL] Performing a SSL Bump")
	defer func() {
		if err := recover(); err != nil {
			buf := make([]byte, 4096)
			buf = buf[:runtime.Stack(buf, false)]
			log.Printf("SSLBump: panic serving connection to %s: %v\n%s", serverAddr, err, buf)
			conn.Close()
		}
	}()
	//PrintMemUsage()
	obsoleteVersion := false
	// Read the client hello so that we can find out the name of the server (not
	// just the address).
	clientHello, err := gsClientHello.ReadClientHello(conn)
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
			log.Println("[SSL] Server Name = " + sn)
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
	log.Println("[SSL] Server Name = " + serverName)
	//
	// CACHE CERT HERE
	//
	cert, rt := CertCache.Get(serverName, serverAddr)
	cachedCert := rt != nil
	log.Println("[SSL] Cached Cert existence = " + strconv.FormatBool(cachedCert))

	if !cachedCert {
		log.Println("[SSL] Starting process to cache certificate")
		log.Println("[SSL] Dialing connection to = " + serverAddr)
		serverConn, err := tls.Dial("tcp", serverAddr, &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: true,
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
		log.Println("[SSL] Validating certificate result = " + strconv.FormatBool(valid))

		cert, err = signCertificate(serverCert, !valid)
		if err != nil {
			log.Println("[SSL] Error signing certificate")
			log.Println("[SSL] Closing connection")
			serverConn.Close()
			GSLogSSL(user, serverAddr, serverName, fmt.Errorf("error generating certificate: %v", err), false)
			ConnectDirect(conn, serverAddr, clientHello, gpt)
			return
		}

		_, err = serverCert.Verify(x509.VerifyOptions{
			Intermediates: certPoolWith(state.PeerCertificates[1:]),
			DNSName:       serverName,
		})
		validWithDefaultRoots := err == nil

		if state.NegotiatedProtocol == "h2" && state.NegotiatedProtocolIsMutual {
			log.Println("[SSL] Negotiated Protocol is " + state.NegotiatedProtocol)
			if validWithDefaultRoots {
				log.Println("[SSL] Valid with default Roots using http Transport")
				rt = http2Transport
			} else {
				log.Println("[SSL] Using Hard Validation Transport")
				rt = newHardValidationTransport(insecureHTTP2Transport, serverName, state.PeerCertificates)
			}
		} else {
			log.Println("[SSL] Negotiated Protocol is " + state.NegotiatedProtocol)
			if validWithDefaultRoots {
				log.Println("[SSL] Valid with default Roots using http2 Transport")
				rt = httpTransport
			} else {
				log.Println("[SSL] Using Hard Validation Transport")
				rt = newHardValidationTransport(insecureHTTPTransport, serverName, state.PeerCertificates)
			}
		}
		CertCache.Put(serverName, serverAddr, cert, rt)
	}
	log.Println("[SSL] Setting up HTTP Server")
	server := http.Server{
		Handler: ProxyHandler{
			TLS:         true,
			connectPort: port,
			user:        authUser,
			rt:          rt,
		},
		TLSConfig: &tls.Config{
			NextProtos:   []string{"h2", "http/1.1"},
			Certificates: []tls.Certificate{cert, TLSCert},
		},
	}
	log.Println("[SSL] Configuring HTTP2 server with configuration")
	err = http2.ConfigureServer(&server, nil)
	if err != nil {
		log.Println("Error configuring HTTP/2 server:", err)
	}
	log.Println("[SSL] Setting up TLS Connection with inserted data")
	tlsConn := tls.Server(&insertingConn{conn, clientHello}, server.TLSConfig)
	err = tlsConn.Handshake()
	log.Println("[SSL] Performed TLS Handshake")
	if err != nil {
		GSLogSSL(user, serverAddr, serverName, fmt.Errorf("error in handshake with client: %v", err), cachedCert)
		conn.Close()
		return
	}

	listener := &singleListener{conn: tlsConn}
	GSLogSSL(user, serverAddr, serverName, nil, cachedCert)
	// conf = nil
	server.Serve(listener)

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
	log.Println("[SSL] client Hello Server name")
	log.Println("[SSL] Data length = " + strconv.Itoa(len(data)))
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
