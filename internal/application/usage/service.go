package usage

import (
	"sync"

	"github.com/dbvault/dbvault/internal/domain"
)

type Service struct {
	mu      sync.Mutex
	records []domain.UsageRecord
}

func (s *Service) Record(record domain.UsageRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}
func (s *Service) Total(org domain.OrganisationID, metric string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	for _, r := range s.records {
		if r.OrganisationID == org && r.Metric == metric {
			total += r.Quantity
		}
	}
	return total
}
