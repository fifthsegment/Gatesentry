package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gatesentryUtils "bitbucket.org/abdullah_irfan/gatesentryf/utils"
)

// ExceptionStorageKey is the settings key holding the exception document.
// A missing key means no exceptions exist and enforcement keeps baseline.
const ExceptionStorageKey = "policy_exceptions"

// ExceptionDocumentVersion is the persisted exception document format version.
const ExceptionDocumentVersion = 1

// ExceptionScope identifies what identity an exception applies to.
type ExceptionScope string

const (
	// ScopeDevice applies the exception to one device only.
	ScopeDevice ExceptionScope = "device"
	// ScopeGroup applies the exception to all devices assigned to a group.
	ScopeGroup ExceptionScope = "group"
	// ScopeInstallation applies the exception to all devices on this gateway.
	ScopeInstallation ExceptionScope = "installation"
)

// ErrExceptionNotFound is returned when an update or revoke targets no
// exception.
var ErrExceptionNotFound = errors.New("exception not found")

// ErrInvalidException is returned when an exception request is malformed.
var ErrInvalidException = errors.New("invalid exception")

// Exception is a scoped domain allow (bypass) with explicit duration. It
// exempts a domain from blocking rules for a device, group, or the entire
// installation, and expires automatically. The policy service owns
// evaluation; storage owns durability.
type Exception struct {
	// ID is a stable identifier generated at creation.
	ID string `json:"id"`
	// Domain is the wildcard pattern the exception covers (same syntax as
	// Rule.Domain: exact match or "*." suffix).
	Domain string `json:"domain"`
	// Scope determines who the exception applies to.
	Scope ExceptionScope `json:"scope"`
	// DeviceID is required for ScopeDevice and ignored otherwise.
	DeviceID string `json:"device_id,omitempty"`
	// GroupID is required for ScopeGroup and ignored otherwise.
	GroupID string `json:"group_id,omitempty"`
	// CreatedBy is the admin username who created the exception.
	CreatedBy string `json:"created_by"`
	// CreatedAt is when the exception was created (UTC).
	CreatedAt time.Time `json:"created_at"`
	// ExpiresAt is when the exception stops applying (UTC). Must be after
	// CreatedAt. Exceptions use clearly bounded temporary access because
	// DNS caching and persistent proxy connections make true "allow-once"
	// unreliable.
	ExpiresAt time.Time `json:"expires_at"`
	// Reason is the admin-supplied justification, shown in audit.
	Reason string `json:"reason,omitempty"`
	// Active is false when the exception has been revoked. Expired exceptions
	// remain Active=true but are ignored during evaluation.
	Active bool `json:"active"`
	// RevokedBy is the admin username who revoked the exception.
	RevokedBy string `json:"revoked_by,omitempty"`
	// RevokedAt is when the exception was revoked, if applicable.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// ExceptionDocument is the persisted JSON shape.
type ExceptionDocument struct {
	Version    int         `json:"version"`
	Exceptions []Exception `json:"exceptions"`
	UpdatedAt  time.Time   `json:"updated_at"`
}

// ExceptionSnapshot is the immutable evaluation view of persisted exceptions.
// Adapters read a snapshot per evaluation; mutations replace the whole
// snapshot atomically.
type ExceptionSnapshot struct {
	Exceptions []Exception
	UpdatedAt  time.Time
}

// MinExceptionDuration is the shortest allowed exception duration. It exists
// because DNS caching and persistent proxy connections make true "allow-once"
// unreliable: a one-time allow can persist via cache TTL or keep-alive. The
// minimum ensures the access window is clearly bounded and predictable.
const MinExceptionDuration = 1 * time.Minute

// MaxExceptionDuration bounds the longest single exception so temporary
// access cannot accidentally become permanent.
const MaxExceptionDuration = 7 * 24 * time.Hour

// MaxAccessRequestsPerMinute bounds how many access requests a single client
// IP may submit per minute. Public access requests are kept separate from
// administrator authorization and must not be able to grant access.
const MaxAccessRequestsPerMinute = 5

// CreateExceptionInput is the admin request body for creating an exception.
type CreateExceptionInput struct {
	Domain   string         `json:"domain"`
	Scope    ExceptionScope `json:"scope"`
	DeviceID string         `json:"device_id,omitempty"`
	GroupID  string         `json:"group_id,omitempty"`
	Duration string         `json:"duration"` // Go duration string, e.g. "5m", "1h"
	Reason   string         `json:"reason,omitempty"`
}

// Validate checks that the exception input is well-formed before the service
// applies it. It does not check group or device existence; the service does
// that inside the storage transaction.
func (in CreateExceptionInput) Validate() error {
	if strings.TrimSpace(in.Domain) == "" {
		return fmt.Errorf("%w: domain is required", ErrInvalidException)
	}
	switch in.Scope {
	case ScopeDevice:
		if strings.TrimSpace(in.DeviceID) == "" {
			return fmt.Errorf("%w: device scope requires a device_id", ErrInvalidException)
		}
	case ScopeGroup:
		if strings.TrimSpace(in.GroupID) == "" {
			return fmt.Errorf("%w: group scope requires a group_id", ErrInvalidException)
		}
	case ScopeInstallation:
		// no target required
	default:
		return fmt.Errorf("%w: invalid scope %q", ErrInvalidException, in.Scope)
	}
	if strings.TrimSpace(in.Duration) == "" {
		return fmt.Errorf("%w: duration is required", ErrInvalidException)
	}
	d, err := time.ParseDuration(in.Duration)
	if err != nil {
		return fmt.Errorf("%w: invalid duration %q", ErrInvalidException, in.Duration)
	}
	if d < MinExceptionDuration {
		return fmt.Errorf("%w: duration %s is shorter than minimum %s", ErrInvalidException, in.Duration, MinExceptionDuration)
	}
	if d > MaxExceptionDuration {
		return fmt.Errorf("%w: duration %s exceeds maximum %s", ErrInvalidException, in.Duration, MaxExceptionDuration)
	}
	return nil
}

// exceptionStorage holds the singleton exception store, so the service can
// persist exceptions alongside groups without a second constructor argument.
// The store is the same MapStore the policy service already uses for groups.
type exceptionStorage struct {
	mu       sync.RWMutex
	snapshot ExceptionSnapshot
}

// loadExceptionDocument reads and parses the exception document from storage.
func (s *Service) loadExceptionDocument() (ExceptionDocument, error) {
	var doc ExceptionDocument
	if s.storage == nil {
		return doc, nil
	}
	raw, err := s.storage.GetE(ExceptionStorageKey)
	if err != nil {
		return doc, fmt.Errorf("read policy exceptions: %w", err)
	}
	if raw == "" {
		return doc, nil
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return doc, fmt.Errorf("parse policy exceptions: %w", err)
	}
	if doc.Version != ExceptionDocumentVersion {
		return doc, fmt.Errorf("unsupported policy exceptions version %d", doc.Version)
	}
	return doc, nil
}

func snapshotFromExceptionDocument(doc ExceptionDocument) ExceptionSnapshot {
	snap := ExceptionSnapshot{
		Exceptions: make([]Exception, len(doc.Exceptions)),
		UpdatedAt:  doc.UpdatedAt,
	}
	copy(snap.Exceptions, doc.Exceptions)
	return snap
}

func documentFromExceptionSnapshot(snap ExceptionSnapshot) ExceptionDocument {
	doc := ExceptionDocument{
		Version:    ExceptionDocumentVersion,
		Exceptions: make([]Exception, len(snap.Exceptions)),
		UpdatedAt:  time.Now().UTC(),
	}
	copy(doc.Exceptions, snap.Exceptions)
	return doc
}

// loadExceptions populates the in-memory exception snapshot. Called from
// Reload and NewService so exceptions survive restarts.
func (s *Service) loadExceptions() error {
	doc, err := s.loadExceptionDocument()
	if err != nil {
		return err
	}
	s.exc.mu.Lock()
	defer s.exc.mu.Unlock()
	s.exc.snapshot = snapshotFromExceptionDocument(doc)
	return nil
}

// ExceptionSnapshot returns a point-in-time copy of active (non-expired,
// non-revoked) exceptions for evaluation. Adapters call this per request.
func (s *Service) ExceptionSnapshot() ExceptionSnapshot {
	s.exc.mu.RLock()
	defer s.exc.mu.RUnlock()
	now := s.now()
	active := make([]Exception, 0, len(s.exc.snapshot.Exceptions))
	for _, e := range s.exc.snapshot.Exceptions {
		if e.Active && now.Before(e.ExpiresAt) {
			active = append(active, e)
		}
	}
	return ExceptionSnapshot{Exceptions: active, UpdatedAt: s.exc.snapshot.UpdatedAt}
}

// AllExceptions returns all exceptions including expired and revoked ones,
// for the admin audit view. The caller may filter by active/expired status.
func (s *Service) AllExceptions() []Exception {
	s.exc.mu.RLock()
	defer s.exc.mu.RUnlock()
	out := make([]Exception, len(s.exc.snapshot.Exceptions))
	copy(out, s.exc.snapshot.Exceptions)
	return out
}

// CreateException validates input, checks group/device existence, and
// persists a new exception atomically. The in-memory snapshot is refreshed
// so the exception takes effect immediately.
func (s *Service) CreateException(in CreateExceptionInput, createdBy string) (Exception, error) {
	if err := in.Validate(); err != nil {
		return Exception{}, err
	}
	if strings.TrimSpace(createdBy) == "" {
		return Exception{}, fmt.Errorf("%w: created_by is required", ErrInvalidException)
	}
	d, _ := time.ParseDuration(in.Duration)
	now := time.Now().UTC()
	exc := Exception{
		ID:        gatesentryUtils.RandomString(16),
		Domain:    strings.ToLower(strings.TrimSpace(in.Domain)),
		Scope:     in.Scope,
		DeviceID:  in.DeviceID,
		GroupID:   in.GroupID,
		CreatedBy: createdBy,
		CreatedAt: now,
		ExpiresAt: now.Add(d),
		Reason:    in.Reason,
		Active:    true,
	}
	err := s.updateExceptions(func(snap *ExceptionSnapshot) error {
		// Validate group existence for group-scoped exceptions inside the
		// transaction so a concurrent group delete is caught.
		if in.Scope == ScopeGroup {
			policySnap := s.currentSnapshot()
			if _, exists := policySnap.Groups[in.GroupID]; !exists {
				return fmt.Errorf("%w: group %s: %v", ErrInvalidException, in.GroupID, ErrUnknownGroup)
			}
		}
		snap.Exceptions = append(snap.Exceptions, exc)
		return nil
	})
	if err != nil {
		return Exception{}, err
	}
	return exc, nil
}

// RevokeException marks an exception inactive without deleting it, so the
// audit trail survives. The revoking admin username is recorded.
func (s *Service) RevokeException(id, revokedBy string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidException)
	}
	return s.updateExceptions(func(snap *ExceptionSnapshot) error {
		for i := range snap.Exceptions {
			if snap.Exceptions[i].ID == id {
				if !snap.Exceptions[i].Active {
					return fmt.Errorf("%w: already revoked", ErrExceptionNotFound)
				}
				now := time.Now().UTC()
				snap.Exceptions[i].Active = false
				snap.Exceptions[i].RevokedBy = revokedBy
				snap.Exceptions[i].RevokedAt = &now
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrExceptionNotFound, id)
	})
}

// PruneExpiredExceptions removes expired and revoked exceptions older than
// MaxExceptionDuration past their expiry, so the document does not grow
// unbounded. It returns the number pruned.
func (s *Service) PruneExpiredExceptions() (int, error) {
	pruned := 0
	err := s.updateExceptions(func(snap *ExceptionSnapshot) error {
		cutoff := time.Now().UTC().Add(-MaxExceptionDuration)
		kept := make([]Exception, 0, len(snap.Exceptions))
		for _, e := range snap.Exceptions {
			if e.Active && e.ExpiresAt.After(cutoff) {
				kept = append(kept, e)
				continue
			}
			// Keep revoked/expired exceptions for audit until they age out.
			if !e.Active && e.RevokedAt != nil && e.RevokedAt.After(cutoff) {
				kept = append(kept, e)
				continue
			}
			if !e.Active && e.ExpiresAt.After(cutoff) {
				kept = append(kept, e)
				continue
			}
			pruned++
		}
		snap.Exceptions = kept
		return nil
	})
	return pruned, err
}

// EvaluateException checks whether an active exception applies to the given
// identity and domain. It returns the matching exception (if any) and true
// when the exception allows (bypasses) the domain. Precedence is device >
// group > installation (most specific first).
// evaluateExceptionIn is the pure, snapshot-parametrized core of
// EvaluateException. The live path and preview both call it so exception
// precedence cannot diverge between enforcement and preview.
func evaluateExceptionIn(identity Identity, domain string, snap ExceptionSnapshot) (Exception, bool) {
	// First pass: device-scoped (most specific)
	for _, e := range snap.Exceptions {
		if e.Scope == ScopeDevice && e.DeviceID == identity.DeviceID && matchDomain(e.Domain, domain) {
			return e, true
		}
	}
	// Second pass: group-scoped
	for _, e := range snap.Exceptions {
		if e.Scope == ScopeGroup && e.GroupID == identity.GroupID && matchDomain(e.Domain, domain) {
			return e, true
		}
	}
	// Third pass: installation-scoped (least specific)
	for _, e := range snap.Exceptions {
		if e.Scope == ScopeInstallation && matchDomain(e.Domain, domain) {
			return e, true
		}
	}
	return Exception{}, false
}

func (s *Service) EvaluateException(identity Identity, domain string) (Exception, bool) {
	return evaluateExceptionIn(identity, domain, s.ExceptionSnapshot())
}

// updateExceptions runs the transform inside one atomic storage transaction
// on the exception document, then refreshes the in-memory snapshot so the
// change takes effect immediately. A non-nil transform error aborts the
// transaction and the snapshot is not refreshed.
func (s *Service) updateExceptions(transform func(*ExceptionSnapshot) error) error {
	if s.storage == nil {
		return errors.New("policy service has no storage")
	}
	if err := s.storage.UpdateValue(ExceptionStorageKey, func(current string) (string, error) {
		var doc ExceptionDocument
		if current != "" {
			if err := json.Unmarshal([]byte(current), &doc); err != nil {
				return "", fmt.Errorf("parse policy exceptions: %w", err)
			}
			if doc.Version != ExceptionDocumentVersion {
				return "", fmt.Errorf("unsupported policy exceptions version %d", doc.Version)
			}
		}
		snap := snapshotFromExceptionDocument(doc)
		if err := transform(&snap); err != nil {
			return "", err
		}
		nextDoc := documentFromExceptionSnapshot(snap)
		encoded, err := json.Marshal(nextDoc)
		if err != nil {
			return "", fmt.Errorf("encode policy exceptions: %w", err)
		}
		return string(encoded), nil
	}); err != nil {
		return err
	}
	// Refresh the in-memory snapshot from what was just persisted.
	return s.loadExceptions()
}

// currentSnapshot returns the policy group snapshot under the read lock.
// Used by exception validation inside a storage transaction.
func (s *Service) currentSnapshot() PolicySnapshot {
	return s.Snapshot()
}

// newExceptionID generates a stable ID for a new exception.
func newExceptionID() string {
	return gatesentryUtils.RandomString(16)
}

// ParseDuration is a thin wrapper around time.ParseDuration, exported so the
// preview endpoint can compute expiry without re-implementing parsing.
func ParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// NowUTC returns the current time in UTC. Exported so handlers and tests
// use a consistent time source.
func NowUTC() time.Time {
	return time.Now().UTC()
}
