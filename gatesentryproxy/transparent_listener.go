//go:build linux

package gatesentryproxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

type TransparentProxyListener struct {
	net.Listener
	ProxyHandler *ProxyHandler
}

func NewTransparentProxyListener(listener net.Listener, handler *ProxyHandler) *TransparentProxyListener {
	return &TransparentProxyListener{
		Listener:     listener,
		ProxyHandler: handler,
	}
}

func CreateTransparentListener(addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, 19, 1) // 19 = IP_TRANSPARENT
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}
	return lc.Listen(context.Background(), "tcp", addr)
}

func (l *TransparentProxyListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}

	go l.handleConnection(conn)

	return &dummyConn{}, nil
}

func (l *TransparentProxyListener) handleConnection(conn net.Conn) {
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	buf := make([]byte, 3)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		if DebugLogging {
			log.Printf("[Transparent] Error reading initial bytes: %v", err)
		}
		return
	}

	conn.SetReadDeadline(time.Time{})

	wrappedConn := &prependConn{
		Conn:   conn,
		buf:    buf[:n],
		offset: 0,
	}

	isTLS := buf[0] == 0x16 && buf[1] == 0x03 && buf[2] >= 0x01

	if DebugLogging {
		if isTLS {
			log.Printf("[Transparent] Detected TLS connection from %s", conn.RemoteAddr())
		} else {
			log.Printf("[Transparent] Detected HTTP connection from %s", conn.RemoteAddr())
		}
	}

	originalDst, err := GetOriginalDestination(conn)
	if err != nil {
		if DebugLogging {
			log.Printf("[Transparent] Failed to get original destination: %v", err)
		}
		originalDst = conn.LocalAddr().String()
	}

	if DebugLogging {
		log.Printf("[Transparent] Original destination: %s", originalDst)
	}

	if isTLS {
		l.handleTransparentHTTPS(wrappedConn, originalDst)
	} else {
		l.handleTransparentHTTP(wrappedConn, originalDst)
	}
}

func (l *TransparentProxyListener) handleTransparentHTTP(conn net.Conn, originalDst string) {
	reader := bufio.NewReader(conn)

	req, err := http.ReadRequest(reader)
	if err != nil {
		if DebugLogging {
			log.Printf("[Transparent] Error reading HTTP request: %v", err)
		}
		return
	}

	req.URL.Scheme = "http"
	req.URL.Host = originalDst

	if req.Host == "" {
		req.Host = originalDst
	}

	respWriter := &connResponseWriter{
		conn:   conn,
		header: make(http.Header),
	}

	authEnabled := false
	if IProxy != nil && IProxy.IsAuthEnabled != nil {
		authEnabled = IProxy.IsAuthEnabled()
	}

	if authEnabled && DebugLogging {
		log.Printf("[Transparent] Authentication is enabled but may not work in transparent mode")
	}

	l.ProxyHandler.ServeHTTP(respWriter, req)

	if f, ok := respWriter.writer.(*bufio.Writer); ok {
		f.Flush()
	}
}

func (l *TransparentProxyListener) handleTransparentHTTPS(conn net.Conn, originalDst string) {
	host, port, err := net.SplitHostPort(originalDst)
	if err != nil {
		host = originalDst
		port = "443"
	}

	serverAddr := net.JoinHostPort(host, port)
	user := ""

	passthru := NewGSProxyPassthru()

	shouldMitm := false
	if IProxy != nil && IProxy.DoMitm != nil {
		shouldMitm = IProxy.DoMitm(serverAddr)
	}

	if shouldMitm {
		if DebugLogging {
			log.Printf("[Transparent] Performing SSL Bump for %s", serverAddr)
		}
		SSLBump(conn, serverAddr, user, "", nil, passthru, l.ProxyHandler.Iproxy)
	} else {
		if DebugLogging {
			log.Printf("[Transparent] Direct tunnel for %s", serverAddr)
		}
		ConnectDirect(conn, serverAddr, nil, passthru)
	}
}

type prependConn struct {
	net.Conn
	buf    []byte
	offset int
}

func (c *prependConn) Read(b []byte) (int, error) {
	if c.offset < len(c.buf) {
		n := copy(b, c.buf[c.offset:])
		c.offset += n
		return n, nil
	}
	return c.Conn.Read(b)
}

type dummyConn struct{}

func (c *dummyConn) Read(b []byte) (n int, err error)  { return 0, io.EOF }
func (c *dummyConn) Write(b []byte) (n int, err error) { return len(b), nil }
func (c *dummyConn) Close() error                      { return nil }
func (c *dummyConn) LocalAddr() net.Addr               { return &net.TCPAddr{} }
func (c *dummyConn) RemoteAddr() net.Addr              { return &net.TCPAddr{} }
func (c *dummyConn) SetDeadline(t time.Time) error     { return nil }
func (c *dummyConn) SetReadDeadline(t time.Time) error { return nil }
func (c *dummyConn) SetWriteDeadline(t time.Time) error {
	return nil
}

type connResponseWriter struct {
	conn        net.Conn
	header      http.Header
	wroteHeader bool
	statusCode  int
	writer      io.Writer
}

func (w *connResponseWriter) Header() http.Header {
	return w.header
}

func (w *connResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = code

	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", code, http.StatusText(code))
	w.conn.Write([]byte(statusLine))

	for key, values := range w.header {
		for _, value := range values {
			w.conn.Write([]byte(key + ": " + value + "\r\n"))
		}
	}
	w.conn.Write([]byte("\r\n"))
}

func (w *connResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.conn.Write(b)
}

func IsTransparentProxyEnabled() bool {
	return TransparentProxyEnabled
}

func SetTransparentProxyEnabled(enabled bool) {
	TransparentProxyEnabled = enabled
	if enabled {
		log.Println("[Transparent] Transparent proxy mode enabled")
	} else {
		log.Println("[Transparent] Transparent proxy mode disabled")
	}
}

func GetTransparentProxyPort() int {
	return TransparentProxyPortHTTPS
}

func SetTransparentProxyPort(port int) {
	TransparentProxyPortHTTPS = port
}

func DetectAndHandleTransparentConnection(conn net.Conn, handler *ProxyHandler) {
	listener := &TransparentProxyListener{
		Listener:     &singleConnListener{conn: conn},
		ProxyHandler: handler,
	}
	listener.handleConnection(conn)
}

type singleConnListener struct {
	conn   net.Conn
	used   bool
	closed bool
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if l.used {
		select {}
	}
	l.used = true
	return l.conn, nil
}

func (l *singleConnListener) Close() error {
	l.closed = true
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

func IsProbablyHTTPRequest(data []byte) bool {
	methods := []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH ", "CONNECT "}

	for _, method := range methods {
		if len(data) >= len(method) && strings.HasPrefix(string(data), method) {
			return true
		}
	}

	return false
}

type TransparentProxyServer struct {
	Server   *http.Server
	Handler  *ProxyHandler
	Listener net.Listener
}

func NewTransparentProxyServer(handler *ProxyHandler) *TransparentProxyServer {
	return &TransparentProxyServer{
		Handler: handler,
	}
}

func (s *TransparentProxyServer) Start(addr string) error {
	listener, err := CreateTransparentListener(addr)
	if err != nil {
		return err
	}

	s.Listener = NewTransparentProxyListener(listener, s.Handler)

	s.Server = &http.Server{
		Handler: s.Handler,
	}

	return s.Server.Serve(s.Listener)
}

func (s *TransparentProxyServer) Stop() error {
	if s.Listener != nil {
		return s.Listener.Close()
	}
	return nil
}
