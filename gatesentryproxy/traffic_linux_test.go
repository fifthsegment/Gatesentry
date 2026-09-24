//go:build linux

package gatesentryproxy

import (
	"io"
	"testing"
)

func TestPrependedBytesAreNotCountedTwice(t *testing.T) {
	counter := NewTrafficCounter()
	base := &trafficTestConn{readData: []byte("hello")}
	counted := counter.WrapConn(base)

	prefix := make([]byte, 2)
	if n, err := counted.Read(prefix); n != 2 || err != nil {
		t.Fatalf("initial read = %d, %v", n, err)
	}
	replayed := &prependConn{Conn: counted, buf: prefix}
	all, err := io.ReadAll(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if string(all) != "hello" {
		t.Fatalf("replayed data = %q", all)
	}
	if got := counter.Snapshot().UploadBytes; got != 5 {
		t.Fatalf("upload bytes = %d, want 5", got)
	}
}
