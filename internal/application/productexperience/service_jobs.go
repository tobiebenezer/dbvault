package productexperience

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
)

var jobStages = map[string][]string{
	"backup":        {"snapshot", "inspect", "plan", "upload", "publish", "verify", "replicate", "complete"},
	"restore":       {"plan", "reserve", "download", "verify", "restore", "replay_logs", "validate", "complete"},
	"restore_drill": {"create_workspace", "restore", "open_database", "run_checks", "record_evidence", "cleanup", "complete"},
	"sandbox":       {"reserve", "create_runtime", "restore", "verify", "publish_connection", "schedule_expiry", "complete"},
	"verification":  {"load_manifest", "verify_signature", "verify_objects", "verify_checksums", "record_result", "complete"},
	"replication":   {"plan_missing_objects", "copy", "verify_replica", "update_coverage", "complete"},
	"doctor":        {"database", "tools", "storage", "keys", "disk", "clock", "tls", "agent", "restore_requirements", "complete"},
	"bundle":        {"collect", "redact", "package", "sign", "verify", "complete"},
	"upgrade":       {"download", "verify", "backup_catalogue", "pause_jobs", "install", "migrate", "restart", "health_check", "commit"},
}

func (s *Service) seedJobs() {
	now := s.now()
	s.jobs = map[string]domain.JobView{}
	s.jobLogs = map[string][]domain.JobLogLine{}
	s.jobEvents = nil
	s.eventSeq = 0
	started := now.Add(-6 * time.Minute)
	running := domain.JobView{
		ID: "job-backup-demo", OrganisationID: "default", ProjectID: "default", ResourceType: "source", ResourceID: "production-postgres", ResourceName: "Production PostgreSQL", JobType: "backup", Status: "running", Stage: "upload", StageIndex: 4, StageCount: 8, Percentage: 62, BytesProcessed: 1_820_000_000, BytesTotal: 2_900_000_000, ThroughputBPS: 31_000_000, Message: "Uploading chunks to primary destination.", CanCancel: true, Attempt: 1, StartedAt: &started, CreatedAt: started, UpdatedAt: now,
	}
	completedAt := now.Add(-2 * time.Hour)
	completedStarted := completedAt.Add(-4 * time.Minute)
	completed := domain.JobView{
		ID: "job-restore-drill-demo", OrganisationID: "default", ProjectID: "default", ResourceType: "source", ResourceID: "production-postgres", ResourceName: "Production PostgreSQL", JobType: "restore_drill", Status: "completed", Stage: "complete", StageIndex: 7, StageCount: 7, Percentage: 100, BytesProcessed: 2_900_000_000, BytesTotal: 2_900_000_000, ThroughputBPS: 28_000_000, Message: "Sandbox restore opened successfully.", CanCancel: false, Attempt: 1, StartedAt: &completedStarted, CompletedAt: &completedAt, CreatedAt: completedStarted, UpdatedAt: completedAt,
	}
	failedAt := now.Add(-47 * time.Minute)
	failedStarted := failedAt.Add(-3 * time.Minute)
	failed := domain.JobView{
		ID: "job-replica-demo", OrganisationID: "default", ProjectID: "default", ResourceType: "destination", ResourceID: "contabo", ResourceName: "Contabo replica", JobType: "replication", Status: "failed", Stage: "copy", StageIndex: 2, StageCount: 5, Percentage: 38, BytesProcessed: 620_000_000, BytesTotal: 1_630_000_000, ThroughputBPS: 0, Message: "Replica upload timed out. Primary backup remains safe.", CanCancel: false, Attempt: 1, StartedAt: &failedStarted, CompletedAt: &failedAt, CreatedAt: failedStarted, UpdatedAt: failedAt, LastErrorCode: "destination_timeout", LastError: "Contabo object storage timed out during multipart upload.",
	}
	for _, job := range []domain.JobView{running, completed, failed} {
		s.jobs[job.ID] = job
		s.jobLogs[job.ID] = []domain.JobLogLine{{ID: job.ID + "-log-1", JobID: job.ID, Level: "info", Stage: job.Stage, Message: "Job accepted by DBVault worker.", CreatedAt: job.CreatedAt}, {ID: job.ID + "-log-2", JobID: job.ID, Level: levelForStatus(job.Status), Stage: job.Stage, Message: job.Message, CreatedAt: job.UpdatedAt}}
		s.appendJobEventLocked(eventTypeForStatus(job.Status), job, job.Message)
	}
}

func (s *Service) Jobs(filters map[string]string) []domain.JobView {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.JobView, 0, len(s.jobs))
	for _, job := range s.jobs {
		if filter := strings.TrimSpace(filters["status"]); filter != "" && job.Status != filter {
			continue
		}
		if filter := strings.TrimSpace(filters["job_type"]); filter != "" && job.JobType != filter {
			continue
		}
		if filter := strings.TrimSpace(filters["source_id"]); filter != "" && job.ResourceID != filter {
			continue
		}
		out = append(out, job)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func (s *Service) Job(id string) (domain.JobView, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *Service) JobLogs(id string) []domain.JobLogLine {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.JobLogLine(nil), s.jobLogs[id]...)
}

func (s *Service) JobEventsSince(last string, limit int) []domain.JobEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	start := 0
	if last != "" {
		for i, event := range s.jobEvents {
			if event.EventID == last {
				start = i + 1
				break
			}
		}
	}
	if start >= len(s.jobEvents) {
		return []domain.JobEvent{}
	}
	end := start + limit
	if end > len(s.jobEvents) {
		end = len(s.jobEvents)
	}
	return append([]domain.JobEvent(nil), s.jobEvents[start:end]...)
}

func (s *Service) CreateJob(jobType, resourceID, resourceName string) domain.JobView {
	if strings.TrimSpace(jobType) == "" {
		jobType = "backup"
	}
	if strings.TrimSpace(resourceID) == "" {
		resourceID = "production-postgres"
	}
	if strings.TrimSpace(resourceName) == "" {
		resourceName = "Production PostgreSQL"
	}
	now := s.now()
	stages := stagesFor(jobType)
	job := domain.JobView{ID: "job-" + jobType + "-" + stableID(now.String()+resourceID), OrganisationID: "default", ProjectID: "default", ResourceType: resourceTypeForJob(jobType), ResourceID: resourceID, ResourceName: resourceName, JobType: jobType, Status: "queued", Stage: stages[0], StageIndex: 1, StageCount: len(stages), Percentage: 0, BytesTotal: defaultBytesForJob(jobType), Message: "Waiting for an available DBVault worker.", CanCancel: true, Attempt: 1, CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.jobLogs[job.ID] = []domain.JobLogLine{{ID: job.ID + "-log-1", JobID: job.ID, Level: "info", Stage: job.Stage, Message: job.Message, CreatedAt: now}}
	s.appendJobEventLocked("job.created", job, job.Message)
	s.mu.Unlock()
	return job
}

func (s *Service) CancelJob(id string) (domain.JobView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return domain.JobView{}, errors.New("job not found")
	}
	if !job.CanCancel || isTerminalJob(job.Status) {
		return domain.JobView{}, errors.New("job cannot be cancelled safely")
	}
	now := s.now()
	job.Status = "cancelled"
	job.CanCancel = false
	job.Message = "Job cancelled at a safe cancellation point."
	job.UpdatedAt = now
	job.CompletedAt = &now
	s.jobs[id] = job
	s.appendLogLocked(id, "warning", job.Stage, job.Message)
	s.appendJobEventLocked("job.cancelled", job, job.Message)
	return job, nil
}

func (s *Service) RetryJob(id string) (domain.JobRetryResponse, error) {
	s.mu.Lock()
	original, ok := s.jobs[id]
	s.mu.Unlock()
	if !ok {
		return domain.JobRetryResponse{}, errors.New("job not found")
	}
	newJob := s.CreateJob(original.JobType, original.ResourceID, original.ResourceName)
	s.mu.Lock()
	newJob.OriginalJobID = original.ID
	newJob.Attempt = original.Attempt + 1
	s.jobs[newJob.ID] = newJob
	s.appendJobEventLocked("job.retrying", newJob, "Retry created from "+original.ID)
	s.mu.Unlock()
	return domain.JobRetryResponse{OriginalJobID: original.ID, NewJobID: newJob.ID, Job: newJob}, nil
}

// GenerateDemoProgress advances one running or queued job. The real production
// worker pool will publish the same event contract from durable jobs.
func (s *Service) GenerateDemoProgress() []domain.JobEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var selected domain.JobView
	found := false
	for _, job := range s.jobs {
		if job.Status == "running" || job.Status == "queued" || job.Status == "cancelling" {
			if !found || job.UpdatedAt.Before(selected.UpdatedAt) {
				selected = job
				found = true
			}
		}
	}
	if !found {
		return nil
	}
	before := len(s.jobEvents)
	now := s.now()
	if selected.Status == "queued" {
		selected.Status = "running"
		selected.Message = "Worker lease acquired."
		selected.StartedAt = &now
		selected.UpdatedAt = now
		s.jobs[selected.ID] = selected
		s.appendLogLocked(selected.ID, "info", selected.Stage, selected.Message)
		s.appendJobEventLocked("job.started", selected, selected.Message)
	} else {
		selected.Percentage += 9
		if selected.Percentage > 100 {
			selected.Percentage = 100
		}
		selected.BytesProcessed = int64((selected.Percentage / 100) * float64(selected.BytesTotal))
		stages := stagesFor(selected.JobType)
		idx := int((selected.Percentage / 100) * float64(len(stages)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(stages) {
			idx = len(stages) - 1
		}
		selected.Stage = stages[idx]
		selected.StageIndex = idx + 1
		selected.ThroughputBPS = 28_000_000
		selected.UpdatedAt = now
		if selected.Percentage >= 100 {
			selected.Status = "completed"
			selected.CanCancel = false
			selected.CompletedAt = &now
			selected.Message = "Job completed and evidence was recorded."
			s.jobs[selected.ID] = selected
			s.appendLogLocked(selected.ID, "info", selected.Stage, selected.Message)
			s.appendJobEventLocked("job.completed", selected, selected.Message)
		} else {
			selected.Status = "running"
			selected.Message = fmt.Sprintf("%s, %.0f%%", humanStage(selected.Stage), selected.Percentage)
			s.jobs[selected.ID] = selected
			s.appendJobEventLocked("job.progress", selected, selected.Message)
		}
	}
	return append([]domain.JobEvent(nil), s.jobEvents[before:]...)
}

func (s *Service) appendJobEventLocked(eventType string, job domain.JobView, message string) domain.JobEvent {
	s.eventSeq++
	event := domain.JobEvent{EventID: fmt.Sprintf("evt-%06d", s.eventSeq), EventType: eventType, JobID: job.ID, OrganisationID: job.OrganisationID, ProjectID: job.ProjectID, ResourceType: job.ResourceType, ResourceID: job.ResourceID, JobType: job.JobType, Status: job.Status, Stage: job.Stage, StageIndex: job.StageIndex, StageCount: job.StageCount, Percentage: clampPercent(job.Percentage), BytesProcessed: job.BytesProcessed, BytesTotal: job.BytesTotal, ThroughputBPS: job.ThroughputBPS, Message: message, CanCancel: job.CanCancel, ErrorCode: job.LastErrorCode, CreatedAt: s.now()}
	s.jobEvents = append(s.jobEvents, event)
	if len(s.jobEvents) > 1000 {
		s.jobEvents = append([]domain.JobEvent(nil), s.jobEvents[len(s.jobEvents)-1000:]...)
	}
	return event
}

func (s *Service) appendLogLocked(jobID, level, stage, message string) {
	line := domain.JobLogLine{ID: jobID + "-log-" + stableID(fmt.Sprintf("%d-%s", len(s.jobLogs[jobID])+1, message)), JobID: jobID, Level: level, Stage: stage, Message: message, CreatedAt: s.now()}
	s.jobLogs[jobID] = append(s.jobLogs[jobID], line)
}

func stagesFor(jobType string) []string {
	if stages, ok := jobStages[jobType]; ok {
		return stages
	}
	return []string{"queued", "running", "complete"}
}

func eventTypeForStatus(status string) string {
	switch status {
	case "completed":
		return "job.completed"
	case "failed":
		return "job.failed"
	case "cancelled":
		return "job.cancelled"
	case "queued":
		return "job.queued"
	default:
		return "job.progress"
	}
}

func levelForStatus(status string) string {
	if status == "failed" {
		return "error"
	}
	if status == "cancelled" {
		return "warning"
	}
	return "info"
}

func isTerminalJob(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "dead_letter":
		return true
	default:
		return false
	}
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func resourceTypeForJob(jobType string) string {
	switch jobType {
	case "replication":
		return "destination"
	case "bundle", "upgrade":
		return "system"
	default:
		return "source"
	}
}

func defaultBytesForJob(jobType string) int64 {
	switch jobType {
	case "doctor":
		return 0
	case "bundle":
		return 120_000_000
	case "sandbox", "restore", "restore_drill":
		return 2_900_000_000
	default:
		return 2_900_000_000
	}
}

func humanStage(stage string) string { return strings.ReplaceAll(stage, "_", " ") }
