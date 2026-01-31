//go:build !linux

package gatesentryproxy

import (
	"errors"
	"net"
	"net/http"
)

var TransparentProxyEnabled = false
var TransparentProxyPortHTTPS = 10414

func GetOriginalDestination(conn net.Conn) (string, error) {
	return "", errors.New("transparent proxy not supported on this platform")
}

func IsTransparentProxyRequest(r *http.Request) bool {
	return false
}

func HandleTransparentHTTP(w http.ResponseWriter, r *http.Request, h *ProxyHandler, originalDst string) {
}

func HandleTransparentHTTPS(conn net.Conn, h *ProxyHandler, originalDst string, user string, passthru *GSProxyPassthru) {
}

func TransparentHTTPHandler(h *ProxyHandler) http.Handler {
	return h
}

func IsLikelyTLSConnection(data []byte) bool {
	return false
}

func PeekConnectionData(conn net.Conn) ([]byte, net.Conn, error) {
	return nil, conn, errors.New("not supported")
}

type peekConn struct {
	net.Conn
	peeked []byte
	used   bool
}

func (c *peekConn) Read(b []byte) (int, error) {
	return c.Conn.Read(b)
}
