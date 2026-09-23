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
	// Version 1 stored one action per group over shared domains and
	// categories; version 2 stores blocked and allowed lists plus targeted
	// rules. loadDocument upgrades a version 1 document in memory and the
	// next write persists it as version 2.
	DocumentVersion = 2
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
	// categories is the downloaded per-category domain index. Startup attaches
	// it once before the DNS listener accepts queries; the blocklist refresh is
	// the only writer of its contents. A nil index means no category data is
	// loaded yet, and category rules match nothing.
	categories *CategoryIndex
	// enabledCategories caches the gateway-wide category selection. The settings
	// store revalidates against disk on every read, so the DNS adapter cannot
	// call it once per query; the writes that change the selection and Reload
	// refresh this cache instead.
	enabledCategories []string
	nowFunc           func() time.Time
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
	s.enabledCategories = LoadEnabledCategories(storage)
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
	if s.storage == nil {
		return withDefaultGroup(PolicyDocument{Version: DocumentVersion}), nil
	}
	raw, err := s.storage.GetE(StorageKey)
	if err != nil {
		return PolicyDocument{}, fmt.Errorf("read policy groups: %w", err)
	}
	return decodeDocument(raw)
}

// decodeDocument parses a stored policy document, upgrading an older format
// in memory, and guarantees the default policy exists. The persisted value is
// only rewritten by the next policy write, so reading never changes storage.
func decodeDocument(raw string) (PolicyDocument, error) {
	doc := PolicyDocument{Version: DocumentVersion}
	if strings.TrimSpace(raw) == "" {
		return withDefaultGroup(doc), nil
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return doc, fmt.Errorf("parse policy groups: %w", err)
	}
	switch probe.Version {
	case 1:
		upgraded, err := upgradeDocumentV1(raw)
		if err != nil {
			return doc, err
		}
		doc = upgraded
	case DocumentVersion:
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return doc, fmt.Errorf("parse policy groups: %w", err)
		}
	default:
		return doc, fmt.Errorf("unsupported policy groups version %d", probe.Version)
	}
	return withDefaultGroup(doc), nil
}

// withDefaultGroup adds the default policy when a document has none, so every
// request always resolves to a policy the administrator can see and edit.
func withDefaultGroup(doc PolicyDocument) PolicyDocument {
	for _, group := range doc.Groups {
		if group.ID == DefaultGroupID {
			return doc
		}
	}
	doc.Groups = append(doc.Groups, DefaultGroup())
	return doc
}

// DefaultGroup is the empty default policy created for a new installation.
func DefaultGroup() PolicyGroup {
	return PolicyGroup{
		ID:          DefaultGroupID,
		Name:        "Default",
		Description: "Applies to every device and user that is not assigned to another policy.",
	}
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

// ReferencedCategories returns the category IDs any policy group selects. The
// DNS blocklist refresh downloads exactly these categories plus the ones
// enabled gateway-wide, so a feed nobody selected never occupies memory.
func (s *Service) ReferencedCategories() []string {
	snapshot := s.Snapshot()
	seen := make(map[string]bool)
	ids := make([]string, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		for _, id := range GroupCategories(group) {
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// GroupCategories lists every category a policy refers to: its blocked
// categories and the categories its rules target.
func GroupCategories(group PolicyGroup) []string {
	ids := append([]string(nil), group.BlockedCategories...)
	for _, rule := range group.Rules {
		ids = append(ids, rule.Target.Categories...)
	}
	return ids
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
	enabled := LoadEnabledCategories(s.storage)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = snapshotFromDocument(doc)
	s.enabledCategories = enabled
	return nil
}

// SaveGroups replaces the group set atomically.
func (s *Service) SaveGroups(groups []PolicyGroup) error {
	next := make(map[string]PolicyGroup, len(groups))
	for _, g := range groups {
		if strings.TrimSpace(g.ID) == "" {
			return errors.New("every policy group needs a stable id")
		}
		if strings.TrimSpace(g.Name) == "" {
			g.Name = g.ID
		}
		normalized, err := NormalizeGroup(g)
		if err != nil {
			return &ValidationError{fmt.Errorf("policy %q: %w", g.ID, err)}
		}
		next[g.ID] = normalized
	}
	if _, ok := next[DefaultGroupID]; !ok {
		next[DefaultGroupID] = DefaultGroup()
	}
	return s.update(func(snap *PolicySnapshot) error {
		snap.Groups = map[string]PolicyGroup{}
		for _, g := range next {
			if err := checkUserConflicts(snap, g); err != nil {
				return err
			}
			snap.Groups[g.ID] = g
		}
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
	group, err := NormalizeGroup(group)
	if err != nil {
		return &ValidationError{err}
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
		if err := checkUserConflicts(snap, group); err != nil {
			return err
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
	group, err := NormalizeGroup(group)
	if err != nil {
		return &ValidationError{err}
	}
	return s.update(func(snap *PolicySnapshot) error {
		current, exists := snap.Groups[groupID]
		if !exists {
			return fmt.Errorf("%w: %s", ErrGroupNotFound, groupID)
		}
		group.ID = groupID
		group.CreatedAt = current.CreatedAt
		group.UpdatedAt = time.Now().UTC()
		if groupID == DefaultGroupID && len(group.Users) > 0 {
			return &ValidationError{errors.New("the default policy already applies to every user that has no policy; assign users to another policy")}
		}
		if err := checkUserConflicts(snap, group); err != nil {
			return err
		}
		snap.Groups[groupID] = group
		return nil
	})
}

// ValidationError marks a policy the administrator has to correct, as
// opposed to a storage failure. Its message is safe to show as written.
type ValidationError struct{ Err error }

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

// OrderedGroups returns every policy in display order: the default policy
// first, then by name, so the list does not reshuffle between loads.
func (s *Service) OrderedGroups() []PolicyGroup {
	snapshot := s.Snapshot()
	groups := make([]PolicyGroup, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if (groups[i].ID == DefaultGroupID) != (groups[j].ID == DefaultGroupID) {
			return groups[i].ID == DefaultGroupID
		}
		if groups[i].Name != groups[j].Name {
			return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
		}
		return groups[i].ID < groups[j].ID
	})
	return groups
}

// ErrDefaultGroup is returned when an operation would remove the default
// policy, which every unassigned request relies on.
var ErrDefaultGroup = errors.New("the default policy cannot be deleted")

// ErrUserConflict is returned when a proxy user is listed on two policies.
// A user resolves to one policy, so a second listing would be ignored and the
// administrator would be misled about which rules apply.
var ErrUserConflict = errors.New("proxy user already belongs to another policy")

func checkUserConflicts(snap *PolicySnapshot, group PolicyGroup) error {
	for _, other := range snap.Groups {
		if other.ID == group.ID {
			continue
		}
		for _, user := range group.Users {
			if containsString(other.Users, user) {
				return fmt.Errorf("%w: %s is in %q", ErrUserConflict, user, other.Name)
			}
		}
	}
	return nil
}

// DeleteGroup removes one policy record and assignments that point to it.
// Devices that were assigned to it fall back to the default policy.
func (s *Service) DeleteGroup(groupID string) error {
	if strings.TrimSpace(groupID) == "" {
		return errors.New("policy group needs an id")
	}
	if groupID == DefaultGroupID {
		return ErrDefaultGroup
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
		if groupID == "" || groupID == DefaultGroupID {
			// The default policy is what an unassigned device gets, so an
			// explicit assignment to it is stored as no assignment.
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
	snap.Groups[DefaultGroupID] = DefaultGroup()
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
		doc, err := decodeDocument(current)
		if err != nil {
			return "", err
		}
		snap := snapshotFromDocument(doc)
		if err := transform(&snap); err != nil {
			return "", err
		}
		if _, ok := snap.Groups[DefaultGroupID]; !ok {
			return "", errors.New("the default policy cannot be removed")
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
	case clientIP == "" || s.devices == nil:
		identity.Explanation = "no client address available"
	default:
		deviceID, ambiguous, stale := s.devices.ResolveDeviceByIP(clientIP)
		switch {
		case ambiguous:
			identity.Source = SourceUnknownNAT
			identity.Explanation = "multiple devices share this address; using the default policy"
		case stale && !requestConfirmsAddress:
			identity.DeviceID = deviceID
			identity.Source = SourceStaleDevice
			identity.Explanation = "device observation is stale; using the default policy"
		case deviceID == "":
			identity.Explanation = "no device observed for this address; using the default policy"
		case stale:
			identity.DeviceID = deviceID
			identity.Source = SourceDevice
			identity.Explanation = "device attributed from observed address; live query confirms the address is active"
		default:
			identity.DeviceID = deviceID
			identity.Source = SourceDevice
			identity.Explanation = "device resolved from observed address"
		}
	}
	if authUser != "" {
		// The login is the more specific identity, so its policy wins over
		// the device's when one names it; the device stays recorded for the
		// logs and for the device's own assignment.
		identity.Source = SourceAuthUser
		identity.Explanation = "authenticated proxy user"
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

// groupIDForSnapshot derives the policy for a base identity (DeviceID,
// AuthUser, Source already resolved) from a given snapshot. Precedence is the
// authenticated proxy user's policy, then the confirmed device's assignment,
// then the default policy. Unknown, NAT-ambiguous, and stale-proxy
// observations resolve to the default policy: a request always has a policy,
// and it is never someone else's. Used per snapshot so a proposed assignment
// is honored without re-resolving device identity.
func groupIDForSnapshot(identity Identity, snap PolicySnapshot) string {
	if identity.AuthUser != "" {
		if id := groupForUserIn(snap, identity.AuthUser); id != "" {
			return id
		}
	}
	if identity.DeviceID != "" && (identity.Source == SourceDevice || identity.Source == SourceAuthUser) {
		if id, ok := snap.Assignments[identity.DeviceID]; ok {
			if _, exists := snap.Groups[id]; exists {
				return id
			}
		}
	}
	return DefaultGroupID
}

// evalContext is the state one decision reads. The live path fills it from the
// service; preview fills it from a proposed snapshot, so both run the same
// evaluator.
type evalContext struct {
	snap              PolicySnapshot
	exc               ExceptionSnapshot
	pa                PauseSnapshot
	now               time.Time
	index             *CategoryIndex
	gatewayCategories []string
}

func (s *Service) liveContext() evalContext {
	s.mu.RLock()
	index := s.categories
	gateway := append([]string(nil), s.enabledCategories...)
	s.mu.RUnlock()
	return evalContext{
		snap:              s.Snapshot(),
		exc:               s.ExceptionSnapshot(),
		pa:                s.PauseSnapshot(),
		now:               s.now(),
		index:             index,
		gatewayCategories: gateway,
	}
}

// evaluate is the one decision path. Scoped exceptions come first because they
// are how an administrator recovers one false positive without editing the
// policy; then the resolved policy decides.
func evaluate(identity Identity, domain string, layer DecisionLayer, ctx evalContext) (groupOutcome, PolicyGroup) {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	group, ok := ctx.snap.Groups[identity.GroupID]
	if !ok {
		group = ctx.snap.Groups[DefaultGroupID]
	}
	if e, ok := evaluateExceptionIn(identity, domain, ctx.exc); ok {
		return groupOutcome{
			Action:  ActionAllow,
			Matched: e.Domain,
			Reason:  "exception (" + string(e.Scope) + ")",
			Trace:   []TraceStep{{Stage: "exception", Applied: true, Action: ActionAllow, Detail: string(e.Scope) + " exception for " + e.Domain}},
		}, group
	}
	pause, paused := evaluatePauseIn(identity, ctx.pa)
	out := evaluateGroup(group, evalRequest{
		domain:            domain,
		user:              identity.AuthUser,
		now:               ctx.now,
		layer:             layer,
		paused:            paused,
		index:             ctx.index,
		gatewayCategories: ctx.gatewayCategories,
	})
	if paused && out.Action != ActionBlock && out.Reason == "" {
		// Name the pause when it is what left the request unblocked, so a
		// log line explains why a normally blocked site loaded.
		for _, step := range out.Trace {
			if strings.HasSuffix(step.Detail, ": paused") {
				out.Reason = "paused: " + string(pause.Scope)
				break
			}
		}
	}
	return out, group
}

// EvaluateDNS decides a DNS query. An allow exempts the domain from the global
// blocklist; a block answers with a block response; ActionNone leaves the
// global blocklist in charge. Conditions only the proxy can see are listed in
// ProxyOnly and never decide a DNS answer.
func (s *Service) EvaluateDNS(identity Identity, domain string) DNSDecision {
	return evaluateDNSIn(identity, domain, s.liveContext())
}

func evaluateDNSIn(identity Identity, domain string, ctx evalContext) DNSDecision {
	out, group := evaluate(identity, domain, LayerDNS, ctx)
	return DNSDecision{
		Action:        out.Action,
		GroupID:       group.ID,
		MatchedDomain: out.Matched,
		RuleID:        out.RuleID,
		ProxyOnly:     out.ProxyOnly,
		SafeSearch:    group.SafeSearch,
		Reason:        out.Reason,
	}
}

// EvaluateProxy is the policy decision for a proxy request: whether the policy
// blocks the domain, allows it explicitly, or blocks only the URLs and
// response types its rules name, in which case the proxy has to inspect the
// connection to see them.
//
// Matched false means no policy decided this request: the proxy keeps the
// gateway's own settings, so a policy never silently changes inspection for
// traffic it does not cover.
func (s *Service) EvaluateProxy(identity Identity, domain string) ProxyMatch {
	return evaluateProxyIn(identity, domain, s.liveContext())
}

func evaluateProxyIn(identity Identity, domain string, ctx evalContext) ProxyMatch {
	out, group := evaluate(identity, domain, LayerExplicitProxy, ctx)
	match := ProxyMatch{GroupID: group.ID, RuleID: out.RuleID, MatchedDomain: out.Matched, Reason: out.Reason}
	conditional := len(out.URLRegexes) > 0 || len(out.ContentTypes) > 0
	switch {
	case out.Action == ActionBlock:
		match.Matched = true
		match.ShouldBlock = true
	case conditional:
		match.Matched = true
		match.ShouldMITM = true
		match.BlockURLRegexes = out.URLRegexes
		match.BlockContentTypes = out.ContentTypes
		match.RuleID = out.ConditionRuleID
		match.Reason = out.ConditionReason
		match.Allowed = out.Action == ActionAllow
	case out.Action == ActionAllow:
		match.Matched = true
		match.Allowed = true
	}
	return match
}

// EvaluateDomain reports the policy action for a domain on the DNS layer.
func (s *Service) EvaluateDomain(identity Identity, domain string) PolicyAction {
	return s.EvaluateDNS(identity, domain).Action
}

// matchDomain reports whether a pattern covers a domain. A plain domain covers
// itself and its subdomains, because that is what an administrator who types
// "tiktok.com" means; "*.example.com" covers only the subdomains, for the rare
// case where the apex must stay reachable.
func matchDomain(pattern, domain string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if pattern == "" || domain == "" {
		return false
	}
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(domain, pattern[1:])
	}
	return domain == pattern || strings.HasSuffix(domain, "."+pattern)
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

// Timezone returns the gateway's configured IANA time zone, used to anchor
// schedules created on the gateway's behalf (templates, presets). It falls
// back to UTC, which is also what a schedule without a zone is evaluated in.
func (s *Service) Timezone() string {
	if s == nil || s.storage == nil {
		return "UTC"
	}
	value, err := s.storage.GetE("timezone")
	if err != nil || strings.TrimSpace(value) == "" {
		return "UTC"
	}
	return strings.TrimSpace(value)
}
