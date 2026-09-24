package policy

import (
	"net/url"
	"regexp"
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

// PreviewRequest is the input to Preview. Domain (or a URL to take it from) is
// required; the remaining fields describe the request context an administrator
// wants to test. Time is an RFC 3339 instant; when empty the service clock is
// used so schedule and pause evaluation match live behavior. Protocol selects
// the enforcement layer: "dns" (the default) or "proxy". On the proxy layer a
// URL is tested against the URL patterns of the rules that apply.
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

// PreviewEvaluation is the full explanation of one policy revision's decision
// for the supplied context. Action is the production evaluator outcome for the
// selected layer, and Trace is the evaluator's own record of how it got there,
// so preview and live enforcement cannot disagree. On the proxy layer,
// BlockURLRegexes and BlockContentTypes are the conditions the proxy tests
// against the request URL and the response; URLBlocked reports whether the
// supplied URL matches one of them.
type PreviewEvaluation struct {
	Action            PolicyAction `json:"action"`
	GroupID           string       `json:"group_id"`
	GroupName         string       `json:"group_name"`
	RuleID            string       `json:"rule_id"`
	MatchedDomain     string       `json:"matched_domain"`
	Reason            string       `json:"reason"`
	SafeSearch        bool         `json:"safe_search"`
	Trace             []TraceStep  `json:"trace"`
	ProxyOnly         []string     `json:"proxy_only"`
	BlockURLRegexes   []string     `json:"block_url_regexes,omitempty"`
	BlockContentTypes []string     `json:"block_content_types,omitempty"`
	URLBlocked        bool         `json:"url_blocked"`
	PolicyRevision    int          `json:"policy_revision"`
}

// PreviewResult compares a proposed policy revision with the active one for the
// supplied context. Active is always the live evaluation; Proposed equals
// Active when no proposed policy is supplied. Changed reports whether the
// proposed revision would alter the decision (action or resolving policy) for
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
// per snapshot so a proposed assignment is honored. Schedule, pause, and
// exception state are read at their live values; Time overrides the clock so
// an administrator can test schedule windows and pause expiry.
func (s *Service) Preview(req PreviewRequest) PreviewResult {
	ctx := s.liveContext()
	if req.Time != "" {
		if t, err := time.Parse(time.RFC3339, req.Time); err == nil {
			ctx.now = t
			// Pauses are live state; re-read them for the requested instant
			// so an expired pause does not apply to a future time.
			ctx.pa = pausesActiveAt(s.pa.all(), t)
		}
	}
	protocolDNS := req.Protocol == "" || strings.EqualFold(req.Protocol, "dns")
	domain := req.Domain
	if domain == "" && req.URL != "" {
		if parsed, err := url.Parse(req.URL); err == nil {
			domain = parsed.Hostname()
		}
	}

	base := s.resolveBaseIdentity(req.ClientIP, req.User, protocolDNS)
	activeEval := previewEvaluation(base, domain, req.URL, ctx, protocolDNS)

	proposedEval := activeEval
	if req.Proposed != nil {
		proposedCtx := ctx
		proposedCtx.snap = snapshotFromProposed(req.Proposed, ctx.snap.Version)
		proposedEval = previewEvaluation(base, domain, req.URL, proposedCtx, protocolDNS)
	}

	identity := base
	identity.GroupID = activeEval.GroupID
	return PreviewResult{
		Identity: identity,
		Active:   activeEval,
		Proposed: proposedEval,
		Changed:  proposedEval.Action != activeEval.Action || proposedEval.GroupID != activeEval.GroupID || proposedEval.URLBlocked != activeEval.URLBlocked,
	}
}

// previewEvaluation resolves the policy for one snapshot and runs the
// production evaluator for the selected layer.
func previewEvaluation(base Identity, domain, requestURL string, ctx evalContext, protocolDNS bool) PreviewEvaluation {
	identity := base
	identity.GroupID = groupIDForSnapshot(base, ctx.snap)
	layer := LayerDNS
	if !protocolDNS {
		layer = LayerExplicitProxy
	}
	out, group := evaluate(identity, domain, layer, ctx)
	eval := PreviewEvaluation{
		Action:            out.Action,
		GroupID:           group.ID,
		GroupName:         group.Name,
		RuleID:            out.RuleID,
		MatchedDomain:     out.Matched,
		Reason:            out.Reason,
		SafeSearch:        group.SafeSearch,
		Trace:             out.Trace,
		ProxyOnly:         out.ProxyOnly,
		BlockURLRegexes:   out.URLRegexes,
		BlockContentTypes: out.ContentTypes,
		PolicyRevision:    ctx.snap.Version,
	}
	if requestURL != "" {
		for _, pattern := range out.URLRegexes {
			if re, err := regexp.Compile(pattern); err == nil && re.MatchString(requestURL) {
				eval.URLBlocked = true
				break
			}
		}
	}
	if eval.ProxyOnly == nil {
		eval.ProxyOnly = []string{}
	}
	return eval
}

// snapshotFromProposed builds an evaluation snapshot from a proposed policy
// revision, inheriting the live document version so the revision number
// reported in preview matches the active revision.
func snapshotFromProposed(p *ProposedPolicy, version int) PolicySnapshot {
	snap := PolicySnapshot{
		Groups:      make(map[string]PolicyGroup, len(p.Groups)+1),
		Assignments: make(map[string]string, len(p.Assignments)),
		Version:     version,
		UpdatedAt:   time.Now().UTC(),
	}
	snap.Groups[DefaultGroupID] = DefaultGroup()
	for _, g := range p.Groups {
		snap.Groups[g.ID] = g
	}
	for _, a := range p.Assignments {
		snap.Assignments[a.DeviceID] = a.GroupID
	}
	return snap
}
