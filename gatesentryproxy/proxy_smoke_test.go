package gatesentryproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type smokeRoundTripper struct{ hits int32 }

func (s *smokeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	atomic.AddInt32(&s.hits, 1)
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{"Content-Type": []string{"text/plain"}}, Body: io.NopCloser(http.NoBody), Request: r}, nil
}

func TestProxyProviderRequestRunsThroughURLFilter(t *testing.T) {
	p := NewGSProxy()
	p.IsAuthEnabled = func() bool { return false }
	p.TimeAccessHandler = func(*GSTimeAccessFilterData) {}
	p.ContentTypeHandler = func(*GSContentTypeFilterData) {}
	p.ContentSizeHandler = func(GSContentSizeFilterData) {}
	p.ContentHandler = func(*GSContentFilterData) {}
	p.ProxyErrorHandler = func(*GSProxyErrorData) {}
	p.UserAccessHandler = func(*GSUserAccessFilterData) {}
	p.DoMitm = func(string) bool { return false }
	p.IsExceptionUrl = func(string) bool { return false }
	p.LogHandler = func(GSLogData) {}
	var filtered string
	p.UrlAccessHandler = func(d *GSUrlFilterData) { filtered = d.Url }
	rt := &smokeRoundTripper{}
	h := ProxyHandler{rt: rt, Iproxy: p}
	r := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if filtered != "https://api.openai.com/v1/models" {
		t.Fatalf("URL filter saw %q", filtered)
	}
	if atomic.LoadInt32(&rt.hits) != 1 {
		t.Fatalf("upstream hits = %d", rt.hits)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}
