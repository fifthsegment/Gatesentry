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

// AccessRequestStorageKey is the settings key holding the access-request
// document. Access requests are kept separate from admin exceptions so a
// public request can never grant access; only an admin approval creates an
// exception.
const AccessRequestStorageKey = "access_requests"

// AccessRequestDocumentVersion is the persisted access-request document
// format version.
const AccessRequestDocumentVersion = 1

// AccessRequestStatus tracks the lifecycle of a public access request.
type AccessRequestStatus string

const (
	// AccessRequestPending means the request awaits admin review.
	AccessRequestPending AccessRequestStatus = "pending"
	// AccessRequestApproved means an admin approved it (and an exception
	// was created).
	AccessRequestApproved AccessRequestStatus = "approved"
	// AccessRequestRejected means an admin rejected it.
	AccessRequestRejected AccessRequestStatus = "rejected"
)

// AccessRequest is a public request from a blocked user to unblock a domain.
// It is deliberately separate from administrator authorization: creating a
// request never grants access; only an admin approval creates an exception.
type AccessRequest struct {
	ID        string              `json:"id"`
	Domain    string              `json:"domain"`
	ClientIP  string              `json:"client_ip"`
	Reason    string              `json:"reason,omitempty"`
	Status    AccessRequestStatus `json:"status"`
	CreatedAt time.Time           `json:"created_at"`
	// ReviewedBy is the admin username who approved or rejected the request.
	ReviewedBy string `json:"reviewed_by,omitempty"`
	// ReviewedAt is when the admin acted.
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	// ExceptionID is set when the request is approved and links to the
	// created exception for audit traceability.
	ExceptionID string `json:"exception_id,omitempty"`
}

// AccessRequestDocument is the persisted JSON shape.
type AccessRequestDocument struct {
	Version   int             `json:"version"`
	Requests  []AccessRequest `json:"requests"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// AccessRequestInput is the public request body. It carries no authentication
// and no scope; the admin decides scope at approval time.
type AccessRequestInput struct {
	Domain string `json:"domain"`
	Reason string `json:"reason,omitempty"`
}

// rateLimiter tracks request counts per client IP within a rolling window.
// It is in-memory only: rate limiting is a soft bound to prevent abuse, not
// a durable security control. A restart resets the counter.
type rateLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	window  time.Duration
	maxHits int
}

func newAccessRequestRateLimiter() *rateLimiter {
	return &rateLimiter{
		hits:    make(map[string][]time.Time),
		window:  time.Minute,
		maxHits: MaxAccessRequestsPerMinute,
	}
}

// allow reports whether the client IP may submit another request. It prunes
// expired entries so the map does not grow unbounded.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	// Prune old entries for this IP and globally (amortized cleanup).
	kept := make([]time.Time, 0, len(rl.hits[ip]))
	for _, t := range rl.hits[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	rl.hits[ip] = kept
	if len(rl.hits[ip]) >= rl.maxHits {
		return false
	}
	rl.hits[ip] = append(rl.hits[ip], now)
	return true
}

// loadAccessRequestDocument reads and parses the access-request document.
func (s *Service) loadAccessRequestDocument() (AccessRequestDocument, error) {
	var doc AccessRequestDocument
	if s.storage == nil {
		return doc, nil
	}
	raw, err := s.storage.GetE(AccessRequestStorageKey)
	if err != nil {
		return doc, fmt.Errorf("read access requests: %w", err)
	}
	if raw == "" {
		return doc, nil
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return doc, fmt.Errorf("parse access requests: %w", err)
	}
	if doc.Version != AccessRequestDocumentVersion {
		return doc, fmt.Errorf("unsupported access requests version %d", doc.Version)
	}
	return doc, nil
}

// CreateAccessRequest validates and persists a public access request. It
// never creates an exception or grants access; approval is a separate admin
// action. The rate limiter bounds requests per client IP per minute.
func (s *Service) CreateAccessRequest(in AccessRequestInput, clientIP string) (AccessRequest, error) {
	if strings.TrimSpace(in.Domain) == "" {
		return AccessRequest{}, fmt.Errorf("%w: domain is required", ErrInvalidException)
	}
	if s.accessRL == nil {
		return AccessRequest{}, errors.New("access request rate limiter not initialized")
	}
	if !s.accessRL.allow(clientIP) {
		return AccessRequest{}, errors.New("rate limit exceeded; please wait before submitting another request")
	}
	req := AccessRequest{
		ID:        gatesentryUtils.RandomString(16),
		Domain:    strings.ToLower(strings.TrimSpace(in.Domain)),
		ClientIP:  clientIP,
		Reason:    in.Reason,
		Status:    AccessRequestPending,
		CreatedAt: time.Now().UTC(),
	}
	err := s.updateAccessRequests(func(doc *AccessRequestDocument) error {
		doc.Requests = append(doc.Requests, req)
		// Keep only the most recent 200 requests to bound growth.
		if len(doc.Requests) > 200 {
			doc.Requests = doc.Requests[len(doc.Requests)-200:]
		}
		return nil
	})
	if err != nil {
		return AccessRequest{}, err
	}
	return req, nil
}

// ListAccessRequests returns all access requests for the admin review view.
func (s *Service) ListAccessRequests() []AccessRequest {
	doc, err := s.loadAccessRequestDocument()
	if err != nil {
		return nil
	}
	out := make([]AccessRequest, len(doc.Requests))
	copy(out, doc.Requests)
	return out
}

// ApproveAccessRequest marks a pending request approved, creates a scoped
// exception, and links it back to the request for audit. The admin supplies
// the exception scope (device/group/installation) and duration at approval
// time; the requester has no say in scope.
func (s *Service) ApproveAccessRequest(requestID string, excInput CreateExceptionInput, adminUser string) (AccessRequest, Exception, error) {
	if strings.TrimSpace(requestID) == "" {
		return AccessRequest{}, Exception{}, fmt.Errorf("%w: request id is required", ErrInvalidException)
	}
	// Validate the exception input first so we fail fast.
	if err := excInput.Validate(); err != nil {
		return AccessRequest{}, Exception{}, err
	}
	if strings.TrimSpace(adminUser) == "" {
		return AccessRequest{}, Exception{}, fmt.Errorf("%w: admin user is required", ErrInvalidException)
	}

	// Create the exception.
	exc, err := s.CreateException(excInput, adminUser)
	if err != nil {
		return AccessRequest{}, Exception{}, err
	}

	// Mark the request approved and link it.
	var updatedReq AccessRequest
	err = s.updateAccessRequests(func(doc *AccessRequestDocument) error {
		for i := range doc.Requests {
			if doc.Requests[i].ID == requestID {
				if doc.Requests[i].Status != AccessRequestPending {
					return fmt.Errorf("%w: request already reviewed", ErrInvalidException)
				}
				now := time.Now().UTC()
				doc.Requests[i].Status = AccessRequestApproved
				doc.Requests[i].ReviewedBy = adminUser
				doc.Requests[i].ReviewedAt = &now
				doc.Requests[i].ExceptionID = exc.ID
				updatedReq = doc.Requests[i]
				return nil
			}
		}
		return fmt.Errorf("%w: access request %s not found", ErrExceptionNotFound, requestID)
	})
	if err != nil {
		// If the request update failed, the exception was already created.
		// This is acceptable: the exception exists and can be revoked
		// independently. Log the inconsistency by returning both results.
		return AccessRequest{}, exc, err
	}
	return updatedReq, exc, nil
}

// RejectAccessRequest marks a pending request rejected without creating an
// exception.
func (s *Service) RejectAccessRequest(requestID, adminUser string) (AccessRequest, error) {
	if strings.TrimSpace(requestID) == "" {
		return AccessRequest{}, fmt.Errorf("%w: request id is required", ErrInvalidException)
	}
	if strings.TrimSpace(adminUser) == "" {
		return AccessRequest{}, fmt.Errorf("%w: admin user is required", ErrInvalidException)
	}
	var updatedReq AccessRequest
	err := s.updateAccessRequests(func(doc *AccessRequestDocument) error {
		for i := range doc.Requests {
			if doc.Requests[i].ID == requestID {
				if doc.Requests[i].Status != AccessRequestPending {
					return fmt.Errorf("%w: request already reviewed", ErrInvalidException)
				}
				now := time.Now().UTC()
				doc.Requests[i].Status = AccessRequestRejected
				doc.Requests[i].ReviewedBy = adminUser
				doc.Requests[i].ReviewedAt = &now
				updatedReq = doc.Requests[i]
				return nil
			}
		}
		return fmt.Errorf("%w: access request %s not found", ErrExceptionNotFound, requestID)
	})
	return updatedReq, err
}

// updateAccessRequests runs the transform inside one atomic storage
// transaction on the access-request document.
func (s *Service) updateAccessRequests(transform func(*AccessRequestDocument) error) error {
	if s.storage == nil {
		return errors.New("policy service has no storage")
	}
	return s.storage.UpdateValue(AccessRequestStorageKey, func(current string) (string, error) {
		var doc AccessRequestDocument
		if current != "" {
			if err := json.Unmarshal([]byte(current), &doc); err != nil {
				return "", fmt.Errorf("parse access requests: %w", err)
			}
			if doc.Version != AccessRequestDocumentVersion {
				return "", fmt.Errorf("unsupported access requests version %d", doc.Version)
			}
		}
		if err := transform(&doc); err != nil {
			return "", err
		}
		nextDoc := AccessRequestDocument{
			Version:   AccessRequestDocumentVersion,
			Requests:  doc.Requests,
			UpdatedAt: time.Now().UTC(),
		}
		encoded, err := json.Marshal(nextDoc)
		if err != nil {
			return "", fmt.Errorf("encode access requests: %w", err)
		}
		return string(encoded), nil
	})
}
