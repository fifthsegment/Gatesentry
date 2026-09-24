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

// PauseStorageKey is the settings key holding the pause document.
const PauseStorageKey = "policy_pauses"

// PauseDocumentVersion is the persisted pause document format version.
const PauseDocumentVersion = 1

// ErrPauseNotFound is returned when a revoke or update targets no pause.
var ErrPauseNotFound = errors.New("pause not found")

// ErrInvalidPause is returned when a pause request is malformed.
var ErrInvalidPause = errors.New("invalid pause")

// PauseScope identifies what identity a pause applies to, mirroring the
// exception scope model so a pause can suppress a block for one device, one
// group, or the entire installation.
type PauseScope string

const (
	// PauseScopeDevice suppresses the group block for one device only.
	PauseScopeDevice PauseScope = "device"
	// PauseScopeGroup suppresses the group block for all devices in a group.
	PauseScopeGroup PauseScope = "group"
	// PauseScopeInstallation suppresses the group block for all devices.
	PauseScopeInstallation PauseScope = "installation"
)

// Pause is an authorized temporary suppression of a group's block action. It is
// the mechanism behind "pause-until": an admin (or a user with pause
// permission) can temporarily lift blocking for a device, group, or the whole
// installation until a specified time. Unlike an exception, a pause does not
// target a specific domain; it suppresses the block action itself for the
// scoped identity so all domains in the group pass during the pause window.
//
// Pauses are durable and survive restarts. They expire automatically; the
// in-memory snapshot is checked at evaluation time so an expired pause takes
// no effect even if the expiry time has passed since the last reload.
type Pause struct {
	// ID is a stable identifier generated at creation.
	ID string `json:"id"`
	// Scope determines who the pause applies to.
	Scope PauseScope `json:"scope"`
	// DeviceID is required for PauseScopeDevice and ignored otherwise.
	DeviceID string `json:"device_id,omitempty"`
	// GroupID is required for PauseScopeGroup and ignored otherwise.
	GroupID string `json:"group_id,omitempty"`
	// CreatedBy is the admin username who created the pause.
	CreatedBy string `json:"created_by"`
	// CreatedAt is when the pause was created (UTC).
	CreatedAt time.Time `json:"created_at"`
	// Until is when the pause expires (UTC). Must be after CreatedAt.
	Until time.Time `json:"until"`
	// Reason is the admin-supplied justification, shown in audit.
	Reason string `json:"reason,omitempty"`
	// Active is false when the pause has been revoked. Expired pauses remain
	// Active=true but are ignored during evaluation.
	Active bool `json:"active"`
	// RevokedBy is the admin username who revoked the pause.
	RevokedBy string `json:"revoked_by,omitempty"`
	// RevokedAt is when the pause was revoked, if applicable.
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// PauseDocument is the persisted JSON shape.
type PauseDocument struct {
	Version   int       `json:"version"`
	Pauses    []Pause   `json:"pauses"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PauseSnapshot is the immutable evaluation view of persisted pauses.
type PauseSnapshot struct {
	Pauses    []Pause
	UpdatedAt time.Time
}

// MinPauseDuration is the shortest allowed pause duration. DNS caching and
// persistent connections make true "instant" pauses unreliable, so the window
// is clearly bounded.
const MinPauseDuration = 1 * time.Minute

// MaxPauseDuration bounds the longest single pause so temporary suppression
// cannot accidentally become permanent.
const MaxPauseDuration = 24 * time.Hour

// CreatePauseInput is the admin request body for creating a pause.
type CreatePauseInput struct {
	Scope    PauseScope `json:"scope"`
	DeviceID string     `json:"device_id,omitempty"`
	GroupID  string     `json:"group_id,omitempty"`
	Duration string     `json:"duration"` // Go duration string, e.g. "30m", "2h"
	Reason   string     `json:"reason,omitempty"`
}

// Validate checks that the pause input is well-formed before the service
// applies it. It does not check group or device existence; the service does
// that inside the storage transaction.
func (in CreatePauseInput) Validate() error {
	switch in.Scope {
	case PauseScopeDevice:
		if strings.TrimSpace(in.DeviceID) == "" {
			return fmt.Errorf("%w: device scope requires a device_id", ErrInvalidPause)
		}
	case PauseScopeGroup:
		if strings.TrimSpace(in.GroupID) == "" {
			return fmt.Errorf("%w: group scope requires a group_id", ErrInvalidPause)
		}
	case PauseScopeInstallation:
		// no target required
	default:
		return fmt.Errorf("%w: invalid scope %q", ErrInvalidPause, in.Scope)
	}
	if strings.TrimSpace(in.Duration) == "" {
		return fmt.Errorf("%w: duration is required", ErrInvalidPause)
	}
	d, err := time.ParseDuration(in.Duration)
	if err != nil {
		return fmt.Errorf("%w: invalid duration %q", ErrInvalidPause, in.Duration)
	}
	if d < MinPauseDuration {
		return fmt.Errorf("%w: duration %s is shorter than minimum %s", ErrInvalidPause, in.Duration, MinPauseDuration)
	}
	if d > MaxPauseDuration {
		return fmt.Errorf("%w: duration %s exceeds maximum %s", ErrInvalidPause, in.Duration, MaxPauseDuration)
	}
	return nil
}

// pauseStorage holds the in-memory pause snapshot, mirroring the exception
// storage pattern so pause writes do not require re-encoding the group
// document.
type pauseStorage struct {
	mu       sync.RWMutex
	snapshot PauseSnapshot
}

// loadPauseDocument reads and parses the pause document from storage.
func (s *Service) loadPauseDocument() (PauseDocument, error) {
	var doc PauseDocument
	if s.storage == nil {
		return doc, nil
	}
	raw, err := s.storage.GetE(PauseStorageKey)
	if err != nil {
		return doc, fmt.Errorf("read policy pauses: %w", err)
	}
	if raw == "" {
		return doc, nil
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return doc, fmt.Errorf("parse policy pauses: %w", err)
	}
	if doc.Version != PauseDocumentVersion {
		return doc, fmt.Errorf("unsupported policy pauses version %d", doc.Version)
	}
	return doc, nil
}

func snapshotFromPauseDocument(doc PauseDocument) PauseSnapshot {
	snap := PauseSnapshot{
		Pauses:    make([]Pause, len(doc.Pauses)),
		UpdatedAt: doc.UpdatedAt,
	}
	copy(snap.Pauses, doc.Pauses)
	return snap
}

func documentFromPauseSnapshot(snap PauseSnapshot) PauseDocument {
	doc := PauseDocument{
		Version:   PauseDocumentVersion,
		Pauses:    make([]Pause, len(snap.Pauses)),
		UpdatedAt: time.Now().UTC(),
	}
	copy(doc.Pauses, snap.Pauses)
	return doc
}

// loadPauses populates the in-memory pause snapshot. Called from Reload and
// NewService so pauses survive restarts.
func (s *Service) loadPauses() error {
	doc, err := s.loadPauseDocument()
	if err != nil {
		return err
	}
	// pa is the field on Service; guarded by its own mutex.
	// We use the same pattern as exceptionStorage.
	s.pa.mu.Lock()
	defer s.pa.mu.Unlock()
	s.pa.snapshot = snapshotFromPauseDocument(doc)
	return nil
}

// PauseSnapshot returns a point-in-time copy of active (non-expired,
// non-revoked) pauses for evaluation. Adapters call this per request.
func (s *Service) PauseSnapshot() PauseSnapshot {
	return pausesActiveAt(s.pa.all(), s.now())
}

// all returns a copy of every stored pause and the snapshot time.
func (p *pauseStorage) all() PauseSnapshot {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return PauseSnapshot{Pauses: append([]Pause(nil), p.snapshot.Pauses...), UpdatedAt: p.snapshot.UpdatedAt}
}

// pausesActiveAt keeps the pauses that are in force at an instant, so live
// enforcement and a preview at another time filter the same way.
func pausesActiveAt(snap PauseSnapshot, now time.Time) PauseSnapshot {
	active := make([]Pause, 0, len(snap.Pauses))
	for _, p := range snap.Pauses {
		if p.Active && now.Before(p.Until) {
			active = append(active, p)
		}
	}
	return PauseSnapshot{Pauses: active, UpdatedAt: snap.UpdatedAt}
}

// AllPauses returns all pauses including expired and revoked ones, for the
// admin audit view.
func (s *Service) AllPauses() []Pause {
	s.pa.mu.RLock()
	defer s.pa.mu.RUnlock()
	out := make([]Pause, len(s.pa.snapshot.Pauses))
	copy(out, s.pa.snapshot.Pauses)
	return out
}

// CreatePause validates input, checks group existence, and persists a new
// pause atomically. The in-memory snapshot is refreshed so the pause takes
// effect immediately.
func (s *Service) CreatePause(in CreatePauseInput, createdBy string) (Pause, error) {
	if err := in.Validate(); err != nil {
		return Pause{}, err
	}
	if strings.TrimSpace(createdBy) == "" {
		return Pause{}, fmt.Errorf("%w: created_by is required", ErrInvalidPause)
	}
	d, _ := time.ParseDuration(in.Duration)
	now := time.Now().UTC()
	pause := Pause{
		ID:        gatesentryUtils.RandomString(16),
		Scope:     in.Scope,
		DeviceID:  in.DeviceID,
		GroupID:   in.GroupID,
		CreatedBy: createdBy,
		CreatedAt: now,
		Until:     now.Add(d),
		Reason:    in.Reason,
		Active:    true,
	}
	err := s.updatePauses(func(snap *PauseSnapshot) error {
		// Validate group existence for group-scoped pauses inside the
		// transaction so a concurrent group delete is caught.
		if in.Scope == PauseScopeGroup {
			policySnap := s.currentSnapshot()
			if _, exists := policySnap.Groups[in.GroupID]; !exists {
				return fmt.Errorf("%w: group %s: %v", ErrInvalidPause, in.GroupID, ErrUnknownGroup)
			}
		}
		snap.Pauses = append(snap.Pauses, pause)
		return nil
	})
	if err != nil {
		return Pause{}, err
	}
	return pause, nil
}

// RevokePause marks a pause inactive without deleting it, preserving the
// audit trail.
func (s *Service) RevokePause(id, revokedBy string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidPause)
	}
	return s.updatePauses(func(snap *PauseSnapshot) error {
		for i := range snap.Pauses {
			if snap.Pauses[i].ID == id {
				if !snap.Pauses[i].Active {
					return fmt.Errorf("%w: already revoked", ErrPauseNotFound)
				}
				now := time.Now().UTC()
				snap.Pauses[i].Active = false
				snap.Pauses[i].RevokedBy = revokedBy
				snap.Pauses[i].RevokedAt = &now
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrPauseNotFound, id)
	})
}

// PruneExpiredPauses removes expired and revoked pauses older than
// MaxPauseDuration past their expiry, so the document does not grow
// unbounded.
func (s *Service) PruneExpiredPauses() (int, error) {
	pruned := 0
	err := s.updatePauses(func(snap *PauseSnapshot) error {
		cutoff := s.now().Add(-MaxPauseDuration)
		kept := make([]Pause, 0, len(snap.Pauses))
		for _, p := range snap.Pauses {
			if p.Active && p.Until.After(cutoff) {
				kept = append(kept, p)
				continue
			}
			if !p.Active && p.RevokedAt != nil && p.RevokedAt.After(cutoff) {
				kept = append(kept, p)
				continue
			}
			if !p.Active && p.Until.After(cutoff) {
				kept = append(kept, p)
				continue
			}
			pruned++
		}
		snap.Pauses = kept
		return nil
	})
	return pruned, err
}

// EvaluatePause checks whether an active pause suppresses the group block for
// the given identity. Precedence is device > group > installation (most
// specific first), mirroring exceptions. A pause suppresses the block action
// only; it does not override allow actions.
// evaluatePauseIn is the pure, snapshot-parametrized core of EvaluatePause.
// The live path and preview both call it so pause precedence cannot diverge
// between enforcement and preview.
func evaluatePauseIn(identity Identity, snap PauseSnapshot) (Pause, bool) {
	// First pass: device-scoped (most specific)
	for _, p := range snap.Pauses {
		if p.Scope == PauseScopeDevice && p.DeviceID == identity.DeviceID {
			return p, true
		}
	}
	// Second pass: group-scoped
	for _, p := range snap.Pauses {
		if p.Scope == PauseScopeGroup && p.GroupID == identity.GroupID {
			return p, true
		}
	}
	// Third pass: installation-scoped (least specific)
	for _, p := range snap.Pauses {
		if p.Scope == PauseScopeInstallation {
			return p, true
		}
	}
	return Pause{}, false
}

func (s *Service) EvaluatePause(identity Identity) (Pause, bool) {
	return evaluatePauseIn(identity, s.PauseSnapshot())
}

// updatePauses runs the transform inside one atomic storage transaction on
// the pause document, then refreshes the in-memory snapshot.
func (s *Service) updatePauses(transform func(*PauseSnapshot) error) error {
	if s.storage == nil {
		return errors.New("policy service has no storage")
	}
	if err := s.storage.UpdateValue(PauseStorageKey, func(current string) (string, error) {
		var doc PauseDocument
		if current != "" {
			if err := json.Unmarshal([]byte(current), &doc); err != nil {
				return "", fmt.Errorf("parse policy pauses: %w", err)
			}
			if doc.Version != PauseDocumentVersion {
				return "", fmt.Errorf("unsupported policy pauses version %d", doc.Version)
			}
		}
		snap := snapshotFromPauseDocument(doc)
		if err := transform(&snap); err != nil {
			return "", err
		}
		nextDoc := documentFromPauseSnapshot(snap)
		encoded, err := json.Marshal(nextDoc)
		if err != nil {
			return "", fmt.Errorf("encode policy pauses: %w", err)
		}
		return string(encoded), nil
	}); err != nil {
		return err
	}
	return s.loadPauses()
}
