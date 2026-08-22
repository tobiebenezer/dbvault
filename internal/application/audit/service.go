package audit

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

var (
	ErrChainTampered = errors.New("audit hash chain integrity verification failed: tamper detected")
	ErrEmptyChain    = errors.New("audit log is empty")
)

// Service provides cryptographic, append-only audit trail logging.
type Service struct {
	mu       sync.RWMutex
	events   []domain.AuditEvent
	lastHash string
}

// New creates a new Audit Service.
func New() *Service {
	return &Service{
		events:   make([]domain.AuditEvent, 0, 1024),
		lastHash: "0000000000000000000000000000000000000000000000000000000000000000",
	}
}

// Record appends a new audit event and computes its cryptographic link in the Merkle chain.
func (s *Service) Record(ctx context.Context, eventType, actorType, actorID, resourceType, resourceID, outcome string, metadata map[string]string) domain.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	eventID := domain.AuditEventID(fmt.Sprintf("audit-%d", now.UnixNano()))

	event := domain.AuditEvent{
		ID:           eventID,
		EventType:    eventType,
		ActorType:    actorType,
		ActorID:      actorID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Outcome:      outcome,
		Metadata:     metadata,
		CreatedAt:    now,
	}

	chained := domain.ChainAuditEvent(event, s.lastHash)
	s.events = append(s.events, chained)
	s.lastHash = chained.EventHash

	return chained
}

// List returns recorded audit events with optional limit and filtering.
func (s *Service) List(limit int, eventTypePrefix string) []domain.AuditEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}

	var results []domain.AuditEvent
	for i := len(s.events) - 1; i >= 0 && len(results) < limit; i-- {
		e := s.events[i]
		if eventTypePrefix == "" || e.EventType == eventTypePrefix {
			results = append(results, e)
		}
	}
	return results
}

// VerifyIntegrity cryptographically checks every event's SHA-256 hash against its previous link.
func (s *Service) VerifyIntegrity() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.events) == 0 {
		return true, nil
	}

	prev := "0000000000000000000000000000000000000000000000000000000000000000"
	for i, e := range s.events {
		if e.PreviousHash != prev {
			return false, fmt.Errorf("%w: mismatch at index %d (event %s)", ErrChainTampered, i, e.ID)
		}
		computed := domain.ChainAuditEvent(e, prev)
		if computed.EventHash != e.EventHash {
			return false, fmt.Errorf("%w: hash mismatch at index %d (event %s)", ErrChainTampered, i, e.ID)
		}
		prev = e.EventHash
	}

	return true, nil
}

// ExportCSV exports audit events to CSV format for SOC2 compliance.
func (s *Service) ExportCSV(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	writer := csv.NewWriter(w)
	defer writer.Flush()

	header := []string{"ID", "Timestamp", "EventType", "ActorType", "ActorID", "ResourceType", "ResourceID", "Outcome", "EventHash", "PreviousHash"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, e := range s.events {
		row := []string{
			string(e.ID),
			e.CreatedAt.Format(time.RFC3339),
			e.EventType,
			e.ActorType,
			e.ActorID,
			e.ResourceType,
			e.ResourceID,
			e.Outcome,
			e.EventHash,
			e.PreviousHash,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
