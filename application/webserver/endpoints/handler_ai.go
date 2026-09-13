package gatesentryWebserverEndpoints

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
)

const (
	aiProbeStateOK            = "ok"
	aiProbeStateUnauthorized  = "unauthorized"
	aiProbeStateUnreachable   = "unreachable"
	aiProbeStateNotConfigured = "not_configured"
	aiGrokModelsURL           = "https://api.x.ai/v1/models"
	aiOpenAIModelsURL         = "https://api.openai.com/v1/models"
	aiProbeTimeout            = 4 * time.Second
	aiProbePublicDNS          = "8.8.8.8:53"
)

// AIEndpointStatus is the reachability of one configured AI backend.
type AIEndpointStatus struct {
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// AIStatusResponse is GET /api/ai/status.
type AIStatusResponse struct {
	Mode    string           `json:"mode"`
	Grok    AIEndpointStatus `json:"grok"`
	ChatGPT AIEndpointStatus `json:"chatgpt"`
	Local   AIEndpointStatus `json:"local"`
	Legacy  AIEndpointStatus `json:"legacy"`
}

func defaultAIProbeClient() *http.Client {
	return newDirectOutboundClient(aiProbePublicDNS, aiProbeTimeout)
}

// newDirectOutboundClient never uses HTTP_PROXY/HTTPS_PROXY and does not
// resolve through GateSentry DNS (loopback). Probes must not be filterable
// by this process's own proxy or blocklists.
func newDirectOutboundClient(resolverAddr string, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = aiProbeTimeout
	}
	resolverAddr = probeDNSResolver(resolverAddr)
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: -1,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: timeout}
				return d.DialContext(ctx, network, resolverAddr)
			},
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return nil, nil // never inherit HTTP_PROXY
			},
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
		},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func probeDNSResolver(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return aiProbePublicDNS
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		host = raw
		port = "53"
	}
	ip := net.ParseIP(host)
	if strings.EqualFold(host, "localhost") || (ip != nil && (ip.IsLoopback() || ip.IsUnspecified())) {
		return aiProbePublicDNS
	}
	return net.JoinHostPort(host, port)
}

// GSApiAIStatusGET probes Grok, OpenAI, Ollama, and the legacy scanner using
// stored settings. Keys are not returned.
func GSApiAIStatusGET(w http.ResponseWriter, _ *http.Request, settings *gatesentry2storage.MapStore) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ProbeAIStatus(settings, nil))
}

// ProbeAIStatus checks each configured endpoint. client may be nil (uses a
// direct outbound client that bypasses HTTP_PROXY and local DNS).
func ProbeAIStatus(settings *gatesentry2storage.MapStore, client *http.Client) AIStatusResponse {
	if client == nil {
		resolver := ""
		if settings != nil {
			resolver = settings.Get("dns_resolver")
		}
		client = newDirectOutboundClient(resolver, aiProbeTimeout)
	}
	mode := strings.ToLower(strings.TrimSpace(settings.Get("ai_image_filtering_mode")))
	if mode == "" {
		mode = "disabled"
	}
	out := AIStatusResponse{Mode: mode}
	var grok, chatgpt, local, legacy AIEndpointStatus
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		grok = probeBearerModels(client, aiGrokModelsURL, settings.Get("ai_grok_api_key"))
	}()
	go func() {
		defer wg.Done()
		chatgpt = probeBearerModels(client, aiOpenAIModelsURL, settings.Get("ai_openai_api_key"))
	}()
	go func() {
		defer wg.Done()
		local = probeOllama(client, settings.Get("ai_local_llm_url"))
	}()
	go func() {
		defer wg.Done()
		legacy = probeHTTPReachable(client, settings.Get("ai_scanner_url"))
	}()
	wg.Wait()
	out.Grok = grok
	out.ChatGPT = chatgpt
	out.Local = local
	out.Legacy = legacy
	return out
}

func probeBearerModels(client *http.Client, modelsURL, apiKey string) AIEndpointStatus {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return AIEndpointStatus{State: aiProbeStateNotConfigured, Detail: "No API key saved"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "Unable to build request"}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("User-Agent", "gatesentry-ai-probe")
	resp, err := client.Do(req)
	if err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "Endpoint not reachable"}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return classifyAuthHTTP(resp.StatusCode)
}

func probeOllama(client *http.Client, raw string) AIEndpointStatus {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AIEndpointStatus{State: aiProbeStateNotConfigured, Detail: "No Ollama URL saved"}
	}
	tagsURL, err := ollamaTagsURL(raw)
	if err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tagsURL, nil)
	if err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "Unable to build request"}
	}
	req.Header.Set("User-Agent", "gatesentry-ai-probe")
	resp, err := client.Do(req)
	if err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "Endpoint not reachable"}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return classifyAuthHTTP(resp.StatusCode)
}

func probeHTTPReachable(client *http.Client, raw string) AIEndpointStatus {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return AIEndpointStatus{State: aiProbeStateNotConfigured, Detail: "No scanner URL saved"}
	}
	if _, err := validateProbeURL(raw); err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "Unable to build request"}
	}
	req.Header.Set("User-Agent", "gatesentry-ai-probe")
	resp, err := client.Do(req)
	if err != nil {
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "Endpoint not reachable"}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	// Any HTTP response means the host answered (POST-only scanners often return 405).
	if resp.StatusCode > 0 {
		return AIEndpointStatus{State: aiProbeStateOK, Detail: "HTTP " + resp.Status}
	}
	return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "No HTTP status"}
}

func classifyAuthHTTP(code int) AIEndpointStatus {
	switch {
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return AIEndpointStatus{State: aiProbeStateUnauthorized, Detail: "API key rejected"}
	case code >= 200 && code < 300:
		return AIEndpointStatus{State: aiProbeStateOK, Detail: "HTTP " + http.StatusText(code)}
	case code == http.StatusTooManyRequests || code == http.StatusPaymentRequired:
		return AIEndpointStatus{State: aiProbeStateOK, Detail: "Authenticated (HTTP " + http.StatusText(code) + ")"}
	default:
		return AIEndpointStatus{State: aiProbeStateUnreachable, Detail: "HTTP " + http.StatusText(code)}
	}
}

func ollamaTagsURL(raw string) (string, error) {
	u, err := validateProbeURL(raw)
	if err != nil {
		return "", err
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" || path == "/" {
		u.Path = "/api/tags"
	} else if !strings.HasSuffix(path, "/api/tags") {
		u.Path = path + "/api/tags"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func validateProbeURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errInvalidProbeURL
	}
	return u, nil
}

var errInvalidProbeURL = errString("URL must be http or https with a host")

type errString string

func (e errString) Error() string { return string(e) }
