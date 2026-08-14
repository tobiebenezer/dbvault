package provisioning

import (
	"strings"
	"testing"
)

func TestInstallJobStages(t *testing.T) {
	job := NewInstallJob("job-1", "server-a")
	if len(job.Stages) < 10 {
		t.Fatalf("expected detailed stages, got %d", len(job.Stages))
	}
	if job.Stages[2].Name != "preflight" {
		t.Fatalf("unexpected stage order: %+v", job.Stages)
	}
}

func TestRedactOutput(t *testing.T) {
	out := RedactOutput("password=abc token=xyz Authorization: Bearer verysecret")
	if strings.Contains(out, "abc") || strings.Contains(out, "verysecret") {
		t.Fatalf("secret leaked: %s", out)
	}
}

func TestAgentOneLineInstall(t *testing.T) {
	cmd := AgentOneLineInstall("https://backup.example.test/", "tok")
	if !strings.Contains(cmd, "/install/agent.sh") || !strings.Contains(cmd, "DBVAULT_AGENT_TOKEN=tok") {
		t.Fatal(cmd)
	}
}
