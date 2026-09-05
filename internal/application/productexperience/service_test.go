package productexperience

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
)

func TestProtectionSummaryIsDeterministicAndActionable(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC) }
	got := svc.ProtectionSummary(context.Background(), "prod")
	if got.Status != domain.ProtectionAtRisk {
		t.Fatalf("status=%s", got.Status)
	}
	if got.Score != 84 {
		t.Fatalf("score=%d", got.Score)
	}
	if len(got.SuggestedActions) == 0 || !got.SuggestedActions[0].Safe {
		t.Fatalf("expected safe suggested action: %#v", got.SuggestedActions)
	}
}

func TestSetupStatePersists(t *testing.T) {
	root := t.TempDir()
	svc, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err := svc.SaveSetupStep("add-storage-destination", []byte(`{"provider":"Filesystem","destination_name":"Local","filesystem_path":"/srv/dbvault"}`))
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentStep != "create-repository" {
		t.Fatalf("next step=%s", state.CurrentStep)
	}
	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Setup().CurrentStep != "create-repository" {
		t.Fatalf("state was not persisted: %#v", reloaded.Setup())
	}
}

func TestSetupDraftsAccumulateWithoutPersistingSecrets(t *testing.T) {
	root := t.TempDir()
	svc, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveSetupStep("add-storage-destination", []byte(`{"provider":"Filesystem","destination_name":"Local backups","filesystem_path":"/srv/dbvault"}`)); err != nil {
		t.Fatal(err)
	}
	state, err := svc.SaveSetupStep("discover-or-add-database", []byte(`{"database_name":"Orders","engine":"PostgreSQL","host":"db.internal","port":"5432","database":"orders","username":"backup","database_password":"super-secret","tls_mode":"verify-full","connect_timeout":"15s"}`))
	if err != nil {
		t.Fatal(err)
	}
	if state.Drafts["add-storage-destination"]["filesystem_path"] != "/srv/dbvault" {
		t.Fatalf("storage draft was overwritten: %#v", state.Drafts)
	}
	if state.Drafts["discover-or-add-database"]["username"] != "backup" {
		t.Fatalf("database draft was not retained: %#v", state.Drafts)
	}
	if _, ok := state.Drafts["discover-or-add-database"]["database_password"]; ok {
		t.Fatalf("password leaked into API state: %#v", state.Drafts)
	}
	persisted, err := os.ReadFile(filepath.Join(root, "setup-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persisted), "super-secret") {
		t.Fatal("database password was persisted in setup-state.json")
	}
	secret, err := os.ReadFile(filepath.Join(root, "secrets", "database-password"))
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "super-secret" {
		t.Fatalf("unexpected secret contents %q", secret)
	}
	state, err = svc.FinishSetup()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].Postgres == nil {
		t.Fatalf("postgres connection missing from generated config: %#v", cfg.Sources)
	}
	resolved, err := cfg.ResolveSecret(cfg.Sources[0].Postgres.Password, true)
	if err != nil || resolved != "super-secret" {
		t.Fatalf("generated password reference does not resolve: value=%q err=%v", resolved, err)
	}
}

func TestFinishSetupWritesValidatedRuntimeConfig(t *testing.T) {
	root := t.TempDir()
	svc, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	steps := map[string]string{
		"add-storage-destination":  `{"provider":"Filesystem","destination_name":"Local backups","filesystem_path":"` + filepath.Join(root, "repository") + `"}`,
		"create-repository":        `{"repository_name":"Production","repository_mode":"Single destination","storage_budget":"20GiB"}`,
		"discover-or-add-database": `{"database_name":"Billing SQLite","engine":"SQLite","database_path":"` + filepath.Join(root, "billing.sqlite") + `"}`,
		"apply-policy":             `{"policy":"Starter"}`,
	}
	for _, step := range []string{"add-storage-destination", "create-repository", "discover-or-add-database", "apply-policy"} {
		if _, err := svc.SaveSetupStep(step, []byte(steps[step])); err != nil {
			t.Fatalf("save %s: %v", step, err)
		}
	}
	state, err := svc.FinishSetup()
	if err != nil {
		t.Fatal(err)
	}
	if state.ConfigPath == "" {
		t.Fatal("finish did not expose the generated config path")
	}
	b, err := os.ReadFile(state.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("generated config is invalid: %v\n%s", err, b)
	}
	if len(cfg.Sources) != 1 || cfg.Sources[0].SQLite == nil || cfg.Sources[0].SQLite.Path == "" {
		t.Fatalf("database connection missing from generated config: %#v", cfg.Sources)
	}
}

func TestDiscoveryDoesNotBackUpAutomatically(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "app.sqlite"), []byte("not read"), 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	items, err := svc.Discover(context.Background(), "local", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	foundSQLite := false
	for _, item := range items {
		if item.Engine == "sqlite" {
			foundSQLite = true
			if item.Status != "candidate" {
				t.Fatalf("discovery should remain candidate, got %s", item.Status)
			}
		}
	}
	if !foundSQLite {
		t.Fatalf("expected sqlite discovery in %#v", items)
	}
}

func TestPolicySimulationAndBundles(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result := svc.Simulate(domain.PolicySimulationRequest{Policy: domain.ProtectionPolicyDraft{BackupFrequency: "Every 12 hours", ReplicaCount: 2}})
	if result.BackupsPerDay != 2 || result.EstimatedThirtyDayBytes == 0 {
		t.Fatalf("bad simulation: %#v", result)
	}
	path, err := svc.CreateSupportBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestJobsCanProgressCancelAndRetry(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	job := svc.CreateJob("backup", "production-postgres", "Production PostgreSQL")
	if job.Status != "queued" {
		t.Fatalf("expected queued job, got %s", job.Status)
	}
	events := svc.GenerateDemoProgress()
	if len(events) == 0 {
		t.Fatal("expected progress events")
	}
	if _, ok := svc.Job(job.ID); !ok {
		t.Fatalf("created job disappeared")
	}
	cancelled, err := svc.CancelJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" || cancelled.CanCancel {
		t.Fatalf("bad cancelled job: %#v", cancelled)
	}
	result, err := svc.RetryJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.OriginalJobID != job.ID || result.NewJobID == "" || result.Job.OriginalJobID != job.ID {
		t.Fatalf("bad retry result: %#v", result)
	}
}

func TestJobEventsAreIncremental(t *testing.T) {
	svc, err := NewWithDemo(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	initial := svc.JobEventsSince("", 100)
	if len(initial) == 0 {
		t.Fatal("expected seeded job events")
	}
	last := initial[len(initial)-1].EventID
	job := svc.CreateJob("restore_drill", "production-postgres", "Production PostgreSQL")
	next := svc.JobEventsSince(last, 100)
	if len(next) == 0 || next[0].JobID != job.ID {
		t.Fatalf("expected only new event for %s, got %#v", job.ID, next)
	}
}

func TestSetupStepCanMoveBackAndPersists(t *testing.T) {
	root := t.TempDir()
	svc, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	state, err := svc.SetSetupStep("run-doctor")
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentStep != "run-doctor" {
		t.Fatalf("current step=%s", state.CurrentStep)
	}
	if _, err := svc.SetSetupStep("not-a-step"); err == nil {
		t.Fatal("expected unknown setup step to fail")
	}
	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Setup().CurrentStep != "run-doctor" {
		t.Fatalf("step was not persisted: %#v", reloaded.Setup())
	}
}

func TestDiscoveryAndAlertStatusActions(t *testing.T) {
	svc, err := NewWithDemo(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	items, err := svc.Discover(context.Background(), "local", nil)
	if err != nil || len(items) == 0 {
		t.Fatalf("discoveries=%#v err=%v", items, err)
	}
	adopted, err := svc.UpdateDiscoveryStatus(items[0].ID, "adopted")
	if err != nil || adopted.Status != "adopted" {
		t.Fatalf("adopted=%#v err=%v", adopted, err)
	}
	alerts := svc.Alerts()
	if len(alerts) == 0 || alerts[0].Status != "open" {
		t.Fatalf("alerts=%#v", alerts)
	}
	acknowledged, err := svc.UpdateAlertStatus(alerts[0].ID, "acknowledged")
	if err != nil || acknowledged.Status != "acknowledged" {
		t.Fatalf("alert=%#v err=%v", acknowledged, err)
	}
}

func TestInventoryProvidesConnectedResources(t *testing.T) {
	svc, err := NewWithDemo(t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	inventory := svc.Inventory()
	if len(inventory.Databases) < 1 || len(inventory.Repositories) != 1 || len(inventory.Destinations) != 2 {
		t.Fatalf("unexpected inventory: %#v", inventory)
	}
	if inventory.Databases[0].RepositoryID != inventory.Repositories[0].ID {
		t.Fatalf("database repository is not connected: %#v", inventory)
	}
}

func TestRestoreApprovalRequestIsRecorded(t *testing.T) {
	svc, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request, err := svc.CreateRestoreApproval("production-postgres", "bad deployment", nil, "production replacement", "console")
	if err != nil {
		t.Fatal(err)
	}
	if request.ID == "" || request.Status != "pending" {
		t.Fatalf("unexpected approval: %#v", request)
	}
	if got := svc.RestoreApprovals(); len(got) != 1 || got[0].ID != request.ID {
		t.Fatalf("approval was not retained: %#v", got)
	}
}
