package tailscale

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

const defaultPollInterval = 60 * time.Second

var errManagerStopped = errors.New("tailscale: manager stopped")

// ManagerStatus is a safe, finite state suitable for exposing through an API.
// It intentionally contains no raw LocalAPI errors or socket paths.
type ManagerStatus string

const (
	StatusDisabled    ManagerStatus = "disabled"
	StatusUnavailable ManagerStatus = "unavailable"
	StatusConnecting  ManagerStatus = "connecting"
	StatusConnected   ManagerStatus = "connected"
	StatusDegraded    ManagerStatus = "degraded"
	StatusStopped     ManagerStatus = "stopped"
)

// Applier consumes complete successful peer snapshots. Clearing observations
// removes runtime addresses while preserving durable user-created links.
// Implementations must not synchronously call Manager lifecycle methods from
// these callbacks; lifecycle callers wait for callbacks to finish.
type Applier interface {
	ApplyTailscaleSnapshot([]Peer) error
	ClearTailscaleObservations()
}

// ManagerSnapshot is a point-in-time, mutation-safe view of Manager state.
type ManagerSnapshot struct {
	Enabled     bool          `json:"enabled"`
	Detected    bool          `json:"detected"`
	Status      ManagerStatus `json:"status"`
	LastRefresh time.Time     `json:"last_refresh,omitempty"`
	Self        *Peer         `json:"self,omitempty"`
	Peers       []Peer        `json:"peers,omitempty"`
}

// Manager detects tailscaled and periodically applies complete peer snapshots.
type Manager struct {
	client   *Client
	applier  Applier
	interval time.Duration

	mu             sync.RWMutex
	enabled        bool
	detected       bool
	status         ManagerStatus
	lastRefresh    time.Time
	self           *Peer
	peers          map[string]Peer
	stopped        bool
	activeCancel   context.CancelFunc
	activeSequence uint64

	transitionSequence uint64
	transitionToken    chan struct{}
	stop               chan struct{}
	done               chan struct{}
	stopOnce           sync.Once
}

// NewManager creates a disabled manager and starts its detection/poll loop.
// A non-positive interval uses the 60-second default.
func NewManager(client *Client, applier Applier, interval time.Duration) *Manager {
	if client == nil {
		client = NewClient()
	}
	if interval <= 0 {
		interval = defaultPollInterval
	}
	manager := &Manager{
		client:          client,
		applier:         applier,
		interval:        interval,
		detected:        client.Detect(),
		status:          StatusDisabled,
		peers:           make(map[string]Peer),
		transitionToken: make(chan struct{}, 1),
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
	}
	manager.transitionToken <- struct{}{}
	go manager.run()
	return manager
}

// Detect refreshes and returns socket availability without querying LocalAPI.
func (m *Manager) Detect() bool {
	detected := m.client.Detect()
	m.mu.Lock()
	m.detected = detected
	if m.enabled && !detected && m.status != StatusStopped {
		m.status = m.failureStatusLocked()
	}
	m.mu.Unlock()
	return detected
}

// Snapshot returns an isolated copy of all public manager state.
func (m *Manager) Snapshot() ManagerSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := ManagerSnapshot{
		Enabled:     m.enabled,
		Detected:    m.detected,
		Status:      m.status,
		LastRefresh: m.lastRefresh,
		Peers:       peersFromMap(m.peers),
	}
	if m.self != nil {
		self := clonePeer(*m.self)
		snapshot.Self = &self
	}
	return snapshot
}

// Peers returns an isolated, stable-order copy of the last successful peers.
func (m *Manager) Peers() []Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return peersFromMap(m.peers)
}

// Peer looks up a peer by its stable PeerStatus.ID.
func (m *Manager) Peer(nodeID string) (Peer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	peer, ok := m.peers[nodeID]
	if !ok {
		return Peer{}, false
	}
	return clonePeer(peer), true
}

// SetEnabled changes collection state. Transitions are ordered when they are
// requested, and a transition superseded while waiting does not overwrite the
// later request. Enabling performs an immediate refresh; disabling clears
// applied runtime observations.
func (m *Manager) SetEnabled(ctx context.Context, enabled bool) error {
	if ctx == nil {
		return errors.New("tailscale: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return errManagerStopped
	}
	m.transitionSequence++
	sequence := m.transitionSequence
	cancel := m.activeCancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	if err := m.lockTransition(ctx); err != nil {
		return err
	}
	defer m.unlockTransition()
	if err := ctx.Err(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return errManagerStopped
	}
	if sequence != m.transitionSequence {
		m.mu.Unlock()
		return nil
	}
	if !enabled {
		m.enabled = false
		m.status = StatusDisabled
		m.self = nil
		m.peers = make(map[string]Peer)
		m.mu.Unlock()
		if m.applier != nil {
			m.applier.ClearTailscaleObservations()
		}
		return nil
	}

	m.enabled = true
	m.status = StatusConnecting
	m.mu.Unlock()
	return m.refreshLocked(ctx)
}

// Stop ends polling and synchronously clears applied runtime observations. It
// is idempotent. Once Stop returns, no new applier operation can begin.
func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stop)

		m.mu.Lock()
		m.stopped = true
		m.enabled = false
		m.status = StatusStopped
		m.self = nil
		m.peers = make(map[string]Peer)
		cancel := m.activeCancel
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}

		m.lockTransitionForStop()
		defer m.unlockTransition()
		<-m.done
		if m.applier != nil {
			m.applier.ClearTailscaleObservations()
		}
	})
}

func (m *Manager) run() {
	defer close(m.done)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := m.lockTransition(context.Background()); err != nil {
				return
			}
			m.mu.RLock()
			enabled := m.enabled && !m.stopped
			m.mu.RUnlock()
			if enabled {
				ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
				_ = m.refreshLocked(ctx)
				cancel()
			} else {
				m.Detect()
			}
			m.unlockTransition()
		case <-m.stop:
			return
		}
	}
}

func (m *Manager) lockTransition(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.stop:
		return errManagerStopped
	case <-m.transitionToken:
		return nil
	}
}

func (m *Manager) lockTransitionForStop() {
	<-m.transitionToken
}

func (m *Manager) unlockTransition() {
	m.transitionToken <- struct{}{}
}

func (m *Manager) refreshLocked(ctx context.Context) error {
	m.mu.RLock()
	enabled := m.enabled && !m.stopped
	m.mu.RUnlock()
	if !enabled {
		return nil
	}

	detected := m.client.Detect()
	m.mu.Lock()
	m.detected = detected
	if !detected {
		if !m.stopped {
			m.status = m.failureStatusLocked()
		}
		m.mu.Unlock()
		return errors.New("tailscale: unavailable")
	}
	if m.stopped || !m.enabled {
		m.mu.Unlock()
		return nil
	}
	m.status = StatusConnecting
	m.mu.Unlock()

	requestCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	if !m.enabled || m.stopped {
		m.mu.Unlock()
		cancel()
		return nil
	}
	m.activeSequence++
	sequence := m.activeSequence
	m.activeCancel = cancel
	m.mu.Unlock()
	status, err := m.client.Status(requestCtx)
	cancel()
	m.mu.Lock()
	if m.activeSequence == sequence {
		m.activeCancel = nil
	}
	stopped := m.stopped
	m.mu.Unlock()
	if stopped {
		return nil
	}
	if err != nil {
		m.mu.Lock()
		if !m.stopped {
			m.status = m.failureStatusLocked()
		}
		m.mu.Unlock()
		return err
	}
	if status.BackendState != "Running" {
		m.mu.Lock()
		if !m.stopped {
			m.status = m.failureStatusLocked()
		}
		m.mu.Unlock()
		return errors.New("tailscale: backend unavailable")
	}
	peers := clonePeers(status.Peers)
	if m.applier != nil {
		m.mu.Lock()
		if !m.enabled || m.stopped {
			m.mu.Unlock()
			return nil
		}
		m.mu.Unlock()
		err = m.applier.ApplyTailscaleSnapshot(clonePeers(peers))
		m.mu.Lock()
		stopped = m.stopped
		if err != nil && !stopped {
			m.status = StatusDegraded
		}
		m.mu.Unlock()
		if stopped {
			return nil
		}
		if err != nil {
			return err
		}
	}

	peerMap := make(map[string]Peer, len(peers))
	for _, peer := range peers {
		if peer.NodeID != "" {
			peerMap[peer.NodeID] = clonePeer(peer)
		}
	}
	m.mu.Lock()
	if !m.enabled || m.stopped {
		m.mu.Unlock()
		return nil
	}
	m.peers = peerMap
	if status.Self == nil {
		m.self = nil
	} else {
		self := clonePeer(*status.Self)
		m.self = &self
	}
	m.lastRefresh = time.Now()
	m.detected = true
	m.status = StatusConnected
	m.mu.Unlock()
	return nil
}

func (m *Manager) failureStatusLocked() ManagerStatus {
	if !m.lastRefresh.IsZero() || m.self != nil || len(m.peers) != 0 {
		return StatusDegraded
	}
	return StatusUnavailable
}

func clonePeers(peers []Peer) []Peer {
	if peers == nil {
		return nil
	}
	result := make([]Peer, len(peers))
	for index, peer := range peers {
		result[index] = clonePeer(peer)
	}
	return result
}

func peersFromMap(peers map[string]Peer) []Peer {
	result := make([]Peer, 0, len(peers))
	for _, peer := range peers {
		result = append(result, clonePeer(peer))
	}
	sortPeers(result)
	return result
}

func sortPeers(peers []Peer) {
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].NodeID < peers[j].NodeID
	})
}
