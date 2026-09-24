package gatesentryDnsServer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	gatesentryTailscale "bitbucket.org/abdullah_irfan/gatesentryf/tailscale"
)

type lifecycleTestManager struct {
	stopCalls atomic.Int32
}

func (*lifecycleTestManager) Snapshot() gatesentryTailscale.ManagerSnapshot {
	return gatesentryTailscale.ManagerSnapshot{}
}
func (*lifecycleTestManager) Peers() []gatesentryTailscale.Peer { return nil }
func (*lifecycleTestManager) Peer(string) (gatesentryTailscale.Peer, bool) {
	return gatesentryTailscale.Peer{}, false
}
func (*lifecycleTestManager) SetEnabled(context.Context, bool) error { return nil }
func (m *lifecycleTestManager) Stop()                                { m.stopCalls.Add(1) }

func TestTailscaleManagerShutdownWinsConstructionRace(t *testing.T) {
	var lifecycle tailscaleManagerLifecycle
	manager := &lifecycleTestManager{}
	constructing := make(chan struct{})
	finishConstruction := make(chan struct{})
	started := make(chan struct{})

	go func() {
		startTailscaleManager(&lifecycle, false, func() TailscaleManager {
			close(constructing)
			<-finishConstruction
			return manager
		})
		close(started)
	}()

	<-constructing
	lifecycle.stop()
	close(finishConstruction)
	<-started

	if got := lifecycle.get(); got != nil {
		t.Fatalf("manager published after shutdown: %T", got)
	}
	if got := manager.stopCalls.Load(); got != 1 {
		t.Fatalf("manager Stop calls = %d, want 1", got)
	}
}

func TestTailscaleManagerRepeatedStartStopStopsEveryManager(t *testing.T) {
	var lifecycle tailscaleManagerLifecycle
	const starts = 32
	managers := make([]*lifecycleTestManager, starts)
	var wg sync.WaitGroup

	for i := range managers {
		managers[i] = &lifecycleTestManager{}
		wg.Add(1)
		go func(manager *lifecycleTestManager) {
			defer wg.Done()
			startTailscaleManager(&lifecycle, false, func() TailscaleManager { return manager })
		}(managers[i])
	}
	wg.Wait()

	for i := 0; i < starts; i++ {
		lifecycle.stop()
	}

	if got := lifecycle.get(); got != nil {
		t.Fatalf("manager remains published after repeated stops: %T", got)
	}
	for i, manager := range managers {
		if got := manager.stopCalls.Load(); got != 1 {
			t.Errorf("manager %d Stop calls = %d, want 1", i, got)
		}
	}
}
