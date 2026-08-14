package approval

import (
	"errors"
	"sync"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

type Service struct {
	mu        sync.Mutex
	requests  map[string]domain.ApprovalRequest
	decisions map[string][]domain.ApprovalDecision
}

func New() *Service {
	return &Service{requests: map[string]domain.ApprovalRequest{}, decisions: map[string][]domain.ApprovalDecision{}}
}
func (s *Service) Create(request domain.ApprovalRequest) error {
	if request.ID == "" || request.RequestedBy == "" || request.RequiredApprovals < 1 {
		return errors.New("invalid approval request")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.requests[request.ID]; ok {
		return errors.New("approval request exists")
	}
	request.Status = domain.ApprovalPending
	s.requests[request.ID] = request
	return nil
}
func (s *Service) Decide(requestID string, decision domain.ApprovalDecision, now time.Time) (domain.ApprovalRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request, ok := s.requests[requestID]
	if !ok {
		return request, errors.New("approval request not found")
	}
	if request.Status != domain.ApprovalPending {
		return request, errors.New("approval request is terminal")
	}
	if request.ExpiresAt.Before(now) {
		request.Status = domain.ApprovalExpired
		s.requests[requestID] = request
		return request, errors.New("approval request expired")
	}
	if decision.ActorID == request.RequestedBy {
		return request, errors.New("requester cannot approve their own destructive action")
	}
	for _, d := range s.decisions[requestID] {
		if d.ActorID == decision.ActorID {
			return request, errors.New("actor has already decided")
		}
	}
	decision.RequestID, decision.CreatedAt = requestID, now
	s.decisions[requestID] = append(s.decisions[requestID], decision)
	if !decision.Approved {
		request.Status = domain.ApprovalRejected
	} else {
		approved := 0
		for _, d := range s.decisions[requestID] {
			if d.Approved {
				approved++
			}
		}
		if approved >= request.RequiredApprovals {
			request.Status = domain.ApprovalApproved
		}
	}
	s.requests[requestID] = request
	return request, nil
}
