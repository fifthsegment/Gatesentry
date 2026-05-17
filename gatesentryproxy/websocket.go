package gatesentryproxy

import (
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

func HandleWebsocketConnection(r *http.Request, w http.ResponseWriter) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	upstreamAddr := r.URL.Host
	if upstreamAddr == "" {
		upstreamAddr = r.Host
	}
	if _, _, err := net.SplitHostPort(upstreamAddr); err != nil {
		upstreamAddr = net.JoinHostPort(upstreamAddr, "80")
	}

	upstreamConn, err := net.DialTimeout("tcp", upstreamAddr, 10*time.Second)
	if err != nil {
		log.Printf("WebSocket proxy: dial %s: %v", upstreamAddr, err)
		return
	}
	defer upstreamConn.Close()

	r.RequestURI = r.URL.RequestURI()

	if err := r.Write(upstreamConn); err != nil {
		log.Printf("WebSocket proxy: write request to %s: %v", upstreamAddr, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(upstreamConn, clientConn)
		upstreamConn.Close()
		wg.Done()
	}()
	go func() {
		io.Copy(clientConn, upstreamConn)
		clientConn.Close()
		wg.Done()
	}()
	wg.Wait()
}
