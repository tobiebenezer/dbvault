package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyCreatesSelfContainedLayout(t *testing.T) {
	root := t.TempDir()
	res, err := Apply(Options{Root: root, Domain: "backup.example.test", SelfSigned: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"etc/dbvault/dbvault.yaml", "etc/systemd/system/dbvault.service", "var/lib/dbvault/setup-token", "var/lib/dbvault/scratch"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	if res.SetupToken == "" || !strings.Contains(res.SetupURL, "backup.example.test") {
		t.Fatalf("bad setup result: %+v", res)
	}
	unit, _ := os.ReadFile(filepath.Join(root, "etc/systemd/system/dbvault.service"))
	if !strings.Contains(string(unit), "dbvault server") {
		t.Fatal("unit does not launch self-contained server mode")
	}
}

func TestDryRunDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	_, err := Apply(Options{Root: root, Domain: "backup.example.test", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/dbvault/dbvault.yaml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote config: %v", err)
	}
}

func TestPlanUpgradeAndUninstall(t *testing.T) {
	if len(PlanUpgrade("a", "b").Steps) == 0 {
		t.Fatal("empty upgrade plan")
	}
	if !PlanUninstall(true).PurgeData {
		t.Fatal("purge flag lost")
	}
}
