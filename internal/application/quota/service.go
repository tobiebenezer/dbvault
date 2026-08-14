package quota

import (
	"fmt"

	"github.com/dbvault/dbvault/internal/domain"
)

type CurrentUsage struct {
	Sources, Agents                       int
	StorageBytes                          int64
	ConcurrentBackups, ConcurrentRestores int
}

func Validate(quota domain.TenantQuota, usage CurrentUsage, operation string) error {
	switch operation {
	case "source.create":
		if quota.MaximumSources > 0 && usage.Sources >= quota.MaximumSources {
			return fmt.Errorf("source quota exceeded")
		}
	case "agent.enrol":
		if quota.MaximumAgents > 0 && usage.Agents >= quota.MaximumAgents {
			return fmt.Errorf("agent quota exceeded")
		}
	case "backup.create":
		if quota.MaximumConcurrentBackup > 0 && usage.ConcurrentBackups >= quota.MaximumConcurrentBackup {
			return fmt.Errorf("concurrent backup quota exceeded")
		}
	case "restore.create":
		if quota.MaximumConcurrentRestore > 0 && usage.ConcurrentRestores >= quota.MaximumConcurrentRestore {
			return fmt.Errorf("concurrent restore quota exceeded")
		}
	case "storage.reserve":
		if quota.MaximumStorageBytes > 0 && usage.StorageBytes >= quota.MaximumStorageBytes {
			return fmt.Errorf("storage quota exceeded; retained backups are preserved")
		}
	}
	return nil
}
