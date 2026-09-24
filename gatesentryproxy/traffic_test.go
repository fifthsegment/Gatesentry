package gatesentryproxy

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type trafficTestConn struct {
	readData  []byte
	writeN    int
	writeData []byte
}

func (c *trafficTestConn) Read(p []byte) (int, error) {
	if len(c.readData) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.readData)
	c.readData = c.readData[n:]
	return n, nil
}
func (c *trafficTestConn) Write(p []byte) (int, error) {
	n := len(p)
	if c.writeN > 0 && c.writeN < n {
		n = c.writeN
	}
	c.writeData = append(c.writeData, p[:n]...)
	if n < len(p) {
		return n, io.ErrShortWrite
	}
	return n, nil
}
func (*trafficTestConn) Close() error                     { return nil }
func (*trafficTestConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (*trafficTestConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (*trafficTestConn) SetDeadline(time.Time) error      { return nil }
func (*trafficTestConn) SetReadDeadline(time.Time) error  { return nil }
func (*trafficTestConn) SetWriteDeadline(time.Time) error { return nil }

type unwrapTestConn struct{ net.Conn }

func (c *unwrapTestConn) Unwrap() net.Conn { return c.Conn }

type oneConnListener struct {
	conn net.Conn
	done bool
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	if l.done {
		return nil, errors.New("done")
	}
	l.done = true
	return l.conn, nil
}
func (*oneConnListener) Close() error   { return nil }
func (*oneConnListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestTrafficCounterCountsActualBytes(t *testing.T) {
	counter := NewTrafficCounter()
	base := &trafficTestConn{readData: []byte("upload"), writeN: 4}
	conn := counter.WrapConn(base)

	buf := make([]byte, 16)
	if n, err := conn.Read(buf); n != 6 || err != nil {
		t.Fatalf("read = %d, %v", n, err)
	}
	if n, err := conn.Write([]byte("download")); n != 4 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write = %d, %v", n, err)
	}

	snapshot := counter.Snapshot()
	if snapshot.UploadBytes != 6 || snapshot.DownloadBytes != 4 || snapshot.TotalBytes != 10 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.StartedAt.IsZero() || snapshot.StartedAt.Location() != time.UTC {
		t.Fatalf("started_at = %v", snapshot.StartedAt)
	}
}

func TestTrafficWrappingIsIdempotent(t *testing.T) {
	counter := NewTrafficCounter()
	wrapped := counter.WrapConn(&trafficTestConn{})
	if counter.WrapConn(wrapped) != wrapped {
		t.Fatal("wrapped connection was wrapped twice")
	}
	listener := counter.WrapListener(&oneConnListener{conn: &trafficTestConn{}})
	if counter.WrapListener(listener) != listener {
		t.Fatal("wrapped listener was wrapped twice")
	}
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := conn.(*trafficConn); !ok {
		t.Fatalf("accepted connection type = %T", conn)
	}
}

func TestTrafficWrappingIsIdempotentThroughOtherWrappers(t *testing.T) {
	counter := NewTrafficCounter()
	counted := counter.WrapConn(&trafficTestConn{readData: []byte("u")})
	wrapped := &unwrapTestConn{Conn: counted}
	if counter.WrapConn(wrapped) != wrapped {
		t.Fatal("hidden counted connection was wrapped twice")
	}
	_, _ = wrapped.Read(make([]byte, 1))
	if got := counter.Snapshot().UploadBytes; got != 1 {
		t.Fatalf("upload bytes = %d, want 1", got)
	}
}

func TestTrafficWrappingSupportsIndependentCounters(t *testing.T) {
	first := NewTrafficCounter()
	second := NewTrafficCounter()
	conn := second.WrapConn(first.WrapConn(&trafficTestConn{readData: []byte("u")}))

	_, _ = conn.Read(make([]byte, 1))
	_, _ = conn.Write([]byte("d"))

	for name, snapshot := range map[string]TrafficSnapshot{
		"first":  first.Snapshot(),
		"second": second.Snapshot(),
	} {
		if snapshot.UploadBytes != 1 || snapshot.DownloadBytes != 1 {
			t.Fatalf("%s snapshot = %+v", name, snapshot)
		}
	}

	listener := second.WrapListener(first.WrapListener(&oneConnListener{conn: &trafficTestConn{}}))
	if listener == first.WrapListener(listener) {
		t.Fatal("different counters unexpectedly shared a listener wrapper")
	}
}

func TestUnwrapTrafficConn(t *testing.T) {
	counter := NewTrafficCounter()
	base := &trafficTestConn{}
	if got := UnwrapTrafficConn(counter.WrapConn(base)); got != base {
		t.Fatalf("unwrapped connection = %T, want base", got)
	}
}

func TestTrafficCounterConcurrentUpdates(t *testing.T) {
	counter := NewTrafficCounter()
	const workers = 32
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn := counter.WrapConn(&trafficTestConn{readData: []byte("u")})
			_, _ = conn.Read(make([]byte, 1))
			_, _ = conn.Write([]byte("d"))
		}()
	}
	wg.Wait()
	snapshot := counter.Snapshot()
	if snapshot.UploadBytes != workers || snapshot.DownloadBytes != workers || snapshot.TotalBytes != workers*2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
