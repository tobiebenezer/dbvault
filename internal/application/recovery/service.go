package recovery

import (
	"context"
	"strings"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Service struct {
	Catalogue ports.Catalogue
	Store     ports.ObjectStore
	Queue     ports.JobQueue
}

type Summary struct {
	RecoveredJobs     int
	RemoteCompletions int
	InterruptedJobs   []domain.JobID
}

func (s *Service) Startup(ctx context.Context) (Summary, error) {
	var out Summary
	if s.Queue != nil {
		ids, err := s.Queue.RecoverExpired(ctx, ports.SystemClock{}.Now())
		if err != nil {
			return out, err
		}
		out.InterruptedJobs = ids
		out.RecoveredJobs = len(ids)
	}
	if s.Store != nil {
		res, err := s.Store.List(ctx, ports.ListObjectsRequest{Prefix: "snapshots/", Limit: 1000})
		if err == nil {
			for _, obj := range res.Objects {
				if strings.HasSuffix(obj.Key, "/complete.json") {
					out.RemoteCompletions++
				}
			}
		}
	}
	return out, nil
}
