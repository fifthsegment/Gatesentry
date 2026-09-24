package gatesentryproxy

import (
	"net"
	"sync/atomic"
	"time"
)

// TrafficSnapshot is the cumulative traffic handled by client-facing proxy
// sockets since this process initialized the counter.
type TrafficSnapshot struct {
	UploadBytes   uint64    `json:"upload_bytes"`
	DownloadBytes uint64    `json:"download_bytes"`
	TotalBytes    uint64    `json:"total_bytes"`
	StartedAt     time.Time `json:"started_at"`
}

// TrafficCounter counts bytes at the client-facing connection boundary.
type TrafficCounter struct {
	uploadBytes   atomic.Uint64
	downloadBytes atomic.Uint64
	startedAt     time.Time
}

func NewTrafficCounter() *TrafficCounter {
	return &TrafficCounter{startedAt: time.Now().UTC()}
}

func (c *TrafficCounter) Snapshot() TrafficSnapshot {
	upload := c.uploadBytes.Load()
	download := c.downloadBytes.Load()
	return TrafficSnapshot{
		UploadBytes:   upload,
		DownloadBytes: download,
		TotalBytes:    upload + download,
		StartedAt:     c.startedAt,
	}
}

type trafficConn struct {
	net.Conn
	counter *TrafficCounter
}

func (c *trafficConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.counter.uploadBytes.Add(uint64(n))
	return n, err
}

func (c *trafficConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.counter.downloadBytes.Add(uint64(n))
	return n, err
}

func (c *trafficConn) Unwrap() net.Conn                { return c.Conn }
func (c *trafficConn) trafficCounter() *TrafficCounter { return c.counter }

func (c *TrafficCounter) WrapConn(conn net.Conn) net.Conn {
	if conn == nil {
		return nil
	}
	for current := conn; current != nil; {
		if counted, ok := current.(interface{ trafficCounter() *TrafficCounter }); ok && counted.trafficCounter() == c {
			return conn
		}
		unwrapper, ok := current.(interface{ Unwrap() net.Conn })
		if !ok {
			break
		}
		next := unwrapper.Unwrap()
		if next == nil || next == current {
			break
		}
		current = next
	}
	return &trafficConn{Conn: conn, counter: c}
}

type trafficListener struct {
	net.Listener
	counter *TrafficCounter
}

func (l *trafficListener) trafficCounter() *TrafficCounter { return l.counter }

func (l *trafficListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return l.counter.WrapConn(conn), nil
}

func (c *TrafficCounter) WrapListener(listener net.Listener) net.Listener {
	if listener == nil {
		return nil
	}
	if counted, ok := listener.(interface{ trafficCounter() *TrafficCounter }); ok && counted.trafficCounter() == c {
		return listener
	}
	return &trafficListener{Listener: listener, counter: c}
}

var processTraffic = NewTrafficCounter()

func CountTraffic(conn net.Conn) net.Conn { return processTraffic.WrapConn(conn) }

func CountTrafficListener(listener net.Listener) net.Listener {
	return processTraffic.WrapListener(listener)
}

func ProxyTrafficSnapshot() TrafficSnapshot { return processTraffic.Snapshot() }

// UnwrapTrafficConn exposes the original connection for socket operations
// such as SO_ORIGINAL_DST without bypassing accounting in normal I/O paths.
func UnwrapTrafficConn(conn net.Conn) net.Conn {
	for {
		unwrapper, ok := conn.(interface{ Unwrap() net.Conn })
		if !ok {
			return conn
		}
		next := unwrapper.Unwrap()
		if next == nil || next == conn {
			return conn
		}
		conn = next
	}
}
