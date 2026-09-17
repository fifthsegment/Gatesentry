package policy

import "time"

// DecisionAction is the normalized outcome of a filtering decision. It is the
// single vocabulary used by logging, APIs, and user-facing responses so that
// DNS, explicit proxy, transparent proxy, and content inspection all speak the
// same language.
type DecisionAction string

const (
	// ActionDecisionAllow permits the request to proceed.
	ActionDecisionAllow DecisionAction = "allow"
	// ActionDecisionBlock denies the request.
	ActionDecisionBlock DecisionAction = "block"
	// ActionDecisionInspect means the request proceeds but content is
	// inspected (for example MITM with content scanning).
	ActionDecisionInspect DecisionAction = "inspect"
	// ActionDecisionBypass means an exception or allow-rule exempted the
	// request from a rule that would otherwise have blocked it.
	ActionDecisionBypass DecisionAction = "bypass"
	// ActionDecisionError means the adapter could not reach a decision
	// because of an upstream or internal error; the request was not
	// filtered on its merits.
	ActionDecisionError DecisionAction = "error"
	// ActionDecisionUnknown means the adapter could not classify the
	// outcome (for example an unhandled response type or a probe that did
	// not match any rule).
	ActionDecisionUnknown DecisionAction = "unknown"
)

// DecisionLayer identifies which filtering stage produced a decision.
type DecisionLayer string

const (
	LayerDNS              DecisionLayer = "dns"
	LayerExplicitProxy    DecisionLayer = "explicit_proxy"
	LayerTransparentProxy DecisionLayer = "transparent_proxy"
	LayerContent          DecisionLayer = "content"
)

// Decision is the canonical structured filtering outcome. The policy service
// is the decision authority; DNS, proxy, and content adapters build a
// Decision at each enforcement point and hand it to the logger. It records
// action, matched rule/list, reason, layer, device/group context, timestamp,
// and the applied policy revision where available.
//
// Redaction: Domain/URL, ClientIP, GroupID, and SourceKind are stored because
// they are required for logs, statistics, and the device/policy drilldown.
// DeviceID is a stable identifier; it is recorded for drilldown but must be
// omitted from user-facing API responses via Redacted. Credentials, browsing
// content, image contents, and full identity explanations are never stored.
type Decision struct {
	Action DecisionAction
	// ResponseType is the adapter-native outcome label (for example the DNS
	// "blocked"/"exception"/"forward" vocabulary or a proxy ProxyAction such
	// as "blocked_url"). Action normalizes it into the shared six-value
	// vocabulary; ResponseType is retained so legacy log and stat viewers
	// that filter on the native label keep working unchanged.
	ResponseType   string
	MatchedRule    string
	Reason         string
	Layer          DecisionLayer
	Domain         string
	URL            string
	ClientIP       string
	DeviceID       string
	GroupID        string
	Source         SourceKind
	PolicyRevision int
	Timestamp      time.Time
}

// NewDecision builds a Decision with a timestamp. Adapters use it so they
// cannot forget to stamp the time.
func NewDecision(action DecisionAction, layer DecisionLayer, domain string) Decision {
	return Decision{
		Action:    action,
		Layer:     layer,
		Domain:    domain,
		Timestamp: time.Now().UTC(),
	}
}

// WithIdentity attaches resolved identity context from the policy service.
func (d Decision) WithIdentity(id Identity) Decision {
	d.DeviceID = id.DeviceID
	d.GroupID = id.GroupID
	d.Source = id.Source
	return d
}

// WithPolicyRevision attaches the policy document version in effect.
func (d Decision) WithPolicyRevision(rev int) Decision {
	d.PolicyRevision = rev
	return d
}

// Redacted returns a copy with stable device identifiers removed for
// user-facing API responses and statistics. Group, source, matched rule,
// reason, and layer are retained because they explain the decision without
// identifying a specific device. ClientIP is retained because it is already
// present in existing log entries and is required for the activity drilldown;
// callers that must hide addresses apply their own policy on top.
func (d Decision) Redacted() Decision {
	out := d
	out.DeviceID = ""
	return out
}

// HasProvenance reports whether the decision carries non-trivial matched-rule
// or group provenance. Adapters and tests use it to distinguish a decision
// that merely names an outcome from one that explains it.
func (d Decision) HasProvenance() bool {
	return d.MatchedRule != "" || d.GroupID != "" || d.PolicyRevision != 0
}
