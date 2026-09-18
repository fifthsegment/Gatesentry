package policy

import (
	"testing"
	"time"
)

func TestNewDecisionStampsTime(t *testing.T) {
	before := time.Now().UTC()
	d := NewDecision(ActionDecisionBlock, LayerDNS, "example.com")
	after := time.Now().UTC()
	if d.Timestamp.Before(before) || d.Timestamp.After(after) {
		t.Fatalf("timestamp %v outside [%v, %v]", d.Timestamp, before, after)
	}
	if d.Action != ActionDecisionBlock {
		t.Fatalf("action = %s, want %s", d.Action, ActionDecisionBlock)
	}
	if d.Layer != LayerDNS {
		t.Fatalf("layer = %s, want %s", d.Layer, LayerDNS)
	}
	if d.Domain != "example.com" {
		t.Fatalf("domain = %s, want example.com", d.Domain)
	}
}

func TestWithIdentity(t *testing.T) {
	d := NewDecision(ActionDecisionAllow, LayerExplicitProxy, "example.com")
	id := Identity{
		DeviceID: "dev-1",
		GroupID:  "kids",
		Source:   SourceDevice,
	}
	d = d.WithIdentity(id)
	if d.DeviceID != "dev-1" {
		t.Fatalf("device id = %s, want dev-1", d.DeviceID)
	}
	if d.GroupID != "kids" {
		t.Fatalf("group id = %s, want kids", d.GroupID)
	}
	if d.Source != SourceDevice {
		t.Fatalf("source = %s, want %s", d.Source, SourceDevice)
	}
	// Original identity must not be mutated.
	if id.DeviceID != "dev-1" {
		t.Fatal("WithIdentity mutated the source identity")
	}
}

func TestWithPolicyRevision(t *testing.T) {
	d := NewDecision(ActionDecisionBlock, LayerDNS, "example.com")
	d = d.WithPolicyRevision(42)
	if d.PolicyRevision != 42 {
		t.Fatalf("revision = %d, want 42", d.PolicyRevision)
	}
}

func TestRedactedRemovesDeviceID(t *testing.T) {
	d := NewDecision(ActionDecisionBlock, LayerDNS, "example.com").
		WithIdentity(Identity{DeviceID: "dev-1", GroupID: "kids", Source: SourceDevice})
	r := d.Redacted()
	if r.DeviceID != "" {
		t.Fatalf("redacted device id = %s, want empty", r.DeviceID)
	}
	if r.GroupID != "kids" {
		t.Fatalf("redacted group id = %s, want kids", r.GroupID)
	}
	// Original decision keeps the device id for drilldown.
	if d.DeviceID != "dev-1" {
		t.Fatal("Redacted mutated the original decision")
	}
}

func TestHasProvenance(t *testing.T) {
	cases := []struct {
		name string
		d    Decision
		want bool
	}{
		{"empty", Decision{}, false},
		{"matched rule", Decision{MatchedRule: "blocklist"}, true},
		{"group id", Decision{GroupID: "kids"}, true},
		{"policy revision", Decision{PolicyRevision: 1}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.HasProvenance(); got != c.want {
				t.Fatalf("HasProvenance() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDecisionActionsAreDistinct(t *testing.T) {
	actions := []DecisionAction{
		ActionDecisionAllow,
		ActionDecisionBlock,
		ActionDecisionInspect,
		ActionDecisionBypass,
		ActionDecisionError,
		ActionDecisionUnknown,
	}
	seen := make(map[DecisionAction]bool, len(actions))
	for _, a := range actions {
		if seen[a] {
			t.Fatalf("duplicate action: %s", a)
		}
		seen[a] = true
	}
	if len(seen) != 6 {
		t.Fatalf("distinct actions = %d, want 6", len(seen))
	}
}

func TestDecisionLayersAreDistinct(t *testing.T) {
	layers := []DecisionLayer{
		LayerDNS,
		LayerExplicitProxy,
		LayerTransparentProxy,
		LayerContent,
	}
	seen := make(map[DecisionLayer]bool, len(layers))
	for _, l := range layers {
		if seen[l] {
			t.Fatalf("duplicate layer: %s", l)
		}
		seen[l] = true
	}
	if len(seen) != 4 {
		t.Fatalf("distinct layers = %d, want 4", len(seen))
	}
}
