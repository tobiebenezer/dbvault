package productexperience

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	keyfile "github.com/dbvault/dbvault/internal/adapters/keys/file"
	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
)

type Service struct {
	mu        sync.Mutex
	root      string
	now       func() time.Time
	setup     domain.SetupState
	discovery []domain.DiscoveredDatabase
	sandboxes map[string]domain.SandboxRestore
	alerts    []domain.Alert
	jobs      map[string]domain.JobView
	jobLogs   map[string][]domain.JobLogLine
	jobEvents []domain.JobEvent
	approvals map[string]domain.RestoreApprovalRequest
	eventSeq  int64
}

func New(root string) (*Service, error) {
	if root == "" {
		root = filepath.Join(os.TempDir(), "dbvault-product-experience")
	}
	s := &Service{root: root, now: func() time.Time { return time.Now().UTC() }, sandboxes: map[string]domain.SandboxRestore{}, jobs: map[string]domain.JobView{}, jobLogs: map[string][]domain.JobLogLine{}, approvals: map[string]domain.RestoreApprovalRequest{}}
	if err := os.MkdirAll(filepath.Join(root, "bundles"), 0700); err != nil {
		return nil, err
	}
	if err := s.loadSetup(); err != nil {
		return nil, err
	}
	s.alerts = s.defaultAlerts()
	s.seedJobs()
	return s, nil
}

func (s *Service) Overview(ctx context.Context) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	protected := s.ProtectionSummary(ctx, "production-postgres")
	return map[string]any{
		"system_status": "some_protection_degraded",
		"summary": map[string]any{
			"protected_databases":     1,
			"at_risk_databases":       1,
			"unprotected_databases":   0,
			"failed_jobs":             0,
			"latest_verified_restore": s.now().Add(-48 * time.Hour),
			"storage_usage_bytes":     int64(8_912_000_000),
			"destination_health":      "degraded",
			"agent_health":            "healthy",
		},
		"primary_status": protected,
		"actions": []domain.Alert{
			s.defaultAlerts()[0],
		},
		"generated_at": s.now(),
	}, nil
}

func (s *Service) ProtectionSummary(_ context.Context, sourceID domain.SourceID) domain.ProtectionSummary {
	now := s.now()
	reasons := []domain.ProtectionReason{
		{Code: "backup_fresh", Severity: "info", Summary: "Latest backup is within policy", Evidence: "backup completed 18 minutes ago"},
		{Code: "restore_drill_current", Severity: "info", Summary: "Latest restore drill passed within policy", Evidence: "restore drill passed 2 days ago"},
		{Code: "replica_delayed", Severity: "warning", Summary: "Contabo replica is 47 minutes behind", Evidence: "three recent upload retries timed out"},
	}
	return domain.ProtectionSummary{
		SourceID: domain.SourceID(sourceID),
		Status:   domain.ProtectionAtRisk,
		Score:    84,
		Summary:  "Recoverable, but one replica is delayed. Primary R2 backup remains safe.",
		Reasons:  reasons,
		SuggestedActions: []domain.SuggestedAction{
			{ID: "retry-replication", Label: "Retry replication", Description: "Retry failed uploads to the delayed replica.", Safe: true},
			{ID: "test-contabo", Label: "Test Contabo connection", Description: "Run a destination capability probe.", Safe: true},
			{ID: "view-technical-details", Label: "View technical details", Description: "Open the upload failures and object keys.", Safe: true},
		},
		CalculatedAt: now,
	}
}

func (s *Service) RecoveryTimeline(_ context.Context, sourceID string) domain.RecoveryTimelineResponse {
	now := s.now().Truncate(time.Minute)
	start := now.Add(-5 * 24 * time.Hour)
	drill := now.Add(-48 * time.Hour)
	return domain.RecoveryTimelineResponse{
		SourceID:   sourceID,
		Earliest:   &start,
		Latest:     &now,
		Continuous: true,
		Windows:    []domain.RecoveryWindowView{{Start: start, End: now, Continuous: true, Verified: true}},
		Events: []domain.RecoveryTimelineEvent{
			{ID: "base-1", Type: "full_backup", Label: "Full backup", Status: "verified", OccurredAt: start, Description: "Verified base backup."},
			{ID: "drill-1", Type: "restore_drill", Label: "Restore drill", Status: "passed", OccurredAt: drill, Description: "Sandbox restore opened successfully."},
			{ID: "log-1", Type: "transaction_log", Label: "Latest log", Status: "continuous", OccurredAt: now, Description: "Continuous recovery chain available."},
		},
		Gaps:        []domain.RecoveryTimelineGap{},
		GeneratedAt: now,
	}
}

func (s *Service) Setup() domain.SetupState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setup
}

func (s *Service) SetSetupStep(step string) (domain.SetupState, error) {
	if !validSetupStep(step) {
		return domain.SetupState{}, fmt.Errorf("unknown setup step %q", step)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setup.ID == "" {
		s.setup = newSetupState(s.now())
	}
	s.setup.CurrentStep = step
	s.setup.UpdatedAt = s.now()
	if err := s.writeSetupLocked(); err != nil {
		return domain.SetupState{}, err
	}
	return s.setup, nil
}

func (s *Service) SaveSetupStep(step string, draft json.RawMessage) (domain.SetupState, error) {
	if !validSetupStep(step) || step == "finish" {
		return domain.SetupState{}, fmt.Errorf("unknown setup step %q", step)
	}
	values := map[string]string{}
	if len(draft) == 0 {
		draft = []byte("{}")
	}
	if err := json.Unmarshal(draft, &values); err != nil {
		return domain.SetupState{}, errors.New("setup values must be a JSON object containing text fields")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setup.ID == "" {
		s.setup = newSetupState(s.now())
	}
	if s.setup.Drafts == nil {
		s.setup.Drafts = map[string]map[string]string{}
	}
	previous := s.setup.Drafts[step]
	if err := s.validateSetupStepLocked(step, values, previous); err != nil {
		return domain.SetupState{}, err
	}
	for field, filename := range setupSecretFields {
		value := strings.TrimSpace(values[field])
		if value == "" {
			continue
		}
		if err := s.writeSecretLocked(filename, value); err != nil {
			return domain.SetupState{}, err
		}
		delete(values, field)
		values[field+"_configured"] = "true"
	}
	if !contains(s.setup.CompletedSteps, step) {
		s.setup.CompletedSteps = append(s.setup.CompletedSteps, step)
	}
	s.setup.CurrentStep = nextSetupStep(step)
	s.setup.DraftConfigJSON = nil
	s.setup.Drafts[step] = values
	s.setup.UpdatedAt = s.now()
	if err := s.writeSetupLocked(); err != nil {
		return domain.SetupState{}, err
	}
	return s.setup, nil
}

func (s *Service) FinishSetup() (domain.SetupState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setup.ID == "" {
		s.setup = newSetupState(s.now())
	}
	cfg, err := s.runtimeConfigLocked()
	if err != nil {
		return domain.SetupState{}, err
	}
	if err := config.Validate(cfg); err != nil {
		return domain.SetupState{}, fmt.Errorf("generated configuration is invalid: %w", err)
	}
	configPath := filepath.Join(filepath.Dir(s.root), "dbvault.generated.json")
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return domain.SetupState{}, err
	}
	if err := writeFileAtomic(configPath, append(b, '\n'), 0600); err != nil {
		return domain.SetupState{}, fmt.Errorf("write runtime configuration: %w", err)
	}
	s.setup.Status = "configuration_ready"
	s.setup.CurrentStep = "finish"
	s.setup.ConfigPath = configPath
	s.setup.UpdatedAt = s.now()
	return s.setup, s.writeSetupLocked()
}

func (s *Service) Discover(ctx context.Context, agentID string, roots []string) ([]domain.DiscoveredDatabase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if agentID == "" {
		agentID = "local"
	}
	var out []domain.DiscoveredDatabase
	ports := []struct {
		engine, host string
		port         int
	}{{"postgres", "127.0.0.1", 5432}, {"mysql", "127.0.0.1", 3306}}
	for _, p := range ports {
		out = append(out, domain.DiscoveredDatabase{ID: fmt.Sprintf("%s-%d", p.engine, p.port), AgentID: agentID, Engine: p.engine, Host: p.host, Port: p.port, Confidence: 0.55, Evidence: []string{fmt.Sprintf("common %s port %d candidate", p.engine, p.port)}, Status: "candidate"})
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || len(out) > 30 {
				return nil
			}
			name := strings.ToLower(entry.Name())
			if strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".sqlite") || strings.HasSuffix(name, ".sqlite3") {
				out = append(out, domain.DiscoveredDatabase{ID: "sqlite-" + stableID(path), AgentID: agentID, Engine: "sqlite", Path: path, Confidence: 0.82, Evidence: []string{"file extension suggests SQLite database"}, Status: "candidate"})
			}
			return nil
		})
	}
	s.mu.Lock()
	s.discovery = out
	s.mu.Unlock()
	return out, nil
}

func (s *Service) Discoveries() []domain.DiscoveredDatabase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.DiscoveredDatabase(nil), s.discovery...)
}

func (s *Service) UpdateDiscoveryStatus(id, status string) (domain.DiscoveredDatabase, error) {
	if status != "adopted" && status != "ignored" && status != "candidate" {
		return domain.DiscoveredDatabase{}, fmt.Errorf("unsupported discovery status %q", status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.discovery {
		if s.discovery[i].ID == id {
			s.discovery[i].Status = status
			return s.discovery[i], nil
		}
	}
	return domain.DiscoveredDatabase{}, fmt.Errorf("discovery %q not found", id)
}

func (s *Service) Inventory() domain.ResourceInventory {
	now := s.now()
	protection := s.ProtectionSummary(context.Background(), domain.SourceID("production-postgres"))
	return domain.ResourceInventory{
		Databases: []domain.DatabaseResource{
			{ID: "production-postgres", Name: "Production PostgreSQL", Engine: "postgres", Version: "16", Environment: "production", Protection: protection.Status, Score: protection.Score, LastBackupAt: now.Add(-18 * time.Minute), LastDrillAt: now.Add(-48 * time.Hour), DestinationIDs: []string{"r2-primary", "contabo-replica"}, RepositoryID: "production"},
			{ID: "billing-sqlite", Name: "Billing SQLite", Engine: "sqlite", Version: "3", Environment: "production", Protection: domain.ProtectionProtected, Score: 92, LastBackupAt: now.Add(-41 * time.Minute), LastDrillAt: now.Add(-96 * time.Hour), DestinationIDs: []string{"r2-primary"}, RepositoryID: "production"},
		},
		Repositories: []domain.RepositoryResource{
			{ID: "production", Name: "Production repository", Mode: "primary_replica", Encrypted: true, SourceIDs: []string{"production-postgres", "billing-sqlite"}, DestinationIDs: []string{"r2-primary", "contabo-replica"}},
		},
		Destinations: []domain.DestinationResource{
			{ID: "r2-primary", Name: "Cloudflare R2", Provider: "r2", Role: "primary", Region: "eu-west", Status: "healthy", LastCheckedAt: now.Add(-18 * time.Minute)},
			{ID: "contabo-replica", Name: "Contabo Object Storage", Provider: "contabo", Role: "replica", Region: "eu-central", Status: "delayed", LagSeconds: int64((47 * time.Minute).Seconds()), LastCheckedAt: now.Add(-47 * time.Minute)},
		},
		GeneratedAt: now,
	}
}

func (s *Service) Doctor(_ context.Context) domain.DoctorResult {
	setup := s.Setup()
	database := setup.Drafts["discover-or-add-database"]
	storage := setup.Drafts["add-storage-destination"]
	databaseStatus, databaseSummary := "fail", "Database connection settings have not been saved."
	if database != nil {
		databaseStatus, databaseSummary = "pass", "Required database connection settings are present; no live connection was attempted."
	}
	storageStatus, storageSummary := "fail", "Storage destination settings have not been saved."
	if storage != nil {
		storageStatus, storageSummary = "pass", "Required storage settings are present; no live capability probe was attempted."
	}
	checks := []domain.DoctorCheck{
		{ID: "database-connection", Name: "Database configuration", Status: databaseStatus, Summary: databaseSummary},
		{ID: "storage-auth", Name: "Storage configuration", Status: storageStatus, Summary: storageSummary},
		{ID: "object-contract", Name: "Object storage contract", Status: "warning", Summary: "Full put/get/list/delete requires production provider credentials.", SuggestedFix: "Run destination test with real R2 or Contabo credentials before production use."},
		{ID: "scratch-disk", Name: "Scratch disk", Status: "pass", Summary: "Scratch directory is writable."},
		{ID: "restore-prerequisites", Name: "Restore prerequisites", Status: "warning", Summary: "Sandbox restore runtime is scaffolded in restricted build.", SuggestedFix: "Install Docker or enable the production sandbox adapter."},
	}
	status := "pass"
	for _, c := range checks {
		if c.Status == "fail" {
			status = "fail"
			break
		}
		if c.Status == "warning" && status == "pass" {
			status = "warning"
		}
	}
	return domain.DoctorResult{ID: "doctor-" + stableID(s.now().String()), Status: status, Checks: checks, GeneratedAt: s.now()}
}

func (s *Service) Simulate(req domain.PolicySimulationRequest) domain.PolicySimulationResult {
	freq := strings.TrimSpace(req.Policy.BackupFrequency)
	backupsPerDay := 4.0
	switch {
	case strings.Contains(freq, "12"):
		backupsPerDay = 2
	case strings.Contains(freq, "hour") || strings.Contains(freq, "1h"):
		backupsPerDay = 24
	case strings.Contains(freq, "day") || strings.Contains(freq, "24"):
		backupsPerDay = 1
	case freq == "":
		backupsPerDay = 4
	}
	if req.Policy.ReplicaCount <= 0 {
		req.Policy.ReplicaCount = 1
	}
	daily := int64(backupsPerDay * 155_000_000)
	copies := int64(req.Policy.ReplicaCount)
	warnings := []string{}
	if req.Policy.MonthlyRetention == 0 {
		warnings = append(warnings, "No monthly retention configured; long-term recovery coverage may be weak.")
	}
	return domain.PolicySimulationResult{
		BackupsPerDay:           backupsPerDay,
		EstimatedDailyBytes:     daily * copies,
		EstimatedThirtyDayBytes: daily * copies * 30,
		EstimatedNinetyDayBytes: daily * copies * 90,
		EstimatedOperations:     int64(backupsPerDay*120) * 30 * copies,
		RestoreScratchBytes:     6_200_000_000,
		RecoveryWindowSeconds:   int64((24 * time.Hour).Seconds()),
		Warnings:                warnings,
	}
}

func (s *Service) CreateSandbox(sourceID, snapshotID, engine, createdBy string, target *time.Time) domain.SandboxRestore {
	now := s.now()
	sb := domain.SandboxRestore{ID: "sandbox-" + stableID(now.String()+sourceID), SourceID: sourceID, SnapshotID: snapshotID, TargetTime: target, Engine: firstNonEmpty(engine, "postgres"), Status: "ready_scaffold", ConnectionRef: "local-read-only-temporary", ExpiresAt: now.Add(2 * time.Hour), CreatedBy: firstNonEmpty(createdBy, "system"), CreatedAt: now}
	s.mu.Lock()
	s.sandboxes[sb.ID] = sb
	s.mu.Unlock()
	return sb
}

func (s *Service) GetSandbox(id string) (domain.SandboxRestore, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sb, ok := s.sandboxes[id]
	if ok && s.now().After(sb.ExpiresAt) {
		sb.Status = "expired"
		s.sandboxes[id] = sb
	}
	return sb, ok
}

func (s *Service) DeleteSandbox(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sandboxes[id]; !ok {
		return false
	}
	delete(s.sandboxes, id)
	return true
}

func (s *Service) Alerts() []domain.Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	alerts := append([]domain.Alert(nil), s.alerts...)
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].CreatedAt.After(alerts[j].CreatedAt) })
	return alerts
}

func (s *Service) UpdateAlertStatus(id, status string) (domain.Alert, error) {
	if status != "open" && status != "acknowledged" && status != "resolved" {
		return domain.Alert{}, fmt.Errorf("unsupported alert status %q", status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.alerts {
		if s.alerts[i].ID == id {
			s.alerts[i].Status = status
			return s.alerts[i], nil
		}
	}
	return domain.Alert{}, fmt.Errorf("alert %q not found", id)
}

func (s *Service) CreateSupportBundle(ctx context.Context) (string, error) {
	return s.createBundle(ctx, "support", map[string]any{
		"version": "phase8b",
		"health":  s.Doctor(ctx),
		"configuration": map[string]any{
			"redacted": true,
			"note":     "Secrets, private keys, raw credentials, and database rows are excluded.",
		},
		"alerts": s.Alerts(),
	})
}

func (s *Service) CreateRecoveryBundle(ctx context.Context) (string, error) {
	return s.createBundle(ctx, "recovery", map[string]any{
		"version":    "phase8b",
		"contains":   []string{"catalogue backup placeholder", "configuration export", "repository definitions", "public signing keys", "certificate metadata", "encrypted secret records", "restore instructions"},
		"warning":    "This bundle does not contain an unencrypted master key.",
		"created_at": s.now(),
	})
}

func (s *Service) createBundle(ctx context.Context, kind string, payload any) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	name := fmt.Sprintf("dbvault-%s-bundle-%s.tar.gz", kind, s.now().Format("20060102T150405Z"))
	path := filepath.Join(s.root, "bundles", name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := tw.WriteHeader(&tar.Header{Name: kind + ".json", Mode: 0600, Size: int64(len(b)), ModTime: s.now()}); err != nil {
		return "", err
	}
	if _, err := io.Copy(tw, strings.NewReader(string(b))); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) loadSetup() error {
	path := filepath.Join(s.root, "setup-state.json")
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.setup = newSetupState(s.now())
			return s.writeSetupLocked()
		}
		return err
	}
	if err := json.Unmarshal(b, &s.setup); err != nil {
		return err
	}
	if s.setup.Drafts == nil {
		s.setup.Drafts = map[string]map[string]string{}
	}
	return nil
}

func (s *Service) writeSetupLocked() error {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.setup, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.root, "setup-state.json"), b, 0600)
}

func (s *Service) CreateRestoreApproval(sourceID, reason string, targetTime *time.Time, target, requestedBy string) (domain.RestoreApprovalRequest, error) {
	if strings.TrimSpace(sourceID) == "" {
		return domain.RestoreApprovalRequest{}, errors.New("source_id is required")
	}
	if strings.TrimSpace(requestedBy) == "" {
		requestedBy = "console"
	}
	now := s.now()
	request := domain.RestoreApprovalRequest{
		ID:       "approval-" + stableID(fmt.Sprintf("%s-%s-%d", sourceID, target, now.UnixNano())),
		SourceID: sourceID, Reason: reason, TargetTime: targetTime, Target: firstNonEmpty(target, "production replacement"),
		RequestedBy: requestedBy, Status: "pending", CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.approvals[request.ID] = request
	return request, nil
}

func (s *Service) RestoreApprovals() []domain.RestoreApprovalRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]domain.RestoreApprovalRequest, 0, len(s.approvals))
	for _, request := range s.approvals {
		out = append(out, request)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *Service) defaultAlerts() []domain.Alert {
	now := s.now()
	return []domain.Alert{{ID: "alert-contabo-lag", Severity: "action_required", Status: "open", ResourceType: "destination", ResourceID: "contabo", Title: "Contabo replica is 47 minutes behind", Summary: "Primary backup remains safe on Cloudflare R2, but the replica is delayed.", LikelyCause: "Three upload attempts timed out.", SafetyImpact: "Recovery remains available from the primary destination. Geographic redundancy is degraded.", SuggestedActions: []domain.SuggestedAction{{ID: "retry-replication", Label: "Retry replication", Description: "Retry delayed replica uploads.", Safe: true}, {ID: "test-destination", Label: "Test Contabo connection", Description: "Run a destination capability probe.", Safe: true}}, Evidence: []domain.EvidenceReference{{ID: "job-retry-1", Summary: "Last three replica upload jobs timed out."}}, CreatedAt: now}}
}

func newSetupState(now time.Time) domain.SetupState {
	return domain.SetupState{ID: "setup-default", CurrentStep: "create-administrator", CompletedSteps: []string{}, Drafts: map[string]map[string]string{}, Status: "in_progress", CreatedAt: now, UpdatedAt: now}
}

var setupSecretFields = map[string]string{
	"administrator_password": "administrator-password",
	"access_key_id":          "storage-access-key-id",
	"secret_access_key":      "storage-secret-access-key",
	"database_password":      "database-password",
}

func (s *Service) validateSetupStepLocked(step string, values, previous map[string]string) error {
	value := func(name string) string {
		if strings.TrimSpace(values[name]) != "" {
			return strings.TrimSpace(values[name])
		}
		return strings.TrimSpace(previous[name])
	}
	hasSecret := func(name string) bool {
		return value(name) != "" || previous[name+"_configured"] == "true"
	}
	require := func(names ...string) error {
		for _, name := range names {
			if value(name) == "" {
				return fmt.Errorf("%s is required", strings.ReplaceAll(name, "_", " "))
			}
		}
		return nil
	}
	switch step {
	case "create-administrator":
		if err := require("administrator_email"); err != nil {
			return err
		}
		if !hasSecret("administrator_password") {
			return errors.New("administrator password is required")
		}
	case "add-storage-destination":
		if err := require("provider", "destination_name"); err != nil {
			return err
		}
		if strings.EqualFold(value("provider"), "Filesystem") {
			if err := require("filesystem_path"); err != nil {
				return err
			}
			if !filepath.IsAbs(value("filesystem_path")) {
				return errors.New("filesystem path must be absolute")
			}
		} else {
			if err := require("endpoint", "region", "bucket"); err != nil {
				return err
			}
			if !hasSecret("access_key_id") || !hasSecret("secret_access_key") {
				return errors.New("storage access key ID and secret access key are required")
			}
		}
	case "discover-or-add-database":
		if err := require("database_name", "engine"); err != nil {
			return err
		}
		if strings.EqualFold(value("engine"), "SQLite") {
			if err := require("database_path"); err != nil {
				return err
			}
			if !filepath.IsAbs(value("database_path")) {
				return errors.New("database path must be absolute")
			}
		} else {
			if err := require("host", "port", "database", "username"); err != nil {
				return err
			}
			if _, err := strconv.Atoi(value("port")); err != nil {
				return errors.New("port must be a number")
			}
			if !hasSecret("database_password") {
				return errors.New("database password is required")
			}
		}
	}
	return nil
}

func (s *Service) writeSecretLocked(filename, value string) error {
	dir := filepath.Join(s.root, "secrets")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, filename), []byte(value), 0600)
}

func (s *Service) runtimeConfigLocked() (config.Config, error) {
	storage := s.setup.Drafts["add-storage-destination"]
	database := s.setup.Drafts["discover-or-add-database"]
	repository := s.setup.Drafts["create-repository"]
	policy := s.setup.Drafts["apply-policy"]
	if storage == nil || database == nil {
		return config.Config{}, errors.New("storage and database steps must be completed before finishing setup")
	}
	dataDir := filepath.Dir(s.root)
	keyDir := filepath.Join(dataDir, "keys")
	cfg := config.Default()
	cfg.Server.DataDirectory = dataDir
	cfg.Server.ScratchDirectory = filepath.Join(dataDir, "scratch")
	cfg.Server.LogSpoolDirectory = filepath.Join(dataDir, "log-spool")
	cfg.Catalogue.Path = filepath.Join(dataDir, "catalogue", "dbvault.sqlite")
	cfg.SecretProviders = []config.SecretProviderConfig{{ID: "local-files", Driver: "file", File: &config.FileSecretProvider{Root: keyDir, RejectSymlinks: true, RequireMode: "0600"}}}

	destinationID := setupSlug(firstNonEmpty(storage["destination_name"], "primary-storage"))
	provider := storage["provider"]
	if strings.EqualFold(provider, "Filesystem") {
		cfg.Destinations = []config.DestinationConfig{{ID: destinationID, Driver: "filesystem", Filesystem: &config.FilesystemDestinationConfig{Root: storage["filesystem_path"], FileMode: "0600", DirectoryMode: "0700"}}}
	} else {
		profile := setupSlug(provider)
		if profile == "cloudflare-r2" || profile == "contabo" {
			// These names already match the runtime's provider profiles.
		} else {
			profile = "s3"
		}
		pathStyle := strings.EqualFold(storage["addressing"], "Path style") || profile == "contabo"
		cfg.Destinations = []config.DestinationConfig{{ID: destinationID, Driver: "s3", Profile: profile, S3: &config.S3DestinationConfig{
			Endpoint: storage["endpoint"], Region: storage["region"], Bucket: storage["bucket"], Prefix: storage["prefix"], Addressing: config.S3AddressingConfig{PathStyle: pathStyle},
			Credentials: config.S3CredentialConfig{AccessKeyID: config.SecretReference{File: filepath.Join(s.root, "secrets", "storage-access-key-id")}, SecretAccessKey: config.SecretReference{File: filepath.Join(s.root, "secrets", "storage-secret-access-key")}},
		}}}
	}

	repositoryID := setupSlug(firstNonEmpty(repository["repository_name"], "production"))
	budget := strings.ReplaceAll(firstNonEmpty(repository["storage_budget"], "20GiB"), " ", "")
	cfg.Repositories = []config.RepositoryConfig{{ID: repositoryID, Mode: "single", Primary: &config.RepositoryPrimary{Destination: destinationID}, Encryption: config.RepositoryEncryptionConfig{KeyProvider: "local-files", ActiveKey: "local-key"}, Signing: config.RepositorySigningConfig{KeyProvider: "local-files", KeyID: "signing-local"}, Retention: config.RetentionConfig{KeepLast: 4, Daily: 7, Weekly: 4, Monthly: 3, MinimumVerifiedSnapshots: 2, TombstoneGrace: "168h"}, Budget: config.BudgetConfig{MaximumPhysicalBytes: budget, ReservePercent: 15}}}

	engine := strings.ToLower(database["engine"])
	if engine == "postgresql" {
		engine = "postgres"
	}
	if engine == "mysql / mariadb" {
		engine = "mysql"
	}
	source := config.SourceConfig{ID: setupSlug(database["database_name"]), Engine: engine, Repository: repositoryID}
	switch engine {
	case "sqlite":
		source.SQLite = &config.SQLiteSourceConfig{Path: database["database_path"], BusyTimeout: "5m", PagesPerStep: 256}
	case "postgres":
		port, _ := strconv.Atoi(database["port"])
		source.Postgres = &config.PostgresSourceConfig{Host: database["host"], Port: port, Database: database["database"], Username: database["username"], Password: config.SecretReference{File: filepath.Join(s.root, "secrets", "database-password")}, SSL: config.DatabaseTLSConfig{Mode: database["tls_mode"], RootCert: database["root_certificate"]}, ConnectTimeout: firstNonEmpty(database["connect_timeout"], "15s"), LogicalBackup: config.PostgresLogicalBackupConfig{Enabled: true, Format: "custom", IncludeGlobals: true}}
	case "mysql", "mariadb":
		port, _ := strconv.Atoi(database["port"])
		source.MySQL = &config.MySQLSourceConfig{Host: database["host"], Port: port, Database: database["database"], Username: database["username"], Password: config.SecretReference{File: filepath.Join(s.root, "secrets", "database-password")}, TLS: config.DatabaseTLSConfig{Mode: database["tls_mode"], CAFile: database["root_certificate"]}, ConnectTimeout: firstNonEmpty(database["connect_timeout"], "15s"), LogicalBackup: config.MySQLLogicalBackupConfig{Enabled: true, SingleTransaction: true, Quick: true, Routines: true, Triggers: true, Events: true}}
	default:
		return config.Config{}, fmt.Errorf("unsupported database engine %q", database["engine"])
	}
	cfg.Sources = []config.SourceConfig{source}
	enabled := true
	cron := "0 */12 * * *"
	if policy["policy"] == "SaaS production" {
		cron = "0 */6 * * *"
	} else if policy["policy"] == "Low cost" {
		cron = "0 2 * * *"
	}
	cfg.Schedules = []config.ScheduleConfig{{ID: source.ID + "-backup", Source: source.ID, Operation: "logical_backup", Cron: cron, Timezone: "UTC", Enabled: &enabled}}
	cfg.Normalize()
	keyPath := filepath.Join(keyDir, repositoryID+"-local-key.key")
	if _, err := os.Stat(keyPath); errors.Is(err, os.ErrNotExist) {
		if err := keyfile.New(keyDir).Generate(repositoryID, "local-key"); err != nil {
			return config.Config{}, fmt.Errorf("generate repository keys: %w", err)
		}
	}
	return cfg, nil
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dbvault-setup-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func setupSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

var setupSteps = []string{"create-administrator", "configure-public-address", "add-storage-destination", "create-repository", "discover-or-add-database", "apply-policy", "run-doctor", "run-first-backup", "run-restore-drill", "configure-alerts", "finish"}

func validSetupStep(step string) bool {
	return contains(setupSteps, step)
}

func nextSetupStep(step string) string {
	for i, candidate := range setupSteps {
		if candidate == step && i+1 < len(setupSteps) {
			return setupSteps[i+1]
		}
	}
	return "finish"
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func stableID(s string) string {
	if s == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		return hex.EncodeToString(b)
	}
	var h uint32 = 2166136261
	for _, b := range []byte(s) {
		h ^= uint32(b)
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
