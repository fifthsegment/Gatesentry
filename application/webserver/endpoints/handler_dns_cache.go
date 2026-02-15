package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

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

// GSApiDNSCacheHistory returns recent per-minute cache stat snapshots.
// GET /api/dns/cache/stats/history?minutes=60
//
// Each snapshot contains cumulative counters.  The frontend computes deltas
// between consecutive entries to derive per-minute hit/miss rates.
func GSApiDNSCacheHistory(w http.ResponseWriter, r *http.Request) {
	rec := gatesentryDnsServer.GetCacheRecorder()
	if rec == nil {
		http.Error(w, `{"error":"Cache recorder not available"}`, http.StatusServiceUnavailable)
		return
	}

	minutes := 60 // default to last hour
	if m := r.URL.Query().Get("minutes"); m != "" {
		if v, err := strconv.Atoi(m); err == nil && v > 0 {
			minutes = v
		}
	}

	history := rec.GetHistory(minutes)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
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
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	maxDuration := time.NewTimer(4 * time.Hour)
	defer maxDuration.Stop()

	for {
		select {
		case <-ctx.Done():
			// Client disconnected.
			return
		case <-maxDuration.C:
			// Force client to reconnect after max duration to re-validate JWT
			fmt.Fprintf(w, "event: reconnect\ndata: {\"reason\":\"max_duration\"}\n\n")
			flusher.Flush()
			return
		case <-heartbeat.C:
			// SSE comment heartbeat to detect dead TCP connections.
			// Without this, idle connections with disconnected clients
			// block forever as zombie goroutines.
			_, err := fmt.Fprintf(w, ": heartbeat %d\n\n", time.Now().Unix())
			if err != nil {
				return // write failed — client disconnected
			}
			flusher.Flush()
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
