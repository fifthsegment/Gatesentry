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

// newSmokeProxy returns a GSProxy wired with no-op handlers and the given
// RuleMatchHandler, so tests can exercise the block/allow decision path
// without a real server or policy service.
func newSmokeProxy(ruleHandler func(domain, user, clientIP string) *PolicyDecision) *GSProxy {
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
	p.UrlAccessHandler = func(*GSUrlFilterData) {}
	p.RuleMatchHandler = ruleHandler
	return p
}

// TestProxyBlocksByRuleMatchHandler verifies that when the RuleMatchHandler
// returns a block decision for a domain, the proxy never contacts upstream
// and logs a blocked_url action. This is the core path that policy group
// blocks flow through.
func TestProxyBlocksByRuleMatchHandler(t *testing.T) {
	var loggedAction ProxyAction
	var handlerCalled bool
	p := newSmokeProxy(func(domain, user, clientIP string) *PolicyDecision {
		if domain == "blocked.example" {
			handlerCalled = true
			return &PolicyDecision{Block: true, Reason: "policy Kids"}
		}
		return nil
	})
	p.LogHandler = func(d GSLogData) { loggedAction = d.Action }
	rt := &smokeRoundTripper{}
	h := ProxyHandler{rt: rt, Iproxy: p}
	r := httptest.NewRequest(http.MethodGet, "http://blocked.example/page", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !handlerCalled {
		t.Fatal("RuleMatchHandler was not called for the blocked domain")
	}
	if atomic.LoadInt32(&rt.hits) != 0 {
		t.Fatalf("upstream should not be contacted on block, got hits = %d", rt.hits)
	}
	if loggedAction != ProxyActionBlockedUrl {
		t.Fatalf("logged action = %q, want %q", loggedAction, ProxyActionBlockedUrl)
	}
}

// TestProxyAllowsWhenRuleMatchHandlerReturnsNil verifies the fall-through
// path: when no rule matches (nil return), the proxy forwards to upstream.
func TestProxyAllowsWhenRuleMatchHandlerReturnsNil(t *testing.T) {
	p := newSmokeProxy(func(domain, user, clientIP string) *PolicyDecision { return nil })
	rt := &smokeRoundTripper{}
	h := ProxyHandler{rt: rt, Iproxy: p}
	r := httptest.NewRequest(http.MethodGet, "http://allowed.example/page", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if atomic.LoadInt32(&rt.hits) != 1 {
		t.Fatalf("upstream hits = %d, want 1 on allow", rt.hits)
	}
}

// TestProxyExceptionBypassesBlock simulates the PER-38 exception flow: the
// RuleMatchHandler first returns a block (group policy), then returns nil
// (exception bypasses the block). The proxy must contact upstream on the
// second request. This test exists because PR #170 shipped without any test
// exercising the block-then-bypass path through the proxy, and the feature
// was only caught by manual validation.
func TestProxyExceptionBypassesBlock(t *testing.T) {
	exceptionActive := false
	p := newSmokeProxy(func(domain, user, clientIP string) *PolicyDecision {
		if domain == "blocked.example" && !exceptionActive {
			return &PolicyDecision{Block: true, Reason: "policy Kids"}
		}
		// When the exception is active, the handler returns nil (no match),
		// simulating EvaluateDomain returning ActionAllow which makes
		// policyGroupDecision return (RuleMatch{Matched: false}, true) --
		// the proxy treats handled=false and falls through to upstream.
		return nil
	})
	rt := &smokeRoundTripper{}
	h := ProxyHandler{rt: rt, Iproxy: p}

	// First request: blocked.
	r := httptest.NewRequest(http.MethodGet, "http://blocked.example/page", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if atomic.LoadInt32(&rt.hits) != 0 {
		t.Fatalf("before exception: upstream hits = %d, want 0", rt.hits)
	}

	// Activate the exception (simulate admin creating a scoped exception).
	exceptionActive = true

	// Second request: bypassed.
	r2 := httptest.NewRequest(http.MethodGet, "http://blocked.example/page", nil)
	r2.RemoteAddr = "192.0.2.20:1234"
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r2)
	if atomic.LoadInt32(&rt.hits) != 1 {
		t.Fatalf("after exception: upstream hits = %d, want 1", rt.hits)
	}
}

// TestProxyBlocksOnlyMatchingURLBeforeContactingUpstream verifies that a URL
// condition is tested before the request leaves the gateway: a matching path
// never reaches upstream, and another path on the same host goes through.
func TestProxyBlocksOnlyMatchingURLBeforeContactingUpstream(t *testing.T) {
	var reason string
	p := newSmokeProxy(func(domain, user, clientIP string) *PolicyDecision {
		return &PolicyDecision{Inspect: true, BlockURLRegexes: []string{"/shorts/"}, Reason: "policy Kids: rule No shorts"}
	})
	p.LogHandler = func(d GSLogData) {
		if d.Action == ProxyActionBlockedUrl {
			reason = d.Reason
		}
	}
	rt := &smokeRoundTripper{}
	h := ProxyHandler{rt: rt, Iproxy: p}

	r := httptest.NewRequest(http.MethodGet, "http://video.example/shorts/abc", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	h.ServeHTTP(httptest.NewRecorder(), r)
	if atomic.LoadInt32(&rt.hits) != 0 {
		t.Fatalf("a matching URL reached upstream, hits = %d", rt.hits)
	}
	if reason != "policy Kids: rule No shorts" {
		t.Fatalf("logged reason = %q, want the rule named", reason)
	}

	r = httptest.NewRequest(http.MethodGet, "http://video.example/watch?v=abc", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	h.ServeHTTP(httptest.NewRecorder(), r)
	if atomic.LoadInt32(&rt.hits) != 1 {
		t.Fatalf("a non-matching URL was blocked, hits = %d", rt.hits)
	}
}

// TestProxyBlocksAResponseContentType verifies a response media-type rule.
func TestProxyBlocksAResponseContentType(t *testing.T) {
	p := newSmokeProxy(func(domain, user, clientIP string) *PolicyDecision {
		return &PolicyDecision{Inspect: true, BlockContentTypes: []string{"text/"}, Reason: "policy Kids"}
	})
	var logged ProxyAction
	p.LogHandler = func(d GSLogData) { logged = d.Action }
	h := ProxyHandler{rt: &smokeRoundTripper{}, Iproxy: p}
	r := httptest.NewRequest(http.MethodGet, "http://video.example/file", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	h.ServeHTTP(httptest.NewRecorder(), r)
	if logged != ProxyActionBlockedFileType {
		t.Fatalf("logged action = %q, want the response type blocked", logged)
	}
}

// TestProxyPolicyAllowSkipsTheGatewayURLBlocklist verifies that a policy
// allow is an exception from the gateway-wide URL blocklist too, so an
// administrator can recover a false positive for one policy.
func TestProxyPolicyAllowSkipsTheGatewayURLBlocklist(t *testing.T) {
	p := newSmokeProxy(func(domain, user, clientIP string) *PolicyDecision {
		return &PolicyDecision{Allow: true}
	})
	p.UrlAccessHandler = func(d *GSUrlFilterData) { d.FilterResponseAction = ProxyActionBlockedUrl }
	rt := &smokeRoundTripper{}
	h := ProxyHandler{rt: rt, Iproxy: p}
	r := httptest.NewRequest(http.MethodGet, "http://allowed.example/page", nil)
	r.RemoteAddr = "192.0.2.20:1234"
	h.ServeHTTP(httptest.NewRecorder(), r)
	if atomic.LoadInt32(&rt.hits) != 1 {
		t.Fatalf("upstream hits = %d, want the policy allow to skip the URL blocklist", rt.hits)
	}
}
