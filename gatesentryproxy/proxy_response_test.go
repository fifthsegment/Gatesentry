package gatesentryproxy

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

type responseRoundTripFunc func(*http.Request) (*http.Response, error)

func (f responseRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type unreadBody struct {
	read bool
}

func (b *unreadBody) Read([]byte) (int, error) {
	b.read = true
	return 0, errors.New("body must not be read")
}
func (*unreadBody) Close() error { return nil }

func responseTestProxy(contentHandler func(*GSContentFilterData)) *GSProxy {
	p := newSmokeProxy(func(string, string, string) *PolicyDecision { return nil })
	p.ContentHandler = contentHandler
	return p
}

func serveTestResponse(t *testing.T, method string, acceptGzip bool, resp *http.Response, contentHandler func(*GSContentFilterData)) *httptest.ResponseRecorder {
	t.Helper()
	p := responseTestProxy(contentHandler)
	h := ProxyHandler{
		Iproxy: p,
		rt: responseRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			resp.Request = r
			return resp, nil
		}),
	}
	r := httptest.NewRequest(method, "http://example.test/resource", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	if acceptGzip {
		r.Header.Set("Accept-Encoding", "gzip")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAccountingWriterCountsOnlyWrittenBytes(t *testing.T) {
	p := responseTestProxy(func(*GSContentFilterData) {})
	var sizes []int64
	p.ContentSizeHandler = func(data GSContentSizeFilterData) {
		sizes = append(sizes, data.ContentSize)
	}

	base := &trafficTestConn{writeN: 4}
	writer := &accountingWriter{writer: base, contentType: "text/plain", passthru: &GSProxyPassthru{User: "alice"}}
	n, err := writer.Write([]byte("download"))
	if n != 4 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write = %d, %v", n, err)
	}
	if len(sizes) != 1 || sizes[0] != 4 {
		t.Fatalf("accounted sizes = %v, want [4]", sizes)
	}
}

func TestReadBoundedBodyUsesLimitPlusOne(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		wantOver  bool
		wantBytes string
	}{
		{name: "below", body: "1234", wantBytes: "1234"},
		{name: "at limit", body: "12345", wantBytes: "12345"},
		{name: "over", body: "123456789", wantOver: true, wantBytes: "123456"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, over, err := readBoundedBody(bytes.NewBufferString(tc.body), 5)
			if err != nil {
				t.Fatal(err)
			}
			if over != tc.wantOver || string(got) != tc.wantBytes {
				t.Fatalf("read = %q, over = %v; want %q, %v", got, over, tc.wantBytes, tc.wantOver)
			}
		})
	}
}

func TestExactlyAtLimitResponseScansOnce(t *testing.T) {
	oldLimit := MaxContentScanSize
	MaxContentScanSize = 5
	defer func() { MaxContentScanSize = oldLimit }()

	body := []byte("12345")
	scans := 0
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"text/html"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: -1,
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(data *GSContentFilterData) {
		scans++
		if !bytes.Equal(data.Content, body) {
			t.Fatalf("scanner body = %q, want %q", data.Content, body)
		}
	})
	if scans != 1 {
		t.Fatalf("content scans = %d, want 1", scans)
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatalf("body = %q, want %q", w.Body.Bytes(), body)
	}
}

func TestUnknownOversizedResponsePreservesOrderAndSkipsScanning(t *testing.T) {
	oldLimit := MaxContentScanSize
	MaxContentScanSize = 5
	defer func() { MaxContentScanSize = oldLimit }()

	body := []byte("prefix-and-remainder")
	scans := 0
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"text/html"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: -1,
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(*GSContentFilterData) { scans++ })
	if scans != 0 {
		t.Fatalf("content scans = %d, want 0", scans)
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatalf("body = %q, want %q", w.Body.Bytes(), body)
	}
}

func TestKnownOversizedResponseStreamsWithoutScanning(t *testing.T) {
	oldLimit := MaxContentScanSize
	MaxContentScanSize = 5
	defer func() { MaxContentScanSize = oldLimit }()

	body := []byte("known-oversized")
	scans := 0
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"text/html"},
			"Content-Length": []string{strconv.Itoa(len(body))},
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(*GSContentFilterData) { scans++ })
	if scans != 0 {
		t.Fatalf("content scans = %d, want 0", scans)
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatalf("body = %q, want %q", w.Body.Bytes(), body)
	}
	if got := w.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length = %q", got)
	}
}

func TestBoundedImagePassesFullBodyToContentHandler(t *testing.T) {
	oldLimit := MaxContentScanSize
	MaxContentScanSize = 1024
	defer func() { MaxContentScanSize = oldLimit }()

	body := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x5a}, 300)...)
	var scanned []byte
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(data *GSContentFilterData) {
		scanned = append([]byte(nil), data.Content...)
	})
	if !bytes.Equal(scanned, body) {
		t.Fatalf("scanner received %d bytes, want %d", len(scanned), len(body))
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatal("forwarded image differs from upstream body")
	}
}

func TestBlockedMediaReplacementUpdatesRepresentationHeaders(t *testing.T) {
	body := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x5a}, 64)...)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":   []string{"image/png"},
			"Content-Length": []string{strconv.Itoa(len(body))},
			"Content-Range":  []string{"bytes 0-9/20"},
			"Accept-Ranges":  []string{"bytes"},
			"ETag":           []string{`"original"`},
			"Digest":         []string{"sha-256=original"},
			"Content-MD5":    []string{"original"},
			"Last-Modified":  []string{"Wed, 21 Oct 2015 07:28:00 GMT"},
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(data *GSContentFilterData) {
		data.FilterResponseAction = ProxyActionBlockedMediaContent
		data.FilterResponse = []byte(`["test reason"]`)
	})
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	for _, key := range []string{"Content-Range", "Accept-Ranges", "ETag", "Digest", "Content-MD5", "Last-Modified"} {
		if got := w.Header().Get(key); got != "" {
			t.Fatalf("%s = %q, want empty", key, got)
		}
	}
	if got := w.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	if got := w.Header().Get("Content-Length"); got != strconv.Itoa(w.Body.Len()) {
		t.Fatalf("Content-Length = %q, body length = %d", got, w.Body.Len())
	}
	if bytes.Equal(w.Body.Bytes(), body) {
		t.Fatal("blocked media body was not replaced")
	}
}

func TestBodylessResponsesDoNotReadOrWriteBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		status int
	}{
		{name: "head", method: http.MethodHead, status: http.StatusOK},
		{name: "informational", method: http.MethodGet, status: http.StatusEarlyHints},
		{name: "no content", method: http.MethodGet, status: http.StatusNoContent},
		{name: "not modified", method: http.MethodGet, status: http.StatusNotModified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &unreadBody{}
			resp := &http.Response{
				StatusCode:    tc.status,
				Header:        http.Header{"Content-Type": []string{"text/html"}},
				Body:          body,
				ContentLength: 9,
			}
			w := serveTestResponse(t, tc.method, false, resp, func(*GSContentFilterData) {
				t.Fatal("bodyless response was scanned")
			})
			if body.read {
				t.Fatal("bodyless response body was read")
			}
			if w.Body.Len() != 0 {
				t.Fatalf("body length = %d, want 0", w.Body.Len())
			}
		})
	}
}

func TestBodylessResponseSkipsContentTypeBlock(t *testing.T) {
	p := newSmokeProxy(func(string, string, string) *PolicyDecision {
		return &PolicyDecision{Inspect: true, BlockContentTypes: []string{"text/"}, Reason: "blocked"}
	})
	body := &unreadBody{}
	h := ProxyHandler{
		Iproxy: p,
		rt: responseRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusNoContent,
				Header:        http.Header{"Content-Type": []string{"text/plain"}},
				Body:          body,
				ContentLength: 9,
				Request:       r,
			}, nil
		}),
	}
	r := httptest.NewRequest(http.MethodGet, "http://example.test/resource", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent || w.Body.Len() != 0 || body.read {
		t.Fatalf("status = %d, body length = %d, upstream read = %v", w.Code, w.Body.Len(), body.read)
	}
}

func TestGzipQZeroIsNotAccepted(t *testing.T) {
	if acceptsEncoding("br, gzip;q=0", "gzip") {
		t.Fatal("gzip;q=0 was accepted")
	}
	if !acceptsEncoding("br, gzip;q=0.5", "gzip") {
		t.Fatal("gzip;q=0.5 was rejected")
	}
}

func TestGzipHeaderOnlyWhenResponseIsCompressed(t *testing.T) {
	oldLimit := MaxContentScanSize
	MaxContentScanSize = 5
	defer func() { MaxContentScanSize = oldLimit }()

	body := []byte("unknown-length-response")
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"text/html"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: -1,
	}
	w := serveTestResponse(t, http.MethodGet, true, resp, func(*GSContentFilterData) {})
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	zr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, body) {
		t.Fatalf("decoded body = %q, want %q", decoded, body)
	}
}

func BenchmarkAccountingWriter16MiB(b *testing.B) {
	p := responseTestProxy(func(*GSContentFilterData) {})
	p.ContentSizeHandler = func(GSContentSizeFilterData) {}
	payload := bytes.Repeat([]byte{'x'}, 16<<20)
	passthru := &GSProxyPassthru{User: "benchmark"}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer := &accountingWriter{writer: io.Discard, contentType: "application/octet-stream", passthru: passthru}
		_, _ = io.Copy(writer, bytes.NewReader(payload))
	}
}

type discardResponseWriter struct{ header http.Header }

func (w *discardResponseWriter) Header() http.Header       { return w.header }
func (*discardResponseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (*discardResponseWriter) WriteHeader(statusCode int)  {}

func BenchmarkResponseKnownOversized16MiB(b *testing.B) {
	p := responseTestProxy(func(*GSContentFilterData) {})
	p.ContentSizeHandler = func(GSContentSizeFilterData) {}
	payload := bytes.Repeat([]byte{'x'}, 16<<20)
	passthru := &GSProxyPassthru{User: "benchmark"}
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := &discardResponseWriter{header: make(http.Header)}
		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: int64(len(payload))}
		_ = writeResponseStream(w, resp, "application/octet-stream", passthru, nil, bytes.NewReader(payload), false)
	}
}

func BenchmarkResponseUnknownOversized16MiB(b *testing.B) {
	p := responseTestProxy(func(*GSContentFilterData) {})
	p.ContentSizeHandler = func(GSContentSizeFilterData) {}
	payload := bytes.Repeat([]byte{'x'}, 16<<20)
	passthru := &GSProxyPassthru{User: "benchmark"}
	const scanLimit = 64 << 10
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := bytes.NewReader(payload)
		prefix, over, _ := readBoundedBody(body, scanLimit)
		if !over {
			b.Fatal("benchmark payload did not exceed scan limit")
		}
		w := &discardResponseWriter{header: make(http.Header)}
		resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: -1}
		_ = writeResponseStream(w, resp, "application/octet-stream", passthru, prefix, body, false)
	}
}

type errorAfterDataBody struct {
	data []byte
	done bool
}

func (b *errorAfterDataBody) Read(p []byte) (int, error) {
	if b.done {
		return 0, io.EOF
	}
	b.done = true
	return copy(p, b.data), errors.New("upstream read failed")
}
func (*errorAfterDataBody) Close() error { return nil }

func TestAcceptsEncodingExactTokenOverridesWildcard(t *testing.T) {
	for _, tc := range []struct {
		header string
		want   bool
	}{
		{header: "*;q=1, gzip;q=0", want: false},
		{header: "*;q=0, gzip;q=1", want: true},
		{header: "br, *;q=0.5", want: true},
	} {
		if got := acceptsEncoding(tc.header, "gzip"); got != tc.want {
			t.Errorf("acceptsEncoding(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

func TestPartialContentIsNotDynamicallyCompressed(t *testing.T) {
	body := bytes.Repeat([]byte("range-data"), 200)
	resp := &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Content-Type":  []string{"text/html"},
			"Content-Range": []string{"bytes 0-1999/4000"},
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	w := serveTestResponse(t, http.MethodGet, true, resp, func(*GSContentFilterData) {})
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want identity", got)
	}
	if got := w.Header().Get("Content-Range"); got != "bytes 0-1999/4000" {
		t.Fatalf("Content-Range = %q", got)
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatal("partial response body changed")
	}
}

func TestNotModifiedPreservesMetadataContentLength(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNotModified,
		Header: http.Header{
			"Content-Length": []string{"4321"},
			"ETag":           []string{`"cached"`},
		},
		Body:          &unreadBody{},
		ContentLength: 0,
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(*GSContentFilterData) {
		t.Fatal("304 was scanned")
	})
	if got := w.Header().Get("Content-Length"); got != "4321" {
		t.Fatalf("Content-Length = %q, want 4321", got)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("304 body length = %d", w.Body.Len())
	}
}

func TestEncodedResponseStreamsWithoutScanning(t *testing.T) {
	body := []byte("opaque-brotli-representation")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":     []string{"text/html"},
			"Content-Encoding": []string{"br"},
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	w := serveTestResponse(t, http.MethodGet, true, resp, func(*GSContentFilterData) {
		t.Fatal("encoded response was scanned")
	})
	if got := w.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("Content-Encoding = %q, want br", got)
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatal("encoded response body changed")
	}
}

func TestBoundedReadErrorReturnsBadGatewayWithoutPartialBody(t *testing.T) {
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"text/html"}},
		Body:          &errorAfterDataBody{data: []byte("partial upstream body")},
		ContentLength: -1,
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(*GSContentFilterData) {
		t.Fatal("partial response was scanned")
	})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("partial upstream body")) {
		t.Fatal("partial upstream body was emitted")
	}
}

func TestResponseHopByHopHeadersAreStripped(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":       []string{"text/plain"},
			"Connection":         []string{"keep-alive, X-Private"},
			"Keep-Alive":         []string{"timeout=5"},
			"Proxy-Authenticate": []string{"Basic"},
			"X-Private":          []string{"secret"},
		},
		Body:          io.NopCloser(bytes.NewReader([]byte("ok"))),
		ContentLength: 2,
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(*GSContentFilterData) {})
	for _, key := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "X-Private"} {
		if got := w.Header().Get(key); got != "" {
			t.Errorf("%s = %q, want empty", key, got)
		}
	}
}

func TestUnsolicitedSwitchingProtocolsReturnsBadGateway(t *testing.T) {
	body := &unreadBody{}
	resp := &http.Response{
		StatusCode:    http.StatusSwitchingProtocols,
		Header:        http.Header{"Connection": []string{"Upgrade"}, "Upgrade": []string{"other"}},
		Body:          body,
		ContentLength: -1,
	}
	w := serveTestResponse(t, http.MethodGet, false, resp, func(*GSContentFilterData) {
		t.Fatal("101 response was scanned")
	})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if body.read {
		t.Fatal("101 response body was read")
	}
}

func TestBlockedMediaReasonsAcceptPlainText(t *testing.T) {
	got := blockedMediaReasons([]byte("adult content"))
	want := []string{"", "Image blocked by Gatesentry", "Reason for blocking", "adult content"}
	if len(got) != len(want) {
		t.Fatalf("reasons = %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reasons = %q, want %q", got, want)
		}
	}
}

type gatedBody struct {
	first   []byte
	release <-chan struct{}
	step    int
}

func (b *gatedBody) Read(p []byte) (int, error) {
	switch b.step {
	case 0:
		b.step++
		return copy(p, b.first), nil
	case 1:
		b.step++
		<-b.release
		return 0, io.EOF
	default:
		return 0, io.EOF
	}
}
func (*gatedBody) Close() error { return nil }

type signalingResponseWriter struct {
	*httptest.ResponseRecorder
	wrote chan struct{}
	once  sync.Once
}

func (w *signalingResponseWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(p)
	w.once.Do(func() { close(w.wrote) })
	return n, err
}

func TestNonInspectableResponseStreamsBeforeUpstreamEOF(t *testing.T) {
	release := make(chan struct{})
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          &gatedBody{first: []byte(`{"ready":true}`), release: release},
		ContentLength: -1,
	}
	p := responseTestProxy(func(*GSContentFilterData) { t.Error("non-inspectable response was scanned") })
	h := ProxyHandler{Iproxy: p, rt: responseRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		resp.Request = r
		return resp, nil
	})}
	r := httptest.NewRequest(http.MethodGet, "http://example.test/data.json", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	w := &signalingResponseWriter{ResponseRecorder: httptest.NewRecorder(), wrote: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(w, r)
		close(done)
	}()
	select {
	case <-w.wrote:
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("response did not stream before upstream EOF")
	}
	close(release)
	<-done
	if got := w.Body.String(); got != `{"ready":true}` {
		t.Fatalf("body = %q", got)
	}
}

func TestRequestRangeDisablesDynamicCompression(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://example.test/resource", nil)
	r.Header.Set("Range", "bytes=0-99")
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	if canDynamicCompress(r, resp, true) {
		t.Fatal("range request was eligible for dynamic compression")
	}
}
