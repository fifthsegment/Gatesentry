package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	dnscache "bitbucket.org/abdullah_irfan/gatesentryf/dns/cache"
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
)

// dnsCacheOrError returns the DNS cache or writes a 503 error.
func dnsCacheOrError(w http.ResponseWriter) *dnscache.DNSCache {
	c := gatesentryDnsServer.GetDNSCache()
	if c == nil {
		http.Error(w, `{"error":"DNS cache not initialized — DNS server may not be running"}`, http.StatusServiceUnavailable)
		return nil
	}
	return c
}

// GSApiDNSCacheStats returns current cache statistics.
// GET /api/dns/cache/stats
func GSApiDNSCacheStats(w http.ResponseWriter, r *http.Request) {
	c := dnsCacheOrError(w)
	if c == nil {
		return
	}

	snap := c.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

// GSApiDNSCacheFlush clears all entries from the DNS cache.
// POST /api/dns/cache/flush
func GSApiDNSCacheFlush(w http.ResponseWriter, r *http.Request) {
	c := dnsCacheOrError(w)
	if c == nil {
		return
	}

	c.Flush()
	log.Println("[DNS Cache] Flushed via admin API")

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"flushed"}`))
}

// GSApiDNSEvents streams real-time DNS cache events via Server-Sent Events (SSE).
// GET /api/dns/events
//
// The endpoint subscribes to the cache's EventBus and forwards events to the
// client as SSE "data:" frames.  The connection stays open until the client
// disconnects (context cancellation) or the DNS cache shuts down.
//
// Wire protocol:
//
//	data: {"type":"query","domain":"example.com","qtype":"A","hit":true, ...}\n\n
//
// Clients reconnect automatically via the EventSource API.
func GSApiDNSEvents(w http.ResponseWriter, r *http.Request) {
	c := dnsCacheOrError(w)
	if c == nil {
		return
	}

	if c.Events == nil {
		http.Error(w, `{"error":"Event bus not available"}`, http.StatusServiceUnavailable)
		return
	}

	// Ensure the ResponseWriter supports flushing (required for SSE).
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"Streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	// Subscribe to the event bus; unsubscribe when the handler returns.
	ch := c.Events.Subscribe()
	defer c.Events.Unsubscribe(ch)

	// SSE headers — must be set before the first write.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

	// Send an initial comment so the client knows the connection is live.
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected.
			return
		case evt, ok := <-ch:
			if !ok {
				// Channel closed — cache or event bus shutting down.
				return
			}
			data, err := json.Marshal(evt)
			if err != nil {
				log.Printf("[SSE] Error marshalling event: %v", err)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
