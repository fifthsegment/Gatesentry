package gatesentryproxy

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// HandleWebsocketConnection upgrades an HTTP request to a bidirectional
// WebSocket tunnel. The proxy acts as a transparent relay — it forwards
// the client's upgrade request to the upstream server, relays the 101
// Switching Protocols response back, and then copies data bidirectionally
// until either side closes the connection.
//
// No content inspection is performed on WebSocket frames — they are
// opaque to the proxy, same as CONNECT tunnel traffic.
func HandleWebsocketConnection(r *http.Request, w http.ResponseWriter) {
	// Determine upstream address from the request URL
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if host == "" {
		http.Error(w, "Bad Request: missing host", http.StatusBadRequest)
		return
	}

	// Default to port 80 for plain HTTP WebSocket connections
	if !strings.Contains(host, ":") {
		host = host + ":80"
	}

	// Connect to upstream
	serverConn, err := safeDialContext(r.Context(), "tcp", host)
	if err != nil {
		log.Printf("[WebSocket] Failed to connect to upstream %s: %v", host, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// Hijack the client connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		log.Printf("[WebSocket] ResponseWriter does not support Hijack")
		serverConn.Close()
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		log.Printf("[WebSocket] Hijack failed: %v", err)
		serverConn.Close()
		return
	}

	// Forward the original upgrade request to upstream.
	// Reconstruct the HTTP request line and headers.
	reqURI := r.URL.RequestURI()
	fmt.Fprintf(serverConn, "%s %s HTTP/1.1\r\n", r.Method, reqURI)
	fmt.Fprintf(serverConn, "Host: %s\r\n", r.Host)
	for key, values := range r.Header {
		for _, v := range values {
			fmt.Fprintf(serverConn, "%s: %s\r\n", key, v)
		}
	}
	fmt.Fprintf(serverConn, "\r\n")

	// Read the upstream response (should be 101 Switching Protocols)
	serverBuf := bufio.NewReader(serverConn)
	upstreamResp, err := http.ReadResponse(serverBuf, r)
	if err != nil {
		log.Printf("[WebSocket] Failed to read upstream response: %v", err)
		clientConn.Close()
		serverConn.Close()
		return
	}

	// Forward the upstream response back to the client
	if err := upstreamResp.Write(clientConn); err != nil {
		log.Printf("[WebSocket] Failed to write response to client: %v", err)
		clientConn.Close()
		serverConn.Close()
		return
	}

	if upstreamResp.StatusCode != http.StatusSwitchingProtocols {
		log.Printf("[WebSocket] Upstream returned %d instead of 101", upstreamResp.StatusCode)
		clientConn.Close()
		serverConn.Close()
		return
	}

	if DebugLogging {
		log.Printf("[WebSocket] Tunnel established: %s → %s", r.RemoteAddr, host)
	}

	// Bidirectional copy — same pattern as ConnectDirect.
	// If the serverBuf has buffered data from the upstream response,
	// we need to drain it first.
	var serverReader io.Reader = serverConn
	if serverBuf.Buffered() > 0 {
		serverReader = io.MultiReader(serverBuf, serverConn)
	}

	var clientReader io.Reader = clientConn
	if clientBuf.Reader.Buffered() > 0 {
		clientReader = io.MultiReader(clientBuf, clientConn)
	}

	done := make(chan struct{}, 2)

	// Server → Client
	go func() {
		io.Copy(clientConn, serverReader)
		// Graceful half-close if the underlying connection supports it
		if tc, ok := clientConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	// Client → Server
	go func() {
		io.Copy(serverConn, clientReader)
		if tc, ok := serverConn.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}()

	// Wait for one direction to finish, then set a deadline on both
	// to allow the other direction to drain gracefully.
	<-done
	clientConn.SetDeadline(time.Now().Add(5 * time.Second))
	serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	<-done

	clientConn.Close()
	serverConn.Close()

	if DebugLogging {
		log.Printf("[WebSocket] Tunnel closed: %s → %s", r.RemoteAddr, host)
	}
}
