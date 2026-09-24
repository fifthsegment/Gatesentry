package tailscale

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recordingApplier struct {
	mu        sync.Mutex
	snapshots [][]Peer
	clears    int
	err       error
}

func (applier *recordingApplier) ApplyTailscaleSnapshot(peers []Peer) error {
	applier.mu.Lock()
	defer applier.mu.Unlock()
	applier.snapshots = append(applier.snapshots, clonePeers(peers))
	return applier.err
}

func (applier *recordingApplier) ClearTailscaleObservations() {
	applier.mu.Lock()
	defer applier.mu.Unlock()
	applier.clears++
}

func (applier *recordingApplier) state() ([][]Peer, int) {
	applier.mu.Lock()
	defer applier.mu.Unlock()
	result := make([][]Peer, len(applier.snapshots))
	for index, peers := range applier.snapshots {
		result[index] = clonePeers(peers)
	}
	return result, applier.clears
}

func TestManagerDisabledDetectsWithoutQueryingLocalAPI(t *testing.T) {
	var calls atomic.Int32
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprint(response, `{"BackendState":"Running"}`)
	})
	defer closeServer()

	manager := NewManager(client, &recordingApplier{}, 10*time.Millisecond)
	defer manager.Stop()
	time.Sleep(35 * time.Millisecond)

	snapshot := manager.Snapshot()
	if snapshot.Enabled || !snapshot.Detected || snapshot.Status != StatusDisabled {
		t.Fatalf("Snapshot() = %#v, want disabled and detected", snapshot)
	}
	if calls.Load() != 0 {
		t.Fatalf("disabled manager made %d LocalAPI calls, want 0", calls.Load())
	}
}

func TestManagerEnableRefreshesAndAppliesImmediately(t *testing.T) {
	var calls atomic.Int32
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprint(response, `{"BackendState":"Running","Self":{"ID":"self"},"Peer":{"key":{"ID":"node-1","HostName":"phone","TailscaleIPs":["100.64.0.8"],"Online":true}}}`)
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()

	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatalf("SetEnabled(true) error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want immediate call", calls.Load())
	}
	snapshot := manager.Snapshot()
	if !snapshot.Enabled || !snapshot.Detected || snapshot.Status != StatusConnected || snapshot.LastRefresh.IsZero() {
		t.Fatalf("Snapshot() = %#v, want connected refresh", snapshot)
	}
	if snapshot.Self == nil || snapshot.Self.NodeID != "self" {
		t.Fatalf("Self = %#v, want self", snapshot.Self)
	}
	if len(snapshot.Peers) != 1 || snapshot.Peers[0].NodeID != "node-1" {
		t.Fatalf("Peers = %#v, want node-1", snapshot.Peers)
	}
	applied, _ := applier.state()
	if len(applied) != 1 || len(applied[0]) != 1 || applied[0][0].NodeID != "node-1" {
		t.Fatalf("applied snapshots = %#v, want node-1", applied)
	}

	peer, ok := manager.Peer("node-1")
	if !ok || peer.Name != "phone" {
		t.Fatalf("Peer(node-1) = %#v, %v", peer, ok)
	}
	peer.Addresses[0] = "mutated"
	snapshot.Peers[0].Addresses[0] = "also-mutated"
	fresh, _ := manager.Peer("node-1")
	if fresh.Addresses[0] != "100.64.0.8" {
		t.Fatalf("lookup exposed internal memory: %#v", fresh)
	}
	if _, ok := manager.Peer("key"); ok {
		t.Fatal("Peer(map-key) succeeded; lookup must use PeerStatus.ID")
	}
}

func TestManagerFailedRefreshRetainsLastGoodSnapshot(t *testing.T) {
	var fail atomic.Bool
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1","HostName":"phone","TailscaleIPs":["100.64.0.8"]}}}`)
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	lastRefresh := manager.Snapshot().LastRefresh

	fail.Store(true)
	if err := manager.SetEnabled(context.Background(), true); err == nil {
		t.Fatal("second SetEnabled(true) error = nil")
	}
	snapshot := manager.Snapshot()
	if snapshot.Status != StatusDegraded || len(snapshot.Peers) != 1 || snapshot.Peers[0].NodeID != "node-1" {
		t.Fatalf("Snapshot() = %#v, want degraded with last-good peer", snapshot)
	}
	if !snapshot.LastRefresh.Equal(lastRefresh) {
		t.Fatalf("LastRefresh changed on failure: %v -> %v", lastRefresh, snapshot.LastRefresh)
	}
	applied, _ := applier.state()
	if len(applied) != 1 {
		t.Fatalf("Apply called on failed refresh: %d snapshots", len(applied))
	}
}

func TestManagerSuccessfulEmptySnapshotClearsMissingPeers(t *testing.T) {
	var empty atomic.Bool
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		if empty.Load() {
			fmt.Fprint(response, `{"BackendState":"Running","Peer":{}}`)
			return
		}
		fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1","TailscaleIPs":["100.64.0.8"]}}}`)
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	empty.Store(true)
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if peers := manager.Peers(); len(peers) != 0 {
		t.Fatalf("Peers() = %#v, want empty complete snapshot", peers)
	}
	applied, _ := applier.state()
	if len(applied) != 2 || len(applied[1]) != 0 {
		t.Fatalf("applied snapshots = %#v, want final empty snapshot", applied)
	}
}

func TestManagerDisableClearsObservationsWithoutStatusRequest(t *testing.T) {
	var calls atomic.Int32
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprint(response, `{"BackendState":"Running"}`)
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	if err := manager.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before {
		t.Fatalf("disable queried LocalAPI: calls %d -> %d", before, calls.Load())
	}
	_, clears := applier.state()
	if clears != 1 {
		t.Fatalf("clears = %d, want 1", clears)
	}
	if snapshot := manager.Snapshot(); snapshot.Enabled || snapshot.Status != StatusDisabled {
		t.Fatalf("Snapshot() = %#v, want disabled", snapshot)
	}
}

func TestManagerPeriodicRefreshesAreSerialized(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	var calls atomic.Int32
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		calls.Add(1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		fmt.Fprint(response, `{"BackendState":"Running"}`)
	})
	defer closeServer()
	manager := NewManager(client, &recordingApplier{}, 2*time.Millisecond)
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	manager.Stop()
	manager.Stop()
	if calls.Load() < 2 {
		t.Fatalf("calls = %d, want periodic refreshes", calls.Load())
	}
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent requests = %d, want 1", maximum.Load())
	}
	if snapshot := manager.Snapshot(); snapshot.Status != StatusStopped {
		t.Fatalf("status = %q, want stopped", snapshot.Status)
	}
}

func TestManagerApplyFailureIsDegradedAndDoesNotPublishSnapshot(t *testing.T) {
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1"}}}`)
	})
	defer closeServer()
	applier := &recordingApplier{err: fmt.Errorf("storage unavailable with sensitive detail")}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()

	if err := manager.SetEnabled(context.Background(), true); err == nil {
		t.Fatal("SetEnabled(true) error = nil")
	}
	snapshot := manager.Snapshot()
	if snapshot.Status != StatusDegraded || len(snapshot.Peers) != 0 || !snapshot.LastRefresh.IsZero() {
		t.Fatalf("Snapshot() = %#v, want degraded without un-applied peer", snapshot)
	}
}

func TestManagerUnavailableAndStoppedErrorsAreSanitized(t *testing.T) {
	client := NewClientWithSocket(t.TempDir() + "/missing.sock")
	manager := NewManager(client, &recordingApplier{}, time.Hour)
	if err := manager.SetEnabled(context.Background(), true); err == nil {
		t.Fatal("SetEnabled(true) error = nil")
	}
	if snapshot := manager.Snapshot(); snapshot.Status != StatusUnavailable || snapshot.Detected {
		t.Fatalf("Snapshot() = %#v, want unavailable", snapshot)
	}
	manager.Stop()
	if err := manager.SetEnabled(context.Background(), true); err == nil {
		t.Fatal("SetEnabled after Stop error = nil")
	}
}

func TestManagerLostSocketRetainsPeersAsDegraded(t *testing.T) {
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1","HostName":"phone","TailscaleIPs":["100.64.0.8"]}}}`)
	})
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	closeServer()

	if err := manager.SetEnabled(context.Background(), true); err == nil {
		t.Fatal("refresh with lost socket error = nil")
	}
	snapshot := manager.Snapshot()
	if snapshot.Status != StatusDegraded || snapshot.Detected || len(snapshot.Peers) != 1 {
		t.Fatalf("Snapshot() = %#v, want degraded last-good snapshot", snapshot)
	}
}

func TestManagerDisableCancelsActiveRefresh(t *testing.T) {
	started := make(chan struct{})
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()

	enabled := make(chan error, 1)
	go func() { enabled <- manager.SetEnabled(context.Background(), true) }()
	<-started
	disabled := make(chan error, 1)
	go func() { disabled <- manager.SetEnabled(context.Background(), false) }()
	select {
	case err := <-disabled:
		if err != nil {
			t.Fatalf("SetEnabled(false) error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("disable did not cancel active refresh")
	}
	if err := <-enabled; err == nil {
		t.Fatal("cancelled enable error = nil")
	}
	if snapshot := manager.Snapshot(); snapshot.Enabled || snapshot.Status != StatusDisabled || len(snapshot.Peers) != 0 {
		t.Fatalf("Snapshot() = %#v, want cleared disabled state", snapshot)
	}
}

func TestManagerReenableUpdatesPeerAddresses(t *testing.T) {
	var second atomic.Bool
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		address := "100.64.0.8"
		if second.Load() {
			address = "100.64.0.9"
		}
		fmt.Fprintf(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1","TailscaleIPs":[%q]}}}`, address)
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if peers := manager.Peers(); len(peers) != 0 {
		t.Fatalf("disabled Peers() = %#v, want no runtime inventory", peers)
	}
	second.Store(true)
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	peer, ok := manager.Peer("node-1")
	if !ok || len(peer.Addresses) != 1 || peer.Addresses[0] != "100.64.0.9" {
		t.Fatalf("Peer(node-1) = %#v, %v; want changed address", peer, ok)
	}
}

func TestManagerSetEnabledHonorsCancelledContextWhileWaiting(t *testing.T) {
	started := make(chan struct{})
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	})
	defer closeServer()
	manager := NewManager(client, &recordingApplier{}, time.Hour)
	defer manager.Stop()

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { first <- manager.SetEnabled(firstCtx, true) }()
	<-started

	waitingCtx, cancelWaiting := context.WithCancel(context.Background())
	waiting := make(chan error, 1)
	go func() { waiting <- manager.SetEnabled(waitingCtx, true) }()
	cancelWaiting()
	select {
	case err := <-waiting:
		if err == nil {
			t.Fatal("waiting SetEnabled error = nil")
		}
	case <-time.After(time.Second):
		t.Fatal("waiting SetEnabled ignored context cancellation")
	}
	cancelFirst()
	<-first
}

func TestManagerNonRunningBackendRetainsLastGoodSnapshot(t *testing.T) {
	var running atomic.Bool
	running.Store(true)
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		if running.Load() {
			fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1","TailscaleIPs":["100.64.0.8"]}}}`)
			return
		}
		fmt.Fprint(response, `{"BackendState":"NeedsLogin","Peer":{}}`)
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	running.Store(false)
	if err := manager.SetEnabled(context.Background(), true); err == nil {
		t.Fatal("NeedsLogin refresh error = nil")
	}
	snapshot := manager.Snapshot()
	if snapshot.Status != StatusDegraded || len(snapshot.Peers) != 1 || snapshot.Peers[0].NodeID != "node-1" {
		t.Fatalf("Snapshot() = %#v, want degraded last-good peer", snapshot)
	}
	applied, _ := applier.state()
	if len(applied) != 1 {
		t.Fatalf("Apply called for NeedsLogin response: %d snapshots", len(applied))
	}
}

type blockingApplier struct {
	mu           sync.Mutex
	applyStarted chan struct{}
	applyRelease chan struct{}
	applyDone    chan struct{}
	clears       int
	writes       int
}

func (applier *blockingApplier) ApplyTailscaleSnapshot(_ []Peer) error {
	close(applier.applyStarted)
	<-applier.applyRelease
	applier.mu.Lock()
	applier.writes++
	applier.mu.Unlock()
	close(applier.applyDone)
	return nil
}

func (applier *blockingApplier) ClearTailscaleObservations() {
	applier.mu.Lock()
	defer applier.mu.Unlock()
	applier.clears++
}

func (applier *blockingApplier) state() (writes, clears int) {
	applier.mu.Lock()
	defer applier.mu.Unlock()
	return applier.writes, applier.clears
}

func TestManagerConcurrentEnableDisableLaterTransitionWins(t *testing.T) {
	started := make(chan struct{})
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	defer manager.Stop()

	enableDone := make(chan error, 1)
	go func() { enableDone <- manager.SetEnabled(context.Background(), true) }()
	<-started
	disableDone := make(chan error, 1)
	go func() { disableDone <- manager.SetEnabled(context.Background(), false) }()

	select {
	case err := <-disableDone:
		if err != nil {
			t.Fatalf("SetEnabled(false) error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("later disable did not complete")
	}
	if err := <-enableDone; err == nil {
		t.Fatal("cancelled enable error = nil")
	}
	if snapshot := manager.Snapshot(); snapshot.Enabled || snapshot.Status != StatusDisabled {
		t.Fatalf("Snapshot() = %#v, want later disable to win", snapshot)
	}
}

type blockingClearApplier struct {
	recordingApplier
	clearStarted chan struct{}
	clearRelease chan struct{}
	blockOnce    sync.Once
}

func (applier *blockingClearApplier) ClearTailscaleObservations() {
	applier.blockOnce.Do(func() {
		close(applier.clearStarted)
		<-applier.clearRelease
	})
	applier.recordingApplier.ClearTailscaleObservations()
}

func TestManagerConcurrentDisableEnableLaterTransitionWins(t *testing.T) {
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1"}}}`)
	})
	defer closeServer()
	applier := &blockingClearApplier{
		clearStarted: make(chan struct{}),
		clearRelease: make(chan struct{}),
	}
	manager := NewManager(client, applier, time.Hour)
	if err := manager.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	disableDone := make(chan error, 1)
	go func() { disableDone <- manager.SetEnabled(context.Background(), false) }()
	<-applier.clearStarted
	enableDone := make(chan error, 1)
	go func() { enableDone <- manager.SetEnabled(context.Background(), true) }()
	close(applier.clearRelease)

	if err := <-disableDone; err != nil {
		t.Fatalf("SetEnabled(false) error = %v", err)
	}
	if err := <-enableDone; err != nil {
		t.Fatalf("SetEnabled(true) error = %v", err)
	}
	if snapshot := manager.Snapshot(); !snapshot.Enabled || snapshot.Status != StatusConnected || len(snapshot.Peers) != 1 {
		t.Fatalf("Snapshot() = %#v, want later enable to win", snapshot)
	}
	manager.Stop()
}

func TestManagerStopCancelsInFlightRefreshAndClearsBeforeReturn(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	client, closeServer := unixTestServer(t, func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(cancelled)
	})
	defer closeServer()
	applier := &recordingApplier{}
	manager := NewManager(client, applier, time.Hour)
	enableDone := make(chan error, 1)
	go func() { enableDone <- manager.SetEnabled(context.Background(), true) }()
	<-started

	stopDone := make(chan struct{})
	go func() {
		manager.Stop()
		close(stopDone)
	}()
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("Stop did not promptly cancel the LocalAPI request")
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("Stop did not wait for refresh cleanup")
	}
	if err := <-enableDone; err != nil {
		t.Fatalf("stopped refresh error = %v, want nil after Stop owns lifecycle", err)
	}
	applied, clears := applier.state()
	if len(applied) != 0 || clears != 1 {
		t.Fatalf("applier state = %d applies, %d clears; want 0, 1", len(applied), clears)
	}
	if snapshot := manager.Snapshot(); snapshot.Status != StatusStopped || snapshot.Enabled || len(snapshot.Peers) != 0 {
		t.Fatalf("Snapshot() after Stop = %#v", snapshot)
	}
}

func TestManagerStopWaitsForInFlightApplyAndClearsLast(t *testing.T) {
	client, closeServer := unixTestServer(t, func(response http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(response, `{"BackendState":"Running","Peer":{"key":{"ID":"node-1"}}}`)
	})
	defer closeServer()
	applier := &blockingApplier{
		applyStarted: make(chan struct{}),
		applyRelease: make(chan struct{}),
		applyDone:    make(chan struct{}),
	}
	manager := NewManager(client, applier, time.Hour)
	enableDone := make(chan error, 1)
	go func() { enableDone <- manager.SetEnabled(context.Background(), true) }()
	<-applier.applyStarted

	stopDone := make(chan struct{})
	go func() {
		manager.Stop()
		close(stopDone)
	}()
	select {
	case <-stopDone:
		t.Fatal("Stop returned before in-flight apply completed")
	case <-time.After(25 * time.Millisecond):
	}

	close(applier.applyRelease)
	<-applier.applyDone
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("Stop did not wait for refresh cleanup")
	}
	<-enableDone

	writes, clears := applier.state()
	if writes != 1 || clears != 1 {
		t.Fatalf("applier state = writes %d, clears %d; want 1, 1", writes, clears)
	}
	if snapshot := manager.Snapshot(); snapshot.Status != StatusStopped || snapshot.Enabled || len(snapshot.Peers) != 0 {
		t.Fatalf("Snapshot() after Stop = %#v", snapshot)
	}

	time.Sleep(25 * time.Millisecond)
	writesAfter, clearsAfter := applier.state()
	if writesAfter != writes || clearsAfter != clears {
		t.Fatalf("applier changed after Stop returned: (%d, %d) -> (%d, %d)", writes, clears, writesAfter, clearsAfter)
	}
}
