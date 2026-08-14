package budget

import "github.com/dbvault/dbvault/internal/domain"

type Level string

const (
	Normal   Level = "normal"
	Warning  Level = "warning"
	Critical Level = "critical"
	Blocked  Level = "blocked"
)

type Status struct {
	RepositoryID       domain.RepositoryID
	MaximumBytes       int64
	PhysicalBytes      int64
	EstimatedNextBytes int64
	ReserveBytes       int64
	AvailableBytes     int64
	Level              Level
}

func Evaluate(repo domain.Repository, physicalBytes, estimatedNextBytes int64) Status {
	max := repo.Budget.MaximumRepositoryBytes
	reserve := max * int64(repo.Budget.ReservePercent) / 100
	if reserve < repo.Budget.MinimumReserveBytes {
		reserve = repo.Budget.MinimumReserveBytes
	}
	avail := max - physicalBytes - reserve
	level := Normal
	if max > 0 {
		ratio := physicalBytes * 100 / max
		if int(ratio) >= repo.Budget.CriticalPercent {
			level = Critical
		} else if int(ratio) >= repo.Budget.WarningPercent {
			level = Warning
		}
		if physicalBytes+estimatedNextBytes+reserve > max {
			level = Blocked
		}
	}
	return Status{RepositoryID: repo.ID, MaximumBytes: max, PhysicalBytes: physicalBytes, EstimatedNextBytes: estimatedNextBytes, ReserveBytes: reserve, AvailableBytes: avail, Level: level}
}
