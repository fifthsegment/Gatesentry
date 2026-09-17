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
	return s, nil
}

// ErrNoPolicy is returned when no policy document exists. It is distinct
// from a storage error: adapters treat it as "no groups configured".
var ErrNoPolicy = errors.New("no policy document")

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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = snapshotFromDocument(doc)
	return nil
}

// SaveGroups replaces the group set atomically.
func (s *Service) SaveGroups(groups []PolicyGroup) error {
	return s.update(func(snap *PolicySnapshot) {
		next := make(map[string]PolicyGroup, len(groups))
		for _, g := range groups {
			next[g.ID] = g
		}
		snap.Groups = next
	})
}

// SaveAssignments replaces device→group assignments atomically.
func (s *Service) SaveAssignments(assignments []DeviceAssignment) error {
	return s.update(func(snap *PolicySnapshot) {
		next := make(map[string]string, len(assignments))
		for _, a := range assignments {
			next[a.DeviceID] = a.GroupID
		}
		snap.Assignments = next
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

func (s *Service) update(transform func(*PolicySnapshot)) error {
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
		transform(&snap)
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

func (s *Service) resolveIdentity(clientIP, authUser string, requestConfirmsAddress bool) Identity {
	snap := s.Snapshot()
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
		identity.GroupID = s.groupForUser(snap, authUser)
		identity.Explanation = "authenticated proxy user"
	case clientIP == "" || s.devices == nil:
		identity.Source = SourceUnknown
		identity.Explanation = "no client address available"
	default:
		deviceID, ambiguous, stale := s.devices.ResolveDeviceByIP(clientIP)
		switch {
		case ambiguous:
			identity.Source = SourceUnknownNAT
			identity.GroupID = ""
			identity.Explanation = "multiple devices share this address; treating as unknown"
		case stale:
			if requestConfirmsAddress {
				identity.DeviceID = deviceID
				identity.Source = SourceDevice
				identity.GroupID = snap.Assignments[deviceID]
				identity.Explanation = "device attributed from observed address; live query confirms the address is active"
			} else {
				identity.DeviceID = deviceID
				identity.Source = SourceStaleDevice
				identity.GroupID = ""
				identity.Explanation = "device observation is stale; using default policy"
			}
		case deviceID == "":
			identity.Source = SourceUnknown
			identity.GroupID = ""
			identity.Explanation = "no device observed for this address"
		default:
			identity.DeviceID = deviceID
			identity.Source = SourceDevice
			identity.GroupID = snap.Assignments[deviceID]
			identity.Explanation = "device resolved from observed address"
		}
	}
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

func (s *Service) groupForUser(snap PolicySnapshot, user string) string {
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

// EvaluateDNS applies group policy to a DNS query. It only reports
// DNS-applicable outcomes; URL, MIME, and TLS-inspection conditions are
// surfaced as explicitly inapplicable rather than being silently treated as
// enforced.
func (s *Service) EvaluateDNS(identity Identity, domain string) DNSDecision {
	snap := s.Snapshot()
	decision := DNSDecision{Action: ActionNone}
	group, ok := snap.Groups[identity.GroupID]
	if !ok {
		return decision
	}
	decision.GroupID = group.ID
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

func dnsInapplicableConditions() []string {
	return []string{"url_regex", "content_type", "mitm"}
}

// EvaluateDomain reports the group action for a domain in any adapter.
func (s *Service) EvaluateDomain(identity Identity, domain string) PolicyAction {
	snap := s.Snapshot()
	group, ok := snap.Groups[identity.GroupID]
	if !ok {
		return ActionNone
	}
	for _, pattern := range group.Domains {
		if matchDomain(pattern, domain) {
			return group.Action
		}
	}
	return ActionNone
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
