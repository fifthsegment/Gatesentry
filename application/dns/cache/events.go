package cache

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType classifies DNS cache events for SSE consumers.
type EventType string

const (
	EventQuery  EventType = "query"  // DNS query received (cache hit or miss)
	EventInsert EventType = "insert" // new entry cached
	EventEvict  EventType = "evict"  // entry evicted (capacity pressure)
	EventExpire EventType = "expire" // entry removed (TTL expired)
	EventFlush  EventType = "flush"  // entire cache flushed
	EventReaper EventType = "reaper" // background reaper sweep completed
)

// Event is a single DNS cache event emitted to SSE subscribers.
// Designed for JSON serialisation over Server-Sent Events.
type Event struct {
	Type      EventType `json:"type"`
	Timestamp int64     `json:"ts"` // Unix milliseconds
	Domain    string    `json:"domain,omitempty"`
	QType     string    `json:"qtype,omitempty"`
	Hit       bool      `json:"hit,omitempty"`        // for EventQuery
	TTL       int       `json:"ttl,omitempty"`        // remaining TTL in seconds
	Reason    string    `json:"reason,omitempty"`     // for EventEvict: "capacity" or "expired"
	Count     int       `json:"count,omitempty"`      // for EventReaper/EventFlush: entries affected
	CacheSize int64     `json:"cache_size,omitempty"` // current entry count after event
}

// JSON returns the event as a JSON byte slice for SSE data lines.
func (e *Event) JSON() []byte {
	b, _ := json.Marshal(e)
	return b
}

// subscriber represents a single SSE client listening for cache events.
type subscriber struct {
	ch     chan *Event
	closed bool
}

// EventBus provides a fan-out publish/subscribe system for DNS cache events.
// Multiple SSE clients can subscribe; events are delivered non-blocking —
// if a subscriber's buffer is full, the event is dropped for that subscriber
// (the DNS server must never block waiting for a slow browser).
//
// Memory safety: each subscriber gets a bounded channel (default 256 events).
// If 10 SSE clients connect, that's 10 × 256 × ~200 bytes ≈ 500 KB — fine
// for a Raspberry Pi. Subscribers that disconnect are cleaned up automatically.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[*subscriber]struct{}
	bufSize     int  // per-subscriber channel buffer size
	enabled     bool // when false, Emit is a no-op (zero overhead if no UI connected)
}

// DefaultBufSize is the per-subscriber event channel buffer.
// 256 events × ~200 bytes ≈ 50 KB per subscriber. At typical home DNS rates
// (10–50 QPS), this gives ~5–25 seconds of buffer before drops occur.
const DefaultBufSize = 256

// NewEventBus creates a new event bus. It starts disabled — call Enable()
// or it will auto-enable on the first Subscribe(). When disabled, Emit() is
// a no-op with zero overhead so the cache doesn't pay any cost when no
// SSE clients are connected.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[*subscriber]struct{}),
		bufSize:     DefaultBufSize,
		enabled:     false,
	}
}

// Subscribe registers a new event listener and returns a channel that
// receives cache events. Call Unsubscribe() with the returned channel
// when the SSE connection closes. The bus auto-enables on first subscriber.
func (eb *EventBus) Subscribe() chan *Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	sub := &subscriber{
		ch: make(chan *Event, eb.bufSize),
	}
	eb.subscribers[sub] = struct{}{}
	eb.enabled = true

	return sub.ch
}

// Unsubscribe removes a subscriber and closes its channel.
// Safe to call multiple times or with an unknown channel.
func (eb *EventBus) Unsubscribe(ch chan *Event) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for sub := range eb.subscribers {
		if sub.ch == ch {
			if !sub.closed {
				close(sub.ch)
				sub.closed = true
			}
			delete(eb.subscribers, sub)
			break
		}
	}

	// Auto-disable when no subscribers (saves overhead)
	if len(eb.subscribers) == 0 {
		eb.enabled = false
	}
}

// Emit sends an event to all subscribers. Non-blocking: if a subscriber's
// buffer is full, the event is dropped for that subscriber.
// When disabled (no subscribers), this is a fast no-op.
func (eb *EventBus) Emit(event *Event) {
	// Fast path: no subscribers — avoid lock entirely
	if !eb.enabled {
		return
	}

	eb.mu.RLock()
	defer eb.mu.RUnlock()

	for sub := range eb.subscribers {
		if sub.closed {
			continue
		}
		// Non-blocking send — drop event if subscriber is slow
		select {
		case sub.ch <- event:
		default:
			// Subscriber buffer full — drop this event.
			// This is intentional: we never block the DNS server for a slow browser.
		}
	}
}

// SubscriberCount returns the current number of active subscribers.
func (eb *EventBus) SubscriberCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	return len(eb.subscribers)
}

// Enable turns on event emission even without subscribers.
// Useful for testing or pre-warming.
func (eb *EventBus) Enable() {
	eb.mu.Lock()
	eb.enabled = true
	eb.mu.Unlock()
}

// ---------- Event constructors (convenience) ----------

func now() int64 {
	return time.Now().UnixMilli()
}

// QueryEvent creates an event for a DNS cache lookup.
func QueryEvent(domain, qtype string, hit bool, ttl int, cacheSize int64) *Event {
	return &Event{
		Type:      EventQuery,
		Timestamp: now(),
		Domain:    domain,
		QType:     qtype,
		Hit:       hit,
		TTL:       ttl,
		CacheSize: cacheSize,
	}
}

// InsertEvent creates an event for a new cache entry.
func InsertEvent(domain, qtype string, ttl int, cacheSize int64) *Event {
	return &Event{
		Type:      EventInsert,
		Timestamp: now(),
		Domain:    domain,
		QType:     qtype,
		TTL:       ttl,
		CacheSize: cacheSize,
	}
}

// EvictEvent creates an event for a capacity-driven eviction.
func EvictEvent(domain, qtype string, cacheSize int64) *Event {
	return &Event{
		Type:      EventEvict,
		Timestamp: now(),
		Domain:    domain,
		QType:     qtype,
		Reason:    "capacity",
		CacheSize: cacheSize,
	}
}

// ExpireEvent creates an event for a TTL expiry removal.
func ExpireEvent(domain, qtype string, cacheSize int64) *Event {
	return &Event{
		Type:      EventExpire,
		Timestamp: now(),
		Domain:    domain,
		QType:     qtype,
		Reason:    "expired",
		CacheSize: cacheSize,
	}
}

// ReaperEvent creates an event summarising a reaper sweep.
func ReaperEvent(count int, cacheSize int64) *Event {
	return &Event{
		Type:      EventReaper,
		Timestamp: now(),
		Count:     count,
		CacheSize: cacheSize,
	}
}

// FlushEvent creates an event for a full cache flush.
func FlushEvent(count int) *Event {
	return &Event{
		Type:      EventFlush,
		Timestamp: now(),
		Count:     count,
		CacheSize: 0,
	}
}
