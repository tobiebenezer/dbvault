package provisioning

import (
	"regexp"
	"strings"
	"time"
)

type JobType string

const (
	JobServerInstall   JobType = "server_install"
	JobServerUpgrade   JobType = "server_upgrade"
	JobServerRepair    JobType = "server_repair"
	JobServerUninstall JobType = "server_uninstall"
	JobAgentEnrol      JobType = "agent_enrol"
)

type StageStatus string

const (
	StagePending   StageStatus = "pending"
	StageRunning   StageStatus = "running"
	StageSucceeded StageStatus = "succeeded"
	StageFailed    StageStatus = "failed"
)

type Stage struct {
	Name        string      `json:"name"`
	Status      StageStatus `json:"status"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	Output      string      `json:"output,omitempty"`
}

type Job struct {
	ID                string  `json:"id"`
	Type              JobType `json:"type"`
	Target            string  `json:"target"`
	Stages            []Stage `json:"stages"`
	RollbackAvailable bool    `json:"rollback_available"`
}

func NewInstallJob(id, target string) Job {
	stages := []string{"created", "connecting", "preflight", "creating_user", "installing_binary", "writing_configuration", "installing_service", "configuring_firewall", "starting_service", "enrolling_agent", "verifying", "completed"}
	job := Job{ID: id, Type: JobServerInstall, Target: target, RollbackAvailable: true}
	for _, s := range stages {
		job.Stages = append(job.Stages, Stage{Name: s, Status: StagePending})
	}
	return job
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|secret|token|access_key|secret_access_key)=([^\s]+)`),
	regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/-]+`),
}

func RedactOutput(s string) string {
	out := s
	for _, pattern := range secretPatterns {
		out = pattern.ReplaceAllString(out, `${1}=REDACTED`)
	}
	return strings.ReplaceAll(out, "--password ", "--password REDACTED ")
}

func AgentOneLineInstall(controller, token string) string {
	return "curl -fsSL " + strings.TrimRight(controller, "/") + "/install/agent.sh | sudo DBVAULT_AGENT_TOKEN=" + token + " sh"
}
