package policy

import (
	"fmt"
	"strings"
	"time"
)

// ProposedPolicy is a candidate policy revision supplied to Preview. It carries
// only groups and assignments: exceptions, pauses, and device observations are
// not part of a proposed policy because preview must not invent device
// identity or grant bypasses. The proposed snapshot is evaluated against the
// live exception, pause, and device state so an administrator sees how a
// group/assignment change alone alters decisions.
type ProposedPolicy struct {
	Groups      []PolicyGroup      `json:"groups"`
	Assignments []DeviceAssignment `json:"assignments"`
}

// PreviewRequest is the input to Preview. Domain is required; the remaining
// fields describe the request context an administrator wants to test. Time is
// an RFC 3339 instant; when empty the service clock is used so schedule and
// pause evaluation match live behavior. Protocol selects which enforcement
// layer's condition model applies: "dns" (the default) or "proxy". URL and
// MIMEType do not change the policy decision itself (the policy service is
// domain-based) but are echoed and used to report which downstream conditions
// remain unresolved.
type PreviewRequest struct {
	ClientIP string          `json:"client_ip"`
	User     string          `json:"user"`
	Domain   string          `json:"domain"`
	Protocol string          `json:"protocol,omitempty"`
	URL      string          `json:"url,omitempty"`
	MIMEType string          `json:"mime,omitempty"`
	Time     string          `json:"time,omitempty"`
	Proposed *ProposedPolicy `json:"proposed,omitempty"`
}

// PreviewStage is one precedence stage in the decision trail. Applied reports
// whether the stage took effect; Action is the action that stage contributes
// (ActionNone when the stage expresses no opinion); Detail is a short,
// non-sensitive explanation such as the matched pattern or scope.
type PreviewStage struct {
	Name    string       `json:"name"`
	Applied bool         `json:"applied"`
	Action  PolicyAction `json:"action"`
	Detail  string       `json:"detail,omitempty"`
}

// PreviewEvaluation is the full explanation of one policy revision's decision
// for the supplied context. Action is the production evaluator outcome (the
// same value EvaluateDNS returns), so preview and live enforcement agree for
// deterministic cases. Stages is the ordered precedence trail. Inapplicable
// conditions are ones the selected layer cannot enforce at all (for example
// URL/MIME/inspection on DNS); unevaluated conditions are ones a downstream
// layer could resolve but this preview does not (for example URL regex on a
// proxy request, which belongs to the rules engine and content filter).
type PreviewEvaluation struct {
	Action                 PolicyAction   `json:"action"`
	GroupID                string         `json:"group_id"`
	RuleID                 string         `json:"rule_id"`
	MatchedDomain          string         `json:"matched_domain"`
	Reason                 string         `json:"reason"`
	Stages                 []PreviewStage `json:"stages"`
	InapplicableConditions []string       `json:"inapplicable_conditions"`
	UnevaluatedConditions  []string       `json:"unevaluated_conditions"`
	PolicyRevision         int            `json:"policy_revision"`
}

// PreviewResult compares a proposed policy revision with the active one for the
// supplied context. Active is always the live evaluation; Proposed equals
// Active when no proposed policy is supplied. Changed reports whether the
// proposed revision would alter the decision (action or resolving group) for
// this request, which is what an administrator needs to know before saving.
type PreviewResult struct {
	Identity Identity          `json:"identity"`
	Active   PreviewEvaluation `json:"active"`
	Proposed PreviewEvaluation `json:"proposed"`
	Changed  bool              `json:"changed"`
}

// Preview evaluates a request context against the active policy and, when
// supplied, a proposed revision, without persisting anything or making
// external classification requests. It resolves device/user identity from the
// live device store (preview never invents devices) but resolves the policy
// group per snapshot so a proposed assignment is honored. Schedule, pause,
// and exception state are read at their live values; Time overrides the clock
// so an administrator can test schedule windows and pause expiry.
func (s *Service) Preview(req PreviewRequest) PreviewResult {
	now := s.now()
	if req.Time != "" {
		if t, err := time.Parse(time.RFC3339, req.Time); err == nil {
			now = t
		}
	}
	protocolDNS := req.Protocol == "" || strings.EqualFold(req.Protocol, "dns")

	base := s.resolveBaseIdentity(req.ClientIP, req.User, protocolDNS)

	liveSnap := s.Snapshot()
	excSnap := s.ExceptionSnapshot()
	pauseSnap := s.PauseSnapshot()

	activeEval := s.previewEvaluation(base, req.Domain, liveSnap, excSnap, pauseSnap, now, protocolDNS)

	proposedSnap := liveSnap
	if req.Proposed != nil {
		proposedSnap = snapshotFromProposed(req.Proposed, liveSnap.Version)
	}
	proposedEval := activeEval
	if req.Proposed != nil {
		proposedEval = s.previewEvaluation(base, req.Domain, proposedSnap, excSnap, pauseSnap, now, protocolDNS)
	}

	identity := base
	identity.GroupID = activeEval.GroupID
	return PreviewResult{
		Identity: identity,
		Active:   activeEval,
		Proposed: proposedEval,
		Changed:  proposedEval.Action != activeEval.Action || proposedEval.GroupID != activeEval.GroupID,
	}
}

// previewEvaluation resolves the group for one snapshot and runs the production
// evaluator plus the precedence trail. The final Action always comes from
// evaluateDNSAt so preview and live enforcement share one decision path.
func (s *Service) previewEvaluation(base Identity, domain string, snap PolicySnapshot, exc ExceptionSnapshot, pa PauseSnapshot, now time.Time, protocolDNS bool) PreviewEvaluation {
	identity := base
	identity.GroupID = groupIDForSnapshot(base, snap)

	dec := s.evaluateDNSAt(identity, domain, snap, exc, pa, now)
	stages := buildPreviewStages(identity, domain, snap, exc, pa, now, s.categories)
	inapplicable, unevaluated := previewConditions(protocolDNS)

	return PreviewEvaluation{
		Action:                 dec.Action,
		GroupID:                dec.GroupID,
		RuleID:                 dec.RuleID,
		MatchedDomain:          dec.MatchedDomain,
		Reason:                 dec.Reason,
		Stages:                 stages,
		InapplicableConditions: inapplicable,
		UnevaluatedConditions:  unevaluated,
		PolicyRevision:         snap.Version,
	}
}

// buildPreviewStages walks the precedence order (exception, group, schedule,
// pause, domain or category match) using the same pure helpers the evaluator
// uses, so the trail always explains the final action without diverging from
// it.
func buildPreviewStages(identity Identity, domain string, snap PolicySnapshot, exc ExceptionSnapshot, pa PauseSnapshot, now time.Time, index *CategoryIndex) []PreviewStage {
	stages := make([]PreviewStage, 0, 5)
	if e, ok := evaluateExceptionIn(identity, domain, exc); ok {
		stages = append(stages, PreviewStage{Name: "exception", Applied: true, Action: ActionAllow, Detail: fmt.Sprintf("scope=%s domain=%s", e.Scope, e.Domain)})
		return stages
	}
	stages = append(stages, PreviewStage{Name: "exception", Applied: false})

	group, ok := snap.Groups[identity.GroupID]
	if !ok {
		stages = append(stages, PreviewStage{Name: "group", Applied: false, Detail: "no group assigned"})
		return stages
	}
	stages = append(stages, PreviewStage{Name: "group", Applied: true, Action: group.Action, Detail: group.ID})

	if !scheduleActiveAt(group, now) {
		stages = append(stages, PreviewStage{Name: "schedule", Applied: true, Action: ActionNone, Detail: "inactive"})
		return stages
	}
	stages = append(stages, PreviewStage{Name: "schedule", Applied: false, Detail: "active"})

	if p, ok := evaluatePauseIn(identity, pa); ok && group.Action == ActionBlock {
		stages = append(stages, PreviewStage{Name: "pause", Applied: true, Action: ActionNone, Detail: fmt.Sprintf("scope=%s", p.Scope)})
		return stages
	}
	stages = append(stages, PreviewStage{Name: "pause", Applied: false})

	matched, categoryID := matchGroupRule(group, domain, index)
	switch {
	case matched == "":
		stages = append(stages, PreviewStage{Name: "group_domain", Applied: false, Detail: "no domain or category match"})
	case categoryID == "":
		stages = append(stages, PreviewStage{Name: "group_domain", Applied: true, Action: group.Action, Detail: matched})
	default:
		stages = append(stages, PreviewStage{Name: "group_category", Applied: true, Action: group.Action, Detail: categoryID})
	}
	if matched != "" {
		outcome := evaluateGroupRulesAt(group, matched, categoryID, identity.AuthUser, now, LayerDNS)
		if outcome.RuleID != "" {
			stages = append(stages, PreviewStage{Name: "group_rule", Applied: true, Action: outcome.Action, Detail: outcome.Reason})
		}
		if len(outcome.Unresolved) > 0 {
			stages = append(stages, PreviewStage{Name: "group_rule_conditions", Applied: false, Detail: "DNS cannot evaluate " + strings.Join(outcome.Unresolved, ", ")})
		}
	}
	return stages
}

// previewConditions reports which conditions the selected layer cannot enforce
// and which remain unresolved by this preview. DNS cannot see URL paths,
// MIME types, or TLS content, so all three are inapplicable. The proxy layer
// could hand URL/MIME/inspection to the rules engine and content filter, but
// the policy preview does not run those, so they are unevaluated rather than
// falsely reported as enforced.
func previewConditions(protocolDNS bool) (inapplicable, unevaluated []string) {
	if protocolDNS {
		return dnsInapplicableConditions(), []string{}
	}
	return []string{}, []string{"url_regex", "content_type", "mitm"}
}

// snapshotFromProposed builds an evaluation snapshot from a proposed policy
// revision, inheriting the live document version so the revision number
// reported in preview matches the active revision.
func snapshotFromProposed(p *ProposedPolicy, version int) PolicySnapshot {
	snap := PolicySnapshot{
		Groups:      make(map[string]PolicyGroup, len(p.Groups)),
		Assignments: make(map[string]string, len(p.Assignments)),
		Version:     version,
		UpdatedAt:   time.Now().UTC(),
	}
	for _, g := range p.Groups {
		snap.Groups[g.ID] = g
	}
	for _, a := range p.Assignments {
		snap.Assignments[a.DeviceID] = a.GroupID
	}
	return snap
}
