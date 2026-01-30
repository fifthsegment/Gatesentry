//go:build linux

package gatesentryproxy

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var TransparentProxyEnabled = false
var TransparentProxyPortHTTPS = 10414

type sockaddrIn struct {
	Family uint16
	Port   uint16
	Addr   [4]byte
	Zero   [8]byte
}

func GetOriginalDestination(conn net.Conn) (string, error) {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return "", errors.New("not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		return "", fmt.Errorf("failed to get file descriptor: %w", err)
	}
	defer file.Close()

	fd := int(file.Fd())
	const SO_ORIGINAL_DST = 80

	var addr sockaddrIn
	addrLen := uint32(unsafe.Sizeof(addr))

	_, _, errno := syscall.Syscall6(
		syscall.SYS_GETSOCKOPT,
		uintptr(fd),
		uintptr(syscall.SOL_IP),
		uintptr(SO_ORIGINAL_DST),
		uintptr(unsafe.Pointer(&addr)),
		uintptr(unsafe.Pointer(&addrLen)),
		0,
	)

	if errno != 0 {
		return "", fmt.Errorf("getsockopt SO_ORIGINAL_DST failed: %w", errno)
	}

	port := (uint16(addr.Port) >> 8) | (uint16(addr.Port) << 8)
	ip := net.IP(addr.Addr[:])

	return net.JoinHostPort(ip.String(), strconv.Itoa(int(port))), nil
}

func IsTransparentProxyRequest(r *http.Request) bool {
	if r.URL.Host == "" || r.URL.Scheme == "" {
		return true
	}

	if !strings.HasPrefix(r.URL.Path, "http://") &&
		!strings.HasPrefix(r.URL.Path, "https://") &&
		r.Host != "" &&
		r.URL.Host == "" {
		return true
	}

	return false
}

func HandleTransparentHTTP(w http.ResponseWriter, r *http.Request, h *ProxyHandler, originalDst string) {
	if DebugLogging {
		log.Printf("[Transparent] Handling HTTP request to %s from %s", originalDst, r.RemoteAddr)
	}

	r.URL.Scheme = "http"
	r.URL.Host = originalDst

	if r.Host == "" {
		r.Host = originalDst
	}

	h.ServeHTTP(w, r)
}

func HandleTransparentHTTPS(conn net.Conn, h *ProxyHandler, originalDst string, user string, passthru *GSProxyPassthru) {
	if DebugLogging {
		log.Printf("[Transparent] Handling HTTPS connection to %s from %s", originalDst, conn.RemoteAddr())
	}

	host, port, err := net.SplitHostPort(originalDst)
	if err != nil {
		host = originalDst
		port = "443"
	}

	serverAddr := net.JoinHostPort(host, port)

	shouldMitm := false
	if IProxy != nil && IProxy.DoMitm != nil {
		shouldMitm = IProxy.DoMitm(serverAddr)
	}

	if shouldMitm {
		if DebugLogging {
			log.Printf("[Transparent] Performing SSL Bump for %s", serverAddr)
		}
		SSLBump(conn, serverAddr, user, "", nil, passthru, h.Iproxy)
	} else {
		if DebugLogging {
			log.Printf("[Transparent] Direct tunnel for %s", serverAddr)
		}
		ConnectDirect(conn, serverAddr, nil, passthru)
	}
}

func TransparentHTTPHandler(h *ProxyHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsTransparentProxyRequest(r) {
			host := r.Host
			if host == "" {
				http.Error(w, "No Host header", http.StatusBadRequest)
				return
			}

			if !strings.Contains(host, ":") {
				host = net.JoinHostPort(host, "80")
			}

			HandleTransparentHTTP(w, r, h, host)
			return
		}

		h.ServeHTTP(w, r)
	})
}

func IsLikelyTLSConnection(data []byte) bool {
	if len(data) < 3 {
		return false
	}

	if data[0] == 0x16 && data[1] == 0x03 && data[2] >= 0x01 {
		return true
	}

	return false
}

func PeekConnectionData(conn net.Conn) ([]byte, net.Conn, error) {
	buf := make([]byte, 3)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, conn, err
	}

	wrappedConn := &peekConn{
		Conn:   conn,
		peeked: buf[:n],
	}

	return buf[:n], wrappedConn, nil
}

type peekConn struct {
	net.Conn
	peeked []byte
	used   bool
}

func (c *peekConn) Read(b []byte) (int, error) {
	if !c.used && len(c.peeked) > 0 {
		c.used = true
		n := copy(b, c.peeked)
		if n < len(c.peeked) {
			return n, nil
		}
		return n, nil
	}
	return c.Conn.Read(b)
}
