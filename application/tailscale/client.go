// Package tailscale provides read-only access to tailscaled's LocalAPI and a
// small lifecycle manager for keeping a snapshot of tailnet peers.
package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// SocketEnv overrides the standard tailscaled socket locations.
	SocketEnv = "GATESENTRY_TAILSCALE_SOCKET"

	// StatusPath is the only LocalAPI endpoint used by Client.
	StatusPath = "/localapi/v0/status"

	defaultRequestTimeout = 5 * time.Second
	maxStatusResponseSize = 1 << 20
)

var defaultSocketPaths = []string{
	"/var/run/tailscale/tailscaled.sock",
	"/run/tailscale/tailscaled.sock",
}

// Peer is the subset of a tailscaled PeerStatus needed by GateSentry. NodeID is
// the stable PeerStatus.ID; map keys, addresses, and names are never identities.
type Peer struct {
	NodeID    string    `json:"node_id"`
	Name      string    `json:"name,omitempty"`
	DNSName   string    `json:"dns_name,omitempty"`
	Addresses []string  `json:"addresses,omitempty"`
	Online    bool      `json:"online"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
	WoLMACs   []string  `json:"wol_macs,omitempty"`
}

// LocalStatus is the minimal, normalized status returned by Client.
type LocalStatus struct {
	BackendState string `json:"backend_state,omitempty"`
	Self         *Peer  `json:"self,omitempty"`
	Peers        []Peer `json:"peers,omitempty"`
}

// Client is a dependency-free, read-only tailscaled LocalAPI client.
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient creates a client that discovers tailscaled's socket for each
// request. This allows tailscaled to start or restart after the client is made.
func NewClient() *Client {
	return &Client{timeout: defaultRequestTimeout}
}

// NewClientWithSocket creates a client pinned to socketPath. It is useful for
// non-standard installations and tests.
func NewClientWithSocket(socketPath string) *Client {
	return &Client{socketPath: strings.TrimSpace(socketPath), timeout: defaultRequestTimeout}
}

// DetectSocket returns the first available tailscaled Unix socket. A client
// explicitly pinned to a socket uses only that socket. Otherwise SocketEnv
// takes precedence over the standard locations.
func (c *Client) DetectSocket() (string, bool) {
	for _, path := range c.socketCandidates() {
		info, err := os.Stat(path)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			return path, true
		}
	}
	return "", false
}

// Detect reports whether a tailscaled Unix socket is currently available. It
// only stats candidate paths and never contacts LocalAPI.
func (c *Client) Detect() bool {
	_, ok := c.DetectSocket()
	return ok
}

// Status performs a bounded, context-aware GET of tailscaled's status.
func (c *Client) Status(ctx context.Context) (LocalStatus, error) {
	if ctx == nil {
		return LocalStatus{}, errors.New("tailscale: nil context")
	}

	socketPath, ok := c.DetectSocket()
	if !ok {
		return LocalStatus{}, errors.New("tailscale: socket unavailable")
	}

	timeout := c.timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "http://local-tailscaled.sock"+StatusPath, nil)
	if err != nil {
		return LocalStatus{}, fmt.Errorf("tailscale: create status request: %w", err)
	}
	request.Host = "local-tailscaled.sock"
	request.Header.Set("Accept", "application/json")

	httpClient := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("tailscale: redirects are not allowed")
		},
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return LocalStatus{}, fmt.Errorf("tailscale: status request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return LocalStatus{}, fmt.Errorf("tailscale: status request returned HTTP %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxStatusResponseSize+1))
	if err != nil {
		return LocalStatus{}, fmt.Errorf("tailscale: read status response: %w", err)
	}
	if len(body) > maxStatusResponseSize {
		return LocalStatus{}, errors.New("tailscale: status response too large")
	}

	var wire statusResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return LocalStatus{}, errors.New("tailscale: invalid status response")
	}
	status, err := normalizeStatus(wire)
	if err != nil {
		return LocalStatus{}, err
	}
	return status, nil
}

// GetStatus is an alias for Status.
func (c *Client) GetStatus(ctx context.Context) (LocalStatus, error) {
	return c.Status(ctx)
}

func (c *Client) socketCandidates() []string {
	if c != nil && c.socketPath != "" {
		return []string{c.socketPath}
	}
	candidates := make([]string, 0, len(defaultSocketPaths)+1)
	if path := strings.TrimSpace(os.Getenv(SocketEnv)); path != "" {
		candidates = append(candidates, path)
	}
	candidates = append(candidates, defaultSocketPaths...)
	return candidates
}

type statusResponse struct {
	BackendState string                 `json:"BackendState"`
	Self         *peerStatus            `json:"Self"`
	Peer         map[string]*peerStatus `json:"Peer"`
}

type peerStatus struct {
	ID           string    `json:"ID"`
	HostName     string    `json:"HostName"`
	DNSName      string    `json:"DNSName"`
	TailscaleIPs []string  `json:"TailscaleIPs"`
	Online       bool      `json:"Online"`
	Active       bool      `json:"Active"`
	LastSeen     time.Time `json:"LastSeen"`
	WoLMACs      []string  `json:"WoLMACs"`
}

func normalizeStatus(wire statusResponse) (LocalStatus, error) {
	status := LocalStatus{BackendState: strings.TrimSpace(wire.BackendState)}
	if wire.Self != nil {
		peer := normalizePeer(*wire.Self)
		if peer.NodeID != "" {
			status.Self = &peer
		}
	}
	status.Peers = make([]Peer, 0, len(wire.Peer))
	seen := make(map[string]struct{}, len(wire.Peer))
	for _, candidate := range wire.Peer {
		if candidate == nil || strings.TrimSpace(candidate.ID) == "" {
			continue
		}
		peer := normalizePeer(*candidate)
		if _, duplicate := seen[peer.NodeID]; duplicate {
			return LocalStatus{}, errors.New("tailscale: invalid status response")
		}
		seen[peer.NodeID] = struct{}{}
		status.Peers = append(status.Peers, peer)
	}
	sortPeers(status.Peers)
	return status, nil
}

func normalizePeer(wire peerStatus) Peer {
	return Peer{
		NodeID:    strings.TrimSpace(wire.ID),
		Name:      strings.TrimSpace(wire.HostName),
		DNSName:   strings.TrimSuffix(strings.TrimSpace(wire.DNSName), "."),
		Addresses: cloneStrings(wire.TailscaleIPs),
		Online:    wire.Online,
		LastSeen:  wire.LastSeen,
		WoLMACs:   cloneStrings(wire.WoLMACs),
	}
}

func clonePeer(peer Peer) Peer {
	peer.Addresses = cloneStrings(peer.Addresses)
	peer.WoLMACs = cloneStrings(peer.WoLMACs)
	return peer
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
