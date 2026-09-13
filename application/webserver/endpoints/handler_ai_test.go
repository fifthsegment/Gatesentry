package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

func setupAIProbeStore(t *testing.T) (*gatesentry2storage.MapStore, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "aiprobe-*")
	if err != nil {
		t.Fatal(err)
	}
	orig := gatesentry2storage.GSBASEDIR
	gatesentry2storage.SetBaseDir(tmpDir + "/")
	store := gatesentry2storage.NewMapStore("test_ai_probe", false)
	return store, func() {
		gatesentry2storage.SetBaseDir(orig)
		os.RemoveAll(tmpDir)
	}
}

func TestProbeAIStatusNotConfigured(t *testing.T) {
	store, cleanup := setupAIProbeStore(t)
	defer cleanup()

	got := ProbeAIStatus(store, defaultAIProbeClient())
	if got.Mode != "disabled" {
		t.Fatalf("mode = %q", got.Mode)
	}
	if got.Grok.State != aiProbeStateNotConfigured {
		t.Fatalf("grok = %+v", got.Grok)
	}
	if got.ChatGPT.State != aiProbeStateNotConfigured {
		t.Fatalf("chatgpt = %+v", got.ChatGPT)
	}
	if got.Local.State != aiProbeStateNotConfigured {
		t.Fatalf("local = %+v", got.Local)
	}
	if got.Legacy.State != aiProbeStateNotConfigured {
		t.Fatalf("legacy = %+v", got.Legacy)
	}
}

func TestProbeBearerModelsStates(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer okSrv.Close()

	client := defaultAIProbeClient()
	if st := probeBearerModels(client, okSrv.URL, ""); st.State != aiProbeStateNotConfigured {
		t.Fatalf("empty key: %+v", st)
	}
	if st := probeBearerModels(client, okSrv.URL, "good-key"); st.State != aiProbeStateOK {
		t.Fatalf("good key: %+v", st)
	}
	if st := probeBearerModels(client, okSrv.URL, "bad-key"); st.State != aiProbeStateUnauthorized {
		t.Fatalf("bad key: %+v", st)
	}
}

func TestProbeOllamaAndLegacy(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer ollama.Close()

	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	defer legacy.Close()

	client := defaultAIProbeClient()
	if st := probeOllama(client, ollama.URL); st.State != aiProbeStateOK {
		t.Fatalf("ollama: %+v", st)
	}
	if st := probeHTTPReachable(client, legacy.URL); st.State != aiProbeStateOK {
		t.Fatalf("legacy 405 should still be reachable: %+v", st)
	}
	if st := probeOllama(client, "not-a-url"); st.State != aiProbeStateUnreachable {
		t.Fatalf("bad ollama url: %+v", st)
	}
}

func TestGSApiAIStatusGETJSON(t *testing.T) {
	store, cleanup := setupAIProbeStore(t)
	defer cleanup()
	store.Update("ai_image_filtering_mode", "local")

	rec := httptest.NewRecorder()
	GSApiAIStatusGET(rec, httptest.NewRequest(http.MethodGet, "/api/ai/status", nil), store)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	var body AIStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Mode != "local" {
		t.Fatalf("mode %q", body.Mode)
	}
}

func TestProbeDNSResolverSkipsLoopback(t *testing.T) {
	if got := probeDNSResolver(""); got != aiProbePublicDNS {
		t.Fatalf("empty = %q", got)
	}
	if got := probeDNSResolver("127.0.0.1:53"); got != aiProbePublicDNS {
		t.Fatalf("loopback = %q", got)
	}
	if got := probeDNSResolver("8.8.8.8:53"); got != "8.8.8.8:53" {
		t.Fatalf("public = %q", got)
	}
}

func TestProbeBearerTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()
	client := newDirectOutboundClient(aiProbePublicDNS, 200*time.Millisecond)
	start := time.Now()
	st := probeBearerModels(client, srv.URL, "key")
	if time.Since(start) > time.Second {
		t.Fatal("probe did not time out")
	}
	if st.State != aiProbeStateUnreachable {
		t.Fatalf("got %+v", st)
	}
}

func TestOllamaTagsURL(t *testing.T) {
	got, err := ollamaTagsURL("http://127.0.0.1:11434")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:11434/api/tags" {
		t.Fatalf("got %q", got)
	}
	got, err = ollamaTagsURL("http://127.0.0.1:11434/api/tags")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:11434/api/tags" {
		t.Fatalf("already tags: %q", got)
	}
}
