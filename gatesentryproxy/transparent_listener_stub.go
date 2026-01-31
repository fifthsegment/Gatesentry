//go:build !linux

package gatesentryproxy

import (
	"errors"
	"log"
	"net"
	"net/http"
)

type TransparentProxyListener struct {
	net.Listener
	ProxyHandler *ProxyHandler
}

func NewTransparentProxyListener(listener net.Listener, handler *ProxyHandler) *TransparentProxyListener {
	return nil
}

func (l *TransparentProxyListener) Accept() (net.Conn, error) {
	return nil, errors.New("transparent proxy not supported on this platform")
}

func CreateTransparentListener(addr string) (net.Listener, error) {
	return nil, errors.New("transparent proxy not supported on this platform")
}

type TransparentProxyServer struct {
	Server   *http.Server
	Handler  *ProxyHandler
	Listener net.Listener
}

func NewTransparentProxyServer(handler *ProxyHandler) *TransparentProxyServer {
	return nil
}

func (s *TransparentProxyServer) Start(addr string) error {
	return errors.New("transparent proxy not supported on this platform")
}

func (s *TransparentProxyServer) Stop() error {
	return nil
}

func IsTransparentProxyEnabled() bool {
	return false
}

func SetTransparentProxyEnabled(enabled bool) {
	TransparentProxyEnabled = enabled
	if enabled {
		log.Println("[Transparent] Transparent proxy mode not supported on this platform")
	}
}

func GetTransparentProxyPort() int {
	return TransparentProxyPortHTTPS
}

func SetTransparentProxyPort(port int) {
	TransparentProxyPortHTTPS = port
}

func DetectAndHandleTransparentConnection(conn net.Conn, handler *ProxyHandler) {
}

func IsProbablyHTTPRequest(data []byte) bool {
	return false
}
