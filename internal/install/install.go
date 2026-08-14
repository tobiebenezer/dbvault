package install

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Options struct {
	Root          string
	Domain        string
	SelfSigned    bool
	OfflineBundle string
	DryRun        bool
	Force         bool
	BinaryPath    string
	Version       string
}

type Result struct {
	Steps      []Step `json:"steps"`
	SetupURL   string `json:"setup_url"`
	SetupToken string `json:"setup_token"`
	ConfigPath string `json:"config_path"`
	UnitPath   string `json:"unit_path"`
}

type Step struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	StartedAt string `json:"started_at"`
}

func Plan(o Options) []string {
	steps := []string{"preflight", "create-system-user", "create-directories", "install-binary", "write-configuration", "write-systemd-service", "generate-setup-token", "start-service", "verify-readiness"}
	if o.SelfSigned {
		steps = append(steps, "generate-self-signed-certificate")
	}
	return steps
}

func Apply(o Options) (Result, error) {
	o = normalise(o)
	result := Result{Steps: []Step{}, ConfigPath: joinRoot(o.Root, "/etc/dbvault/dbvault.yaml"), UnitPath: joinRoot(o.Root, "/etc/systemd/system/dbvault.service")}
	record := func(name, status, msg string) {
		result.Steps = append(result.Steps, Step{Name: name, Status: status, Message: msg, StartedAt: time.Now().UTC().Format(time.RFC3339)})
	}
	for _, step := range Plan(o) {
		record(step, "planned", "")
	}
	if o.DryRun {
		result.SetupURL = setupURL(o)
		return result, nil
	}
	for _, dir := range []string{"/etc/dbvault", "/etc/dbvault/tls", "/var/lib/dbvault", "/var/lib/dbvault/catalogue", "/var/lib/dbvault/repository", "/var/lib/dbvault/scratch", "/var/lib/dbvault/log-spool", "/var/lib/dbvault/certificates", "/var/log/dbvault"} {
		if err := os.MkdirAll(joinRoot(o.Root, dir), 0700); err != nil {
			return result, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(result.ConfigPath, []byte(DefaultConfig(o)), 0640); err != nil {
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(result.UnitPath), 0755); err != nil {
		return result, err
	}
	if err := os.WriteFile(result.UnitPath, []byte(SystemdUnit(o)), 0644); err != nil {
		return result, err
	}
	token, err := GenerateSetupToken()
	if err != nil {
		return result, err
	}
	setupTokenPath := joinRoot(o.Root, "/var/lib/dbvault/setup-token")
	if err := os.WriteFile(setupTokenPath, []byte(token+"\n"), 0600); err != nil {
		return result, err
	}
	result.SetupToken = token
	result.SetupURL = setupURL(o)
	return result, nil
}

func normalise(o Options) Options {
	if o.Root == "" {
		o.Root = "/"
	}
	if o.Version == "" {
		o.Version = "dev"
	}
	if o.BinaryPath == "" {
		o.BinaryPath = "/usr/local/bin/dbvault"
	}
	return o
}

func GenerateSetupToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := strings.ToUpper(hex.EncodeToString(b))
	return s[0:4] + "-" + s[4:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:24] + "-" + s[24:28] + "-" + s[28:32], nil
}

func setupURL(o Options) string {
	if o.Domain == "" {
		return "https://127.0.0.1/setup"
	}
	if strings.HasPrefix(o.Domain, "http://") || strings.HasPrefix(o.Domain, "https://") {
		return strings.TrimRight(o.Domain, "/") + "/setup"
	}
	return "https://" + o.Domain + "/setup"
}

func joinRoot(root, p string) string {
	if root == "" || root == "/" {
		return p
	}
	return filepath.Join(root, strings.TrimPrefix(p, "/"))
}

func DefaultConfig(o Options) string {
	tlsMode := "self_signed"
	if !o.SelfSigned && o.Domain != "" {
		tlsMode = "acme"
	}
	publicURL := setupURL(o)
	publicURL = strings.TrimSuffix(publicURL, "/setup")
	return fmt.Sprintf(`version: 1
server:
  data_directory: /var/lib/dbvault
  scratch_directory: /var/lib/dbvault/scratch
  log_spool_directory: /var/lib/dbvault/log-spool
  listen: 0.0.0.0:443
  public_url: %q
  tls:
    mode: %s
catalogue:
  driver: sqlite
  path: /var/lib/dbvault/catalogue/dbvault.sqlite
`, publicURL, tlsMode)
}

func SystemdUnit(o Options) string {
	binary := o.BinaryPath
	if binary == "" {
		binary = "/usr/local/bin/dbvault"
	}
	return fmt.Sprintf(`[Unit]
Description=DBVault Database Protection Platform
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=dbvault
Group=dbvault
ExecStart=%s server --config /etc/dbvault/dbvault.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
WorkingDirectory=/var/lib/dbvault
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
RestrictRealtime=true
ReadWritePaths=/var/lib/dbvault
ReadWritePaths=/var/log/dbvault
ReadWritePaths=/etc/dbvault/tls
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
`, binary)
}

type UpgradePlan struct {
	CurrentVersion string
	TargetVersion  string
	Steps          []string
}

func PlanUpgrade(current, target string) UpgradePlan {
	return UpgradePlan{CurrentVersion: current, TargetVersion: target, Steps: []string{"download-manifest", "verify-signature", "verify-checksum", "checkpoint-jobs", "backup-catalogue", "install-binary-atomically", "restart", "health-check", "commit-upgrade"}}
}

type UninstallPlan struct {
	PurgeData bool
	Steps     []string
}

func PlanUninstall(purge bool) UninstallPlan {
	steps := []string{"stop-service", "disable-service", "remove-binary", "preserve-configuration", "preserve-data"}
	if purge {
		steps = append(steps, "purge-configuration", "purge-data")
	}
	return UninstallPlan{PurgeData: purge, Steps: steps}
}
