package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	gatesentry2storage "bitbucket.org/abdullah_irfan/gatesentryf/storage"
	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
)

const (
	// StorageKey is the optional, schema-1-compatible settings key holding
	// the policy document. A missing key means no groups exist and all
	// adapters keep their pre-policy behavior.
	StorageKey = "policy_groups"
	// DocumentVersion is the persisted policy document format version.
	DocumentVersion = 1
)

// DeviceResolver is the read-only view the policy service needs from the
// discovery-owned device store. Discovery stays the only writer of observed
// identity; policy only reads it to resolve request context.
type DeviceResolver interface {
	// ResolveDeviceByIP returns the device observed at an IP. The boolean
	// reports whether more than one device currently claims that address,
	// which makes identity ambiguous (NAT or address reuse).
	// The third boolean reports whether the observation is stale: old
	// enough that the address may have been reassigned to a different
	// device, which makes the observation unsafe to enforce against.
	ResolveDeviceByIP(ip string) (deviceID string, ambiguous bool, stale bool)
}

// Service owns policy enforcement decisions. It is the dedicated policy
// engine: discovery observes devices, storage owns durability, and this
// service owns how a resolved identity maps to an enforcement outcome.
type Service struct {
	storage *gatesentry2storage.MapStore
	devices DeviceResolver

	mu       sync.RWMutex
	snapshot PolicySnapshot
	// exc holds the in-memory exception snapshot, loaded from the same
	// MapStore as policy groups. Separate from the group snapshot so
	// exception writes do not require re-encoding the group document.
	exc exceptionStorage
	pa  pauseStorage
	// accessRL rate-limits public access requests per client IP. It is
	// in-memory only so a restart resets the counter.
	accessRL *rateLimiter
	nowFunc  func() time.Time
}

// NewService creates a policy service. A nil storage is supported for tests
// and yields an empty policy; production wiring must pass the settings store.
func NewService(storage *gatesentry2storage.MapStore, devices DeviceResolver) (*Service, error) {
	s := &Service{storage: storage, devices: devices}
	doc, err := s.loadDocument()
	if err != nil {
		return nil, err
	}
	s.snapshot = snapshotFromDocument(doc)
	if err := s.loadExceptions(); err != nil {
		return nil, err
	}
	if err := s.loadPauses(); err != nil {
		return nil, err
	}
	s.accessRL = newAccessRequestRateLimiter()
	s.nowFunc = func() time.Time { return time.Now() }
	return s, nil
}

// now returns the current time via the service's clock. Tests override
// nowFunc via SetClock so they can control DST, midnight, and overlap
// scenarios deterministically.
func (s *Service) now() time.Time {
	if s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

// SetClock replaces the service's time source. It is intended for tests that
// need to verify DST transitions, midnight boundaries, and overlap behavior
// without waiting on real wall-clock time. Production code must not call it.
func (s *Service) SetClock(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nowFunc = func() time.Time { return t }
}

// SetClockFunc installs a custom time function, for tests that need a
// moving clock (e.g. to simulate a pause expiring between requests).
func (s *Service) SetClockFunc(f func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nowFunc = f
}

// scheduleActive reports whether a group's schedule is active at the
// service's current time. A group with no schedule (nil) is always active,
// preserving pre-PER-39 behavior.
// scheduleActiveAt reports whether a group's schedule is active at an
// arbitrary instant. It is the pure core the live path and preview share, so
// schedule decisions cannot drift between enforcement and preview.
func scheduleActiveAt(group PolicyGroup, now time.Time) bool {
	if group.Schedule == nil {
		return true
	}
	return group.Schedule.IsActive(now)
}

func (s *Service) scheduleActive(group PolicyGroup) bool {
	return scheduleActiveAt(group, s.now())
}

// ErrNoPolicy is returned when no policy document exists. It is distinct
// from a storage error: adapters treat it as "no groups configured".
var ErrNoPolicy = errors.New("no policy document")

// ErrUnknownGroup is returned when an assignment targets a group that is not
// in the persisted policy document.
var ErrUnknownGroup = errors.New("unknown policy group")

// ErrGroupExists is returned when a create operation would overwrite an
// existing policy record.
var ErrGroupExists = errors.New("policy group already exists")

// ErrGroupNotFound is returned when an update or delete targets no group.
var ErrGroupNotFound = errors.New("policy group not found")

func (s *Service) loadDocument() (PolicyDocument, error) {
	var doc PolicyDocument
	if s.storage == nil {
		return doc, nil
	}
	raw, err := s.storage.GetE(StorageKey)
	if err != nil {
		return doc, fmt.Errorf("read policy groups: %w", err)
	}
	if raw == "" {
		return doc, nil
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return doc, fmt.Errorf("parse policy groups: %w", err)
	}
	if doc.Version != DocumentVersion {
		return doc, fmt.Errorf("unsupported policy groups version %d", doc.Version)
	}
	return doc, nil
}

func snapshotFromDocument(doc PolicyDocument) PolicySnapshot {
	snap := PolicySnapshot{
		Groups:      make(map[string]PolicyGroup, len(doc.Groups)),
		Assignments: make(map[string]string, len(doc.Assignments)),
		Version:     doc.Version,
		UpdatedAt:   doc.UpdatedAt,
	}
	for _, g := range doc.Groups {
		snap.Groups[g.ID] = g
	}
	for _, a := range doc.Assignments {
		snap.Assignments[a.DeviceID] = a.GroupID
	}
	return snap
}

func documentFromSnapshot(snap PolicySnapshot, migratedFrom string) PolicyDocument {
	doc := PolicyDocument{
		Version:      DocumentVersion,
		Groups:       make([]PolicyGroup, 0, len(snap.Groups)),
		Assignments:  make([]DeviceAssignment, 0, len(snap.Assignments)),
		MigratedFrom: migratedFrom,
		UpdatedAt:    time.Now().UTC(),
	}
	for _, g := range snap.Groups {
		doc.Groups = append(doc.Groups, g)
	}
	sort.Slice(doc.Groups, func(i, j int) bool {
		if doc.Groups[i].Priority != doc.Groups[j].Priority {
			return doc.Groups[i].Priority < doc.Groups[j].Priority
		}
		return doc.Groups[i].ID < doc.Groups[j].ID
	})
	for deviceID, groupID := range snap.Assignments {
		doc.Assignments = append(doc.Assignments, DeviceAssignment{DeviceID: deviceID, GroupID: groupID})
	}
	sort.Slice(doc.Assignments, func(i, j int) bool {
		return doc.Assignments[i].DeviceID < doc.Assignments[j].DeviceID
	})
	return doc
}

// Snapshot returns a point-in-time immutable view for evaluation.
func (s *Service) Snapshot() PolicySnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Copies: callers must not observe concurrent mutation.
	groups := make(map[string]PolicyGroup, len(s.snapshot.Groups))
	for id, g := range s.snapshot.Groups {
		groups[id] = g
	}
	assignments := make(map[string]string, len(s.snapshot.Assignments))
	for d, g := range s.snapshot.Assignments {
		assignments[d] = g
	}
	return PolicySnapshot{Groups: groups, Assignments: assignments, Version: s.snapshot.Version, UpdatedAt: s.snapshot.UpdatedAt}
}

// Reload re-reads the persisted document. On error the previous snapshot is
// retained and the error is returned, so a transient read failure cannot
// silently change live enforcement.
func (s *Service) Reload() error {
	doc, err := s.loadDocument()
	if err != nil {
		return err
	}
	if err := s.loadExceptions(); err != nil {
		return err
	}
	if err := s.loadPauses(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = snapshotFromDocument(doc)
	return nil
}

// SaveGroups replaces the group set atomically.
func (s *Service) SaveGroups(groups []PolicyGroup) error {
	return s.update(func(snap *PolicySnapshot) error {
		next := make(map[string]PolicyGroup, len(groups))
		for _, g := range groups {
			next[g.ID] = g
		}
		snap.Groups = next
		return nil
	})
}

// CreateGroup creates one ordinary policy record without replacing any other
// group. Template application uses this so reapplying a starter cannot
// silently overwrite edits made after the first application.
func (s *Service) CreateGroup(group PolicyGroup) error {
	if strings.TrimSpace(group.ID) == "" {
		return errors.New("policy group needs an id")
	}
	if strings.TrimSpace(group.Name) == "" {
		return errors.New("policy group needs a name")
	}
	now := time.Now().UTC()
	if group.CreatedAt.IsZero() {
		group.CreatedAt = now
	}
	group.UpdatedAt = now
	return s.update(func(snap *PolicySnapshot) error {
		if _, exists := snap.Groups[group.ID]; exists {
			return fmt.Errorf("%w: %s", ErrGroupExists, group.ID)
		}
		snap.Groups[group.ID] = group
		return nil
	})
}

// UpdateGroup edits one ordinary policy record while preserving its stable ID
// and creation time. Other groups and assignments remain in the transaction.
func (s *Service) UpdateGroup(groupID string, group PolicyGroup) error {
	if strings.TrimSpace(groupID) == "" {
		return errors.New("policy group needs an id")
	}
	if strings.TrimSpace(group.Name) == "" {
		return errors.New("policy group needs a name")
	}
	return s.update(func(snap *PolicySnapshot) error {
		current, exists := snap.Groups[groupID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
		}
		group.ID = groupID
		group.CreatedAt = current.CreatedAt
		group.UpdatedAt = time.Now().UTC()
		snap.Groups[groupID] = group
		return nil
	})
}

// DeleteGroup removes one policy record and assignments that point to it.
func (s *Service) DeleteGroup(groupID string) error {
	if strings.TrimSpace(groupID) == "" {
		return errors.New("policy group needs an id")
	}
	return s.update(func(snap *PolicySnapshot) error {
		if _, exists := snap.Groups[groupID]; !exists {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
		}
		delete(snap.Groups, groupID)
		for deviceID, assignedGroupID := range snap.Assignments {
			if assignedGroupID == groupID {
				delete(snap.Assignments, deviceID)
			}
		}
		return nil
	})
}

// SaveAssignments replaces device→group assignments atomically.
func (s *Service) SaveAssignments(assignments []DeviceAssignment) error {
	return s.update(func(snap *PolicySnapshot) error {
		next := make(map[string]string, len(assignments))
		for _, a := range assignments {
			next[a.DeviceID] = a.GroupID
		}
		snap.Assignments = next
		return nil
	})
}

// SetDeviceAssignment assigns one device to a group, or clears the assignment
// when groupID is empty. The read, validation, and write happen inside a
// single storage transaction, so concurrent edits to other device assignments
// cannot be lost and an invalid group aborts without writing.
func (s *Service) SetDeviceAssignment(deviceID, groupID string) error {
	if deviceID == "" {
		return errors.New("policy assignment needs a device id")
	}
	return s.update(func(snap *PolicySnapshot) error {
		if groupID == "" {
			delete(snap.Assignments, deviceID)
			return nil
		}
		if _, exists := snap.Groups[groupID]; !exists {
			return fmt.Errorf("%w: %s", ErrUnknownGroup, groupID)
		}
		snap.Assignments[deviceID] = groupID
		return nil
	})
}

// SaveMigration persists a complete one-time migration as a single atomic
// write. It refuses to clobber a document that already exists or appears
// concurrently, so re-running migration can never discard API edits, and a
// crash cannot leave a half-applied migration behind.
func (s *Service) SaveMigration(groups []PolicyGroup, assignments []DeviceAssignment, migratedFrom string) error {
	if s.storage == nil {
		return errors.New("policy service has no storage")
	}
	snap := PolicySnapshot{
		Groups:      make(map[string]PolicyGroup, len(groups)),
		Assignments: make(map[string]string, len(assignments)),
	}
	for _, group := range groups {
		snap.Groups[group.ID] = group
	}
	for _, assignment := range assignments {
		snap.Assignments[assignment.DeviceID] = assignment.GroupID
	}
	doc := documentFromSnapshot(snap, migratedFrom)
	encoded, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode policy groups: %w", err)
	}
	return s.storage.UpdateValue(StorageKey, func(current string) (string, error) {
		if strings.TrimSpace(current) != "" {
			// A policy document already exists (earlier migration or an API
			// write racing this one). Migration is one-shot: keep it intact.
			return current, nil
		}
		return string(encoded), nil
	})
}

// update runs transform inside one atomic storage transaction. A non-nil
// transform error aborts the transaction and leaves the persisted document
// untouched.
func (s *Service) update(transform func(*PolicySnapshot) error) error {
	if s.storage == nil {
		return errors.New("policy service has no storage")
	}
	return s.storage.UpdateValue(StorageKey, func(current string) (string, error) {
		var doc PolicyDocument
		if current != "" {
			if err := json.Unmarshal([]byte(current), &doc); err != nil {
				return "", fmt.Errorf("parse policy groups: %w", err)
			}
			if doc.Version != DocumentVersion {
				return "", fmt.Errorf("unsupported policy groups version %d", doc.Version)
			}
		}
		snap := snapshotFromDocument(doc)
		if err := transform(&snap); err != nil {
			return "", err
		}
		nextDoc := documentFromSnapshot(snap, doc.MigratedFrom)
		encoded, err := json.Marshal(nextDoc)
		if err != nil {
			return "", fmt.Errorf("encode policy groups: %w", err)
		}
		return string(encoded), nil
	})
}

// ResolveIdentity maps request context to a policy identity. IP is used only
// as a lookup key into observed devices. Authenticated user identity is kept
// distinct: an authenticated user is never assumed to be a device, and a
// device observation is never treated as authentication.
// It applies the strict staleness rule used by adapters where the request
// itself is not evidence that the address is currently active (the proxy
// paths): a stale observation falls back to the default policy because the
// address may have been reassigned to a different device.
func (s *Service) ResolveIdentity(clientIP, authUser string) Identity {
	return s.resolveIdentity(clientIP, authUser, false)
}

// ResolveIdentityForDNS maps request context for the DNS adapter. A live DNS
// query from an address is direct evidence that the address is active, so a
// stale LastSeen does not downgrade this request's identity to unknown;
// discovery's current IP attribution is used and the NAT/shared-address
// ambiguity guard still applies. (Staleness-based fallback remains the rule
// for adapters without per-request evidence; see ResolveIdentity.)
func (s *Service) ResolveIdentityForDNS(clientIP, authUser string) Identity {
	return s.resolveIdentity(clientIP, authUser, true)
}

// resolveBaseIdentity resolves device/user/source from the live device store
// without binding a policy group, so the group can be derived per snapshot
// (proposed vs active) by preview. requestConfirmsAddress selects the DNS
// path, where a live query is evidence the address is active and a stale
// observation is still attributed rather than downgraded.
func (s *Service) resolveBaseIdentity(clientIP, authUser string, requestConfirmsAddress bool) Identity {
	// The unauthenticated explicit proxy passes the client address as the
	// user fallback. An address is a lookup key, never an authenticated
	// identity, so it must not take the authenticated-user path.
	if isAddressLike(authUser) {
		authUser = ""
	}
	identity := Identity{AuthUser: authUser, Source: SourceUnknown}
	switch {
	case authUser != "":
		identity.Source = SourceAuthUser
		identity.Explanation = "authenticated proxy user"
	case clientIP == "" || s.devices == nil:
		identity.Source = SourceUnknown
		identity.Explanation = "no client address available"
	default:
		deviceID, ambiguous, stale := s.devices.ResolveDeviceByIP(clientIP)
		switch {
		case ambiguous:
			identity.Source = SourceUnknownNAT
			identity.Explanation = "multiple devices share this address; treating as unknown"
		case stale:
			if requestConfirmsAddress {
				identity.DeviceID = deviceID
				identity.Source = SourceDevice
				identity.Explanation = "device attributed from observed address; live query confirms the address is active"
			} else {
				identity.DeviceID = deviceID
				identity.Source = SourceStaleDevice
				identity.Explanation = "device observation is stale; using default policy"
			}
		case deviceID == "":
			identity.Source = SourceUnknown
			identity.Explanation = "no device observed for this address"
		default:
			identity.DeviceID = deviceID
			identity.Source = SourceDevice
			identity.Explanation = "device resolved from observed address"
		}
	}
	return identity
}

func (s *Service) resolveIdentity(clientIP, authUser string, requestConfirmsAddress bool) Identity {
	identity := s.resolveBaseIdentity(clientIP, authUser, requestConfirmsAddress)
	identity.GroupID = groupIDForSnapshot(identity, s.Snapshot())
	return identity
}

// isAddressLike reports whether a user value is an IP address rather than a
// real username. The explicit proxy uses the client address as the
// unauthenticated user fallback, so such values must be ignored here.
func isAddressLike(user string) bool {
	if user == "" {
		return false
	}
	if net.ParseIP(user) != nil {
		return true
	}
	if host, _, err := net.SplitHostPort(user); err == nil {
		return net.ParseIP(host) != nil
	}
	return false
}

// groupForUserIn resolves the winning group for an authenticated user from a
// snapshot, choosing the lowest priority (then lowest ID for determinism). It
// is the pure core shared by live identity resolution and preview.
func groupForUserIn(snap PolicySnapshot, user string) string {
	var best string
	bestPriority := int(^uint(0) >> 1)
	for _, group := range snap.Groups {
		for _, candidate := range group.Users {
			if candidate != user {
				continue
			}
			if group.Priority < bestPriority || (group.Priority == bestPriority && best != "" && group.ID < best) {
				best = group.ID
				bestPriority = group.Priority
			}
		}
	}
	return best
}

func (s *Service) groupForUser(snap PolicySnapshot, user string) string {
	return groupForUserIn(snap, user)
}

// groupIDForSnapshot derives the policy group for a base identity (DeviceID,
// AuthUser, Source already resolved) from a given snapshot. Only authenticated
// users and confirmed devices resolve to a group; unknown, NAT-ambiguous, and
// stale-proxy observations resolve to none. Used per-snapshot so a proposed
// assignment is honored without re-resolving device identity.
func groupIDForSnapshot(identity Identity, snap PolicySnapshot) string {
	switch identity.Source {
	case SourceAuthUser:
		return groupForUserIn(snap, identity.AuthUser)
	case SourceDevice:
		return snap.Assignments[identity.DeviceID]
	default:
		return ""
	}
}

// EvaluateDNS applies group policy and exceptions to a DNS query. It only
// reports DNS-applicable outcomes; URL, MIME, and TLS-inspection conditions
// are surfaced as explicitly inapplicable rather than being silently treated
// as enforced.
//
// Exception precedence: a scoped exception (device > group > installation)
// that matches the domain exempts it from the group block. This lets an
// administrator recover a false positive for one device without creating a
// global bypass.
// evaluateDNSAt is the pure, snapshot-parametrized core of EvaluateDNS. The
// live path and preview both call it so DNS enforcement and preview cannot
// diverge: there is one evaluator, not a parallel one.
func (s *Service) evaluateDNSAt(identity Identity, domain string, snap PolicySnapshot, exc ExceptionSnapshot, pa PauseSnapshot, now time.Time) DNSDecision {
	decision := DNSDecision{Action: ActionNone}

	// Check scoped exceptions first. An active exception that matches the
	// domain reports an allow so the caller can bypass the global blocklist,
	// regardless of group policy.
	if e, ok := evaluateExceptionIn(identity, domain, exc); ok {
		decision.Action = ActionAllow
		decision.Reason = "scoped exception: " + string(e.Scope)
		// GroupID left empty for exception bypasses; Reason carries the scope
		decision.InapplicableConditions = dnsInapplicableConditions()
		return decision
	}

	group, ok := snap.Groups[identity.GroupID]
	if !ok {
		decision.InapplicableConditions = dnsInapplicableConditions()
		return decision
	}
	decision.GroupID = group.ID
	// If the group has a schedule and it is not currently active, the group
	// expresses no opinion: the block action does not apply and enforcement
	// falls through to the global blocklist. DNS cache and established
	// connections may retain prior decisions; see schedule documentation.
	if !scheduleActiveAt(group, now) {
		decision.Reason = "group schedule inactive"
		decision.InapplicableConditions = dnsInapplicableConditions()
		return decision
	}
	// An active pause suppresses the group block action for the paused scope
	// (device > group > installation). Pauses do not override allow actions:
	// an allow group stays allow. DNS caches and established connections
	// may retain prior decisions until they expire.
	if p, ok := evaluatePauseIn(identity, pa); ok && group.Action == ActionBlock {
		decision.Action = ActionNone
		decision.Reason = "paused: " + string(p.Scope)
		decision.InapplicableConditions = dnsInapplicableConditions()
		return decision
	}
	for _, pattern := range group.Domains {
		if matchDomain(pattern, domain) {
			decision.MatchedDomain = pattern
			decision.Action = group.Action
			decision.Reason = "group policy"
			break
		}
	}
	decision.InapplicableConditions = dnsInapplicableConditions()
	return decision
}

func (s *Service) EvaluateDNS(identity Identity, domain string) DNSDecision {
	return s.evaluateDNSAt(identity, domain, s.Snapshot(), s.ExceptionSnapshot(), s.PauseSnapshot(), s.now())
}

func dnsInapplicableConditions() []string {
	return []string{"url_regex", "content_type", "mitm"}
}

// DNSInapplicableConditions exposes the conditions DNS enforcement cannot
// evaluate. APIs and logs use it so the public limitation always matches the
// policy engine rather than duplicating the list.
func DNSInapplicableConditions() []string {
	return dnsInapplicableConditions()
}

// EvaluateDomain reports the group action for a domain in any adapter.
// A scoped exception (device > group > installation) overrides a group
// block with an allow, so a false positive can be recovered for one entity
// without a global bypass.
// evaluateDomainAt is the pure, snapshot-parametrized core of EvaluateDomain,
// sharing the evaluator with preview so the two cannot diverge.
func (s *Service) evaluateDomainAt(identity Identity, domain string, snap PolicySnapshot, exc ExceptionSnapshot, pa PauseSnapshot, now time.Time) PolicyAction {
	if _, ok := evaluateExceptionIn(identity, domain, exc); ok {
		return ActionAllow
	}
	group, ok := snap.Groups[identity.GroupID]
	if !ok {
		return ActionNone
	}
	// A schedule that is not currently active means the group expresses no
	// opinion about this domain right now. Enforcement falls through.
	if !scheduleActiveAt(group, now) {
		return ActionNone
	}
	// An active pause suppresses the group block action for the paused scope.
	// Allow actions are not affected.
	if _, ok := evaluatePauseIn(identity, pa); ok && group.Action == ActionBlock {
		return ActionNone
	}
	for _, pattern := range group.Domains {
		if matchDomain(pattern, domain) {
			return group.Action
		}
	}
	return ActionNone
}

func (s *Service) EvaluateDomain(identity Identity, domain string) PolicyAction {
	return s.evaluateDomainAt(identity, domain, s.Snapshot(), s.ExceptionSnapshot(), s.PauseSnapshot(), s.now())
}

// matchDomain mirrors the rules engine wildcard semantics: exact match or
// "*." suffix match including the bare suffix.
func matchDomain(pattern, domain string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	domain = strings.ToLower(strings.TrimSpace(domain))
	if pattern == domain {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:]
		return domain == suffix || strings.HasSuffix(domain, "."+suffix)
	}
	return false
}

func newGroupID() string {
	return gatesentryUtils.RandomString(16)
}

// EnsureGroupID assigns an ID to a group that lacks one. Mutation helpers use
// it so persisted records always have stable identifiers.
func EnsureGroupID(group *PolicyGroup) {
	if group != nil && group.ID == "" {
		group.ID = newGroupID()
	}
}
