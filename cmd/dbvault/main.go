package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dbvault/dbvault/internal/adapters/catalogue/memory"
	gzipc "github.com/dbvault/dbvault/internal/adapters/compression/gzip"
	"github.com/dbvault/dbvault/internal/adapters/encryption/aead"
	"github.com/dbvault/dbvault/internal/adapters/id"
	jobqueue "github.com/dbvault/dbvault/internal/adapters/jobqueue/sqlite"
	ms "github.com/dbvault/dbvault/internal/adapters/manifest"
	"github.com/dbvault/dbvault/internal/adapters/notification/webhook"
	"github.com/dbvault/dbvault/internal/adapters/scratch"
	"github.com/dbvault/dbvault/internal/adapters/source/mysql"
	"github.com/dbvault/dbvault/internal/adapters/source/postgres"
	"github.com/dbvault/dbvault/internal/adapters/source/sqlite"
	"github.com/dbvault/dbvault/internal/adapters/storage/filesystem"
	"github.com/dbvault/dbvault/internal/adapters/telemetry/prometheus"
	"github.com/dbvault/dbvault/internal/application/backup"
	"github.com/dbvault/dbvault/internal/application/controlplane"
	"github.com/dbvault/dbvault/internal/application/database"
	"github.com/dbvault/dbvault/internal/application/doctor"
	"github.com/dbvault/dbvault/internal/application/garbagecollection"
	"github.com/dbvault/dbvault/internal/application/productexperience"
	"github.com/dbvault/dbvault/internal/application/scheduler"
	"github.com/dbvault/dbvault/internal/bootstrap"
	"github.com/dbvault/dbvault/internal/config"
	"github.com/dbvault/dbvault/internal/domain"
	installer "github.com/dbvault/dbvault/internal/install"
	"github.com/dbvault/dbvault/internal/platform/controllerapi"
	"github.com/dbvault/dbvault/internal/ports"
	dbvserver "github.com/dbvault/dbvault/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	switch os.Args[1] {
	case "server":
		serverCmd(os.Args[2:])
	case "install":
		installCmd(os.Args[2:])
	case "upgrade":
		upgradeCmd(os.Args[2:])
	case "uninstall":
		uninstallCmd(os.Args[2:])
	case "agent":
		agentCmd(os.Args[2:])
	case "config":
		configCmd(os.Args[2:])
	case "key":
		keyCmd(os.Args[2:])
	case "source":
		sourceCmd(os.Args[2:])
	case "destination":
		destinationCmd(os.Args[2:])
	case "repository":
		repositoryCmd(os.Args[2:])
	case "backup-create":
		backupCreate(os.Args[2:])
	case "backup-list":
		backupList(os.Args[2:])
	case "restore-plan":
		restorePlan(os.Args[2:])
	case "restore-run":
		restoreRun(os.Args[2:])
	case "doctor":
		doctorCmd(os.Args[2:])
	case "protection":
		protectionCmd(os.Args[2:])
	case "bundle":
		bundleCmd(os.Args[2:])
	case "engine":
		engineCmd(os.Args[2:])
	case "recovery":
		recoveryCmd(os.Args[2:])
	case "postgres":
		postgresRecoveryCmd(os.Args[2:])
	case "mysql":
		mysqlRecoveryCmd(os.Args[2:])
	case "probe":
		probeCmd(os.Args[2:])
	default:
		usage()
	}
}
func usage() {
	fmt.Println("dbvault commands: server, install, upgrade, uninstall, probe, agent install|enrol|status, config validate|print-effective, source test|inspect, destination test|capabilities, repository explain, key generate, engine list|inspect, backup-create, backup-list, restore-plan, restore-run, recovery window|plan|verify-chain|drill, postgres wal|physical-backup|restore-point, mysql binlog, doctor, protection status, bundle support|recovery create")
}
func loadApp(configPath string) (*bootstrap.Application, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return bootstrap.Build(context.Background(), cfg, slog.Default())
}
func configCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("config commands: validate, print-effective")
		return
	}
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	_ = fs.Parse(args[1:])
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	switch args[0] {
	case "validate":
		fmt.Println("config OK")
	case "print-effective":
		b, marshalErr := config.MarshalRedactedJSON(cfg)
		if marshalErr != nil {
			fmt.Println(marshalErr)
			os.Exit(1)
		}
		fmt.Println(string(b))
	default:
		fmt.Println("unknown config command")
	}
}
func keyCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("DBVault Master Key Management")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  dbvault key status               Show active master encryption key metadata & fingerprint")
		fmt.Println("  dbvault key generate [--out path] Generate a new 256-bit AEAD AES Master Key & Disaster Recovery Sheet")
		fmt.Println("  dbvault key set --key <val>      Set or import an existing 256-bit Hex Key or Passphrase")
		fmt.Println("  dbvault key export               Export the Master Key & printable Disaster Recovery Kit")
		return
	}

	keyPath := os.Getenv("DBVAULT_KEY_FILE")
	if keyPath == "" {
		keyPath = "./scratch/data/master.key"
	}

	switch args[0] {
	case "status", "show":
		var keyBytes []byte
		var source string
		if envKey := os.Getenv("DBVAULT_MASTER_KEY"); strings.TrimSpace(envKey) != "" {
			source = "Environment Variable (DBVAULT_MASTER_KEY)"
			if b, err := hex.DecodeString(strings.TrimSpace(envKey)); err == nil && len(b) == 32 {
				keyBytes = b
			} else {
				h := sha256.Sum256([]byte(strings.TrimSpace(envKey)))
				keyBytes = h[:]
			}
		} else if b, err := os.ReadFile(keyPath); err == nil && len(b) > 0 {
			source = "Key File: " + keyPath
			trimmed := strings.TrimSpace(string(b))
			if dec, err := hex.DecodeString(trimmed); err == nil && len(dec) == 32 {
				keyBytes = dec
			} else {
				h := sha256.Sum256([]byte(trimmed))
				keyBytes = h[:]
			}
		}

		if len(keyBytes) == 0 {
			fmt.Println("Status:      No master key configured (will auto-generate on first backup)")
			fmt.Println("Default Path:", keyPath)
			return
		}
		h := sha256.Sum256(keyBytes)
		fp := "sha256:" + hex.EncodeToString(h[:])[:16]
		fmt.Println("Status:       Active & Ready")
		fmt.Println("Source:      ", source)
		fmt.Println("Algorithm:    AEAD AES-256-GCM (256-bit)")
		fmt.Println("Fingerprint: ", fp)
		fmt.Println("")
		fmt.Println("Tip: Run 'dbvault key export' to view the Emergency Disaster Recovery Kit.")

	case "generate":
		fs := flag.NewFlagSet("key generate", flag.ExitOnError)
		out := fs.String("out", keyPath, "destination key file path")
		_ = fs.Parse(args[1:])

		key := make([]byte, 32)
		_, _ = rand.Read(key)
		hexStr := hex.EncodeToString(key)
		_ = os.MkdirAll(filepath.Dir(*out), 0700)
		if err := os.WriteFile(*out, []byte(hexStr+"\n"), 0600); err != nil {
			fmt.Printf("Error writing key to %s: %v\n", *out, err)
			os.Exit(1)
		}
		h := sha256.Sum256(key)
		fp := "sha256:" + hex.EncodeToString(h[:])[:16]

		fmt.Println("================================================================================")
		fmt.Println("DBVAULT EMERGENCY DISASTER RECOVERY KIT")
		fmt.Println("================================================================================")
		fmt.Println("CRITICAL: Save this Master Key in your password manager (1Password / Bitwarden).")
		fmt.Println("Without this key, backups in Cloudflare R2 / S3 cannot be decrypted if the server is lost.")
		fmt.Println("")
		fmt.Println("Master Key (HEX):  ", hexStr)
		fmt.Println("Key Fingerprint:   ", fp)
		fmt.Println("Saved to:          ", *out)
		fmt.Println("Cipher:             AEAD AES-256-GCM (256-bit)")
		fmt.Println("================================================================================")

	case "set":
		fs := flag.NewFlagSet("key set", flag.ExitOnError)
		rawKey := fs.String("key", "", "32-byte hex key or passphrase")
		out := fs.String("out", keyPath, "destination key file path")
		_ = fs.Parse(args[1:])

		if strings.TrimSpace(*rawKey) == "" {
			fmt.Println("Error: --key argument is required (e.g. dbvault key set --key <hex-key-or-passphrase>)")
			os.Exit(1)
		}
		trimmed := strings.TrimSpace(*rawKey)
		var keyBytes []byte
		if dec, err := hex.DecodeString(trimmed); err == nil && len(dec) == 32 {
			keyBytes = dec
		} else {
			h := sha256.Sum256([]byte(trimmed))
			keyBytes = h[:]
		}
		hexStr := hex.EncodeToString(keyBytes)
		_ = os.MkdirAll(filepath.Dir(*out), 0700)
		if err := os.WriteFile(*out, []byte(hexStr+"\n"), 0600); err != nil {
			fmt.Printf("Error writing key to %s: %v\n", *out, err)
			os.Exit(1)
		}
		h := sha256.Sum256(keyBytes)
		fp := "sha256:" + hex.EncodeToString(h[:])[:16]
		fmt.Println("Master Key successfully updated & verified.")
		fmt.Println("Fingerprint: ", fp)
		fmt.Println("Saved to:    ", *out)

	case "export":
		var keyBytes []byte
		var source string
		if envKey := os.Getenv("DBVAULT_MASTER_KEY"); strings.TrimSpace(envKey) != "" {
			source = "env:DBVAULT_MASTER_KEY"
			if b, err := hex.DecodeString(strings.TrimSpace(envKey)); err == nil && len(b) == 32 {
				keyBytes = b
			} else {
				h := sha256.Sum256([]byte(strings.TrimSpace(envKey)))
				keyBytes = h[:]
			}
		} else if b, err := os.ReadFile(keyPath); err == nil && len(b) > 0 {
			source = keyPath
			trimmed := strings.TrimSpace(string(b))
			if dec, err := hex.DecodeString(trimmed); err == nil && len(dec) == 32 {
				keyBytes = dec
			} else {
				h := sha256.Sum256([]byte(trimmed))
				keyBytes = h[:]
			}
		}
		if len(keyBytes) == 0 {
			fmt.Println("No master key found. Run 'dbvault key generate' to create one.")
			os.Exit(1)
		}
		hexStr := hex.EncodeToString(keyBytes)
		h := sha256.Sum256(keyBytes)
		fp := "sha256:" + hex.EncodeToString(h[:])[:16]

		fmt.Println("================================================================================")
		fmt.Println("DBVAULT EMERGENCY DISASTER RECOVERY KIT")
		fmt.Println("================================================================================")
		fmt.Println("Master Key (HEX):  ", hexStr)
		fmt.Println("Key Fingerprint:   ", fp)
		fmt.Println("Source Path:       ", source)
		fmt.Println("Encryption Cipher:  AEAD AES-256-GCM (256-bit)")
		fmt.Println("Exported At:       ", time.Now().UTC().Format(time.RFC3339))
		fmt.Println("")
		fmt.Println("HOW TO RESTORE ON A NEW SERVER FROM ZERO:")
		fmt.Println("1. Download DBVault on any fresh machine:")
		fmt.Println("   curl -fsSL https://get.dbvault.io | sh")
		fmt.Println("")
		fmt.Println("2. Export your Master Key:")
		fmt.Printf("   export DBVAULT_MASTER_KEY=\"%s\"\n", hexStr)
		fmt.Println("")
		fmt.Println("3. Restore your database directly from Cloudflare R2 / S3:")
		fmt.Println("   dbvault restore --snapshot latest --target postgresql://postgres:pass@localhost:5432/my_database")
		fmt.Println("================================================================================")

	default:
		fmt.Println("unknown key command. Try: status, generate, set, export")
	}
}
func backupCreate(args []string) {
	fs := flag.NewFlagSet("backup-create", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	sourcePath := fs.String("source", "", "legacy SQLite source path")
	repoPath := fs.String("repo", "./repository", "legacy repository directory")
	scratchPath := fs.String("scratch", "./scratch", "legacy scratch directory")
	keyHex := fs.String("key", "", "legacy 32-byte hex or raw key string")
	_ = fs.Parse(args)
	if *cfgPath != "" {
		app, err := loadApp(*cfgPath)
		if err != nil {
			fmt.Println("startup failed:", err)
			os.Exit(1)
		}
		defer app.Close(context.Background())
		src, err := domainSource(app.Binding)
		if err != nil {
			fmt.Println("source error:", err)
			os.Exit(1)
		}
		res, err := app.Backup.Create(context.Background(), backup.Command{Source: src, Trigger: domain.TriggerManual})
		if err != nil {
			fmt.Println("backup failed:", err)
			os.Exit(1)
		}
		printBackup(res)
		return
	}
	legacyBackup(*sourcePath, *repoPath, *scratchPath, *keyHex)
}
func printBackup(res backup.Result) {
	if res.Duplicate {
		fmt.Println("duplicate snapshot:", res.Snapshot.ID)
		return
	}
	fmt.Println("backup committed:", res.Snapshot.ID)
	fmt.Println("manifest:", res.Snapshot.ManifestObjectKey)
}
func legacyBackup(sourcePath, repoPath, scratchPath, keyHex string) {
	if sourcePath == "" || keyHex == "" {
		fmt.Println("source and key are required")
		os.Exit(2)
	}
	if !filepath.IsAbs(sourcePath) {
		abs, _ := filepath.Abs(sourcePath)
		sourcePath = abs
	}
	key := []byte(keyHex)
	if decoded, err := hex.DecodeString(keyHex); err == nil && len(decoded) == 32 {
		key = decoded
	}
	if len(key) < 32 {
		p := make([]byte, 32)
		copy(p, key)
		key = p
	}
	if len(key) > 32 {
		key = key[:32]
	}
	enc, err := aead.New(key)
	if err != nil {
		panic(err)
	}
	cat := memory.New()
	store := filesystem.New(repoPath)
	if err := store.Validate(context.Background()); err != nil {
		panic(err)
	}
	repo := domain.Repository{ID: "repo_local", Name: "local", Mode: domain.RepositorySingle, PrimaryDestination: "filesystem", Encryption: domain.EncryptionPolicy{Algorithm: "aes-256-gcm", KeyID: "local-key"}}
	svc := backup.Service{Catalogue: cat, Source: sqlite.New("sqlite3"), Store: store, Scratch: scratch.New(scratchPath), Compressor: gzipc.New(6), Encryptor: enc, Signer: ms.NewHMACSigner(key), Clock: ports.SystemClock{}, IDs: id.Generator{}, DedupKey: key, Repository: repo, TargetChunkBytes: 4 * 1024 * 1024}
	source := domain.Source{ID: "source_local", Name: "local", Driver: "sqlite", Enabled: true, Path: sourcePath, RepositoryID: repo.ID}
	res, err := svc.Create(context.Background(), backup.Command{Source: source, Trigger: domain.TriggerManual})
	if err != nil {
		fmt.Println("backup failed:", err)
		os.Exit(1)
	}
	printBackup(res)
}
func backupList(args []string) {
	fs := flag.NewFlagSet("backup-list", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	_ = fs.Parse(args)
	app, err := loadApp(*cfgPath)
	if err != nil {
		fmt.Println("startup failed:", err)
		os.Exit(1)
	}
	defer app.Close(context.Background())
	snaps, err := app.Catalogue.ListSnapshots(context.Background(), "")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	for _, s := range snaps {
		fmt.Printf("%s\t%s\t%d bytes\t%s\n", s.ID, s.Status, s.DatabaseSize, s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
}
func restorePlan(args []string) {
	fs := flag.NewFlagSet("restore-plan", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	snap := fs.String("snapshot", "", "snapshot id")
	target := fs.String("target", "", "target path")
	_ = fs.Parse(args)
	app, err := loadApp(*cfgPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer app.Close(context.Background())
	p, err := app.Restore.Plan(context.Background(), domain.SnapshotID(*snap), *target)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Printf("snapshot=%s size=%d chunks=%d overwrite_required=%v\n", p.SnapshotID, p.LogicalSize, p.ChunkCount, p.OverwriteRequired)
}
func restoreRun(args []string) {
	fs := flag.NewFlagSet("restore-run", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	snap := fs.String("snapshot", "", "snapshot id")
	target := fs.String("target", "", "target path")
	replace := fs.Bool("replace", false, "replace target")
	_ = fs.Parse(args)
	app, err := loadApp(*cfgPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer app.Close(context.Background())
	if err := app.Restore.Restore(context.Background(), domain.SnapshotID(*snap), *target, *replace); err != nil {
		fmt.Println("restore failed:", err)
		os.Exit(1)
	}
	fmt.Println("restore completed:", *target)
}
func doctorCmd(args []string) {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path (required)")
	_ = fs.Parse(args)
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "dbvault doctor: --config is required")
		os.Exit(2)
	}
	report, err := doctorRun(context.Background(), *cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor failed:", err)
		os.Exit(1)
	}
	for _, c := range report.Checks {
		marker := "ok  "
		if !c.OK() {
			marker = "FAIL"
		}
		fmt.Printf("%s\t%s\t%s\n", marker, c.Name, c.Detail)
	}
	if !report.Healthy() {
		failed := 0
		for _, c := range report.Checks {
			if !c.OK() {
				failed++
			}
		}
		fmt.Fprintf(os.Stderr, "doctor: %d check(s) failed\n", failed)
		os.Exit(1)
	}
	fmt.Println("doctor OK")
}

func doctorRun(ctx context.Context, configPath string) (doctor.Report, error) {
	return doctor.Run(ctx, configPath, slog.Default())
}

func engineCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("engine commands: list, inspect <engine>")
		return
	}
	reg := database.NewRegistry(
		postgres.New(nil, postgres.DefaultConfig()),
		mysql.New(nil, mysql.DefaultConfig("mysql")),
		mysql.New(nil, mysql.DefaultConfig("mariadb")),
	)
	switch args[0] {
	case "list":
		for _, d := range reg.List() {
			fmt.Printf("%s\tapi=%d\tstreaming=%v\tparallel=%v\tglobals=%v\n", d.Engine, d.API, d.SupportsStreaming, d.SupportsParallel, d.SupportsGlobals)
		}
	case "inspect":
		if len(args) < 2 {
			fmt.Println("usage: dbvault engine inspect <postgres|mysql|mariadb>")
			return
		}
		eng := domain.DatabaseEngine(args[1])
		d, ok := reg.Get(eng)
		if !ok {
			fmt.Println("unknown engine:", args[1])
			os.Exit(1)
		}
		desc := d.Descriptor()
		fmt.Printf("engine=%s api=%d formats=%v modes=%v streaming=%v parallel=%v globals=%v\n", desc.Engine, desc.API, desc.SupportedFormats, desc.SupportedModes, desc.SupportsStreaming, desc.SupportsParallel, desc.SupportsGlobals)
	default:
		fmt.Println("unknown engine command")
	}
}

func recoveryCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("recovery commands: window show, plan, verify-chain, drill")
		return
	}
	switch args[0] {
	case "window":
		if len(args) >= 2 && args[1] == "show" {
			fs := flag.NewFlagSet("recovery window show", flag.ExitOnError)
			source := fs.String("source", "", "source id")
			_ = fs.Parse(args[2:])
			fmt.Printf("source=%s continuous=false windows=0 note=run Phase 5 collectors to calculate remote recovery windows\n", *source)
			return
		}
	case "plan":
		fs := flag.NewFlagSet("recovery plan", flag.ExitOnError)
		source := fs.String("source", "", "source id")
		target := fs.String("target-time", "", "RFC3339 recovery target time")
		rp := fs.String("restore-point", "", "named restore point")
		_ = fs.Parse(args[1:])
		fmt.Printf("pitr plan source=%s target_time=%s restore_point=%s status=scaffolded\n", *source, *target, *rp)
		return
	case "verify-chain":
		fs := flag.NewFlagSet("recovery verify-chain", flag.ExitOnError)
		source := fs.String("source", "", "source id")
		level := fs.String("level", "continuity", "verification level")
		_ = fs.Parse(args[1:])
		fmt.Printf("chain source=%s level=%s continuous=false status=scaffolded\n", *source, *level)
		return
	case "drill":
		fs := flag.NewFlagSet("recovery drill", flag.ExitOnError)
		source := fs.String("source", "", "source id")
		target := fs.String("target", "random", "target selection")
		_ = fs.Parse(args[1:])
		fmt.Printf("pitr drill source=%s target=%s status=scaffolded\n", *source, *target)
		return
	}
	fmt.Println("unknown recovery command")
}

func postgresRecoveryCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("postgres commands: physical-backup create, wal status|push|fetch|verify, restore-point create")
		return
	}
	switch args[0] {
	case "physical-backup":
		if len(args) >= 2 && args[1] == "create" {
			fs := flag.NewFlagSet("postgres physical-backup create", flag.ExitOnError)
			source := fs.String("source", "", "source id")
			typ := fs.String("type", "full", "full|incremental")
			_ = fs.Parse(args[2:])
			fmt.Printf("postgres physical backup source=%s type=%s status=scaffolded\n", *source, *typ)
			return
		}
	case "wal":
		if len(args) < 2 {
			fmt.Println("postgres wal commands: status, push, fetch, verify")
			return
		}
		fs := flag.NewFlagSet("postgres wal", flag.ExitOnError)
		source := fs.String("source", "", "source id")
		file := fs.String("file", "", "file path")
		name := fs.String("name", "", "native WAL name")
		target := fs.String("target", "", "target path")
		_ = fs.Parse(args[2:])
		fmt.Printf("postgres wal command=%s source=%s file=%s name=%s target=%s status=scaffolded\n", args[1], *source, *file, *name, *target)
		return
	case "restore-point":
		if len(args) >= 2 && args[1] == "create" {
			fs := flag.NewFlagSet("postgres restore-point create", flag.ExitOnError)
			source := fs.String("source", "", "source id")
			name := fs.String("name", "", "restore point name")
			_ = fs.Parse(args[2:])
			fmt.Printf("postgres restore point source=%s name=%s status=scaffolded\n", *source, *name)
			return
		}
	}
	fmt.Println("unknown postgres command")
}

func mysqlRecoveryCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("mysql commands: binlog status|verify")
		return
	}
	if args[0] == "binlog" && len(args) >= 2 {
		fs := flag.NewFlagSet("mysql binlog", flag.ExitOnError)
		source := fs.String("source", "", "source id")
		_ = fs.Parse(args[2:])
		fmt.Printf("mysql binlog command=%s source=%s status=scaffolded\n", args[1], *source)
		return
	}
	fmt.Println("unknown mysql command")
}

func domainSource(binding config.RuntimeBinding) (domain.Source, error) {
	source := binding.Source
	switch {
	case source.Engine == "sqlite" && source.SQLite != nil:
		return domain.Source{ID: domain.SourceID(source.ID), Name: source.ID, Driver: "sqlite", Enabled: source.IsEnabled(), Path: source.SQLite.Path, RepositoryID: domain.RepositoryID(source.Repository)}, nil
	case (source.Engine == "mysql" || source.Engine == "mariadb") && source.MySQL != nil:
		return domain.Source{ID: domain.SourceID(source.ID), Name: source.ID, Driver: source.Engine, Enabled: source.IsEnabled(), RepositoryID: domain.RepositoryID(source.Repository)}, nil
	default:
		return domain.Source{}, fmt.Errorf("source %s uses %s; this standalone backup command currently executes sqlite and mysql/mariadb sources", source.ID, source.Engine)
	}
}

func sourceCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("source commands: test, inspect")
		return
	}
	fs := flag.NewFlagSet("source", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	id := fs.String("id", "", "source id")
	_ = fs.Parse(args[1:])
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	binding, err := cfg.Binding(*id)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	switch args[0] {
	case "inspect":
		b, _ := json.MarshalIndent(binding.Source, "", "  ")
		fmt.Println(string(b))
	case "test":
		if err := testSource(cfg, binding.Source); err != nil {
			fmt.Println("source test failed:", err)
			os.Exit(1)
		}
		fmt.Printf("source %s connection configuration is reachable and its secret reference resolves\n", binding.Source.ID)
	default:
		fmt.Println("unknown source command")
	}
}

func testSource(cfg config.Config, source config.SourceConfig) error {
	switch source.Engine {
	case "sqlite":
		if source.SQLite == nil {
			return fmt.Errorf("sqlite configuration missing")
		}
		info, err := os.Stat(source.SQLite.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("sqlite path is not a regular file")
		}
		return nil
	case "postgres":
		if source.Postgres == nil {
			return fmt.Errorf("postgres configuration missing")
		}
		if _, err := cfg.ResolveSecret(source.Postgres.Password, false); err != nil {
			return fmt.Errorf("password: %w", err)
		}
		return dialDatabase(source.Postgres.Host, source.Postgres.Port)
	case "mysql", "mariadb":
		if source.MySQL == nil {
			return fmt.Errorf("mysql configuration missing")
		}
		if _, err := cfg.ResolveSecret(source.MySQL.Password, false); err != nil {
			return fmt.Errorf("password: %w", err)
		}
		return dialDatabase(source.MySQL.Host, source.MySQL.Port)
	default:
		return fmt.Errorf("unsupported engine %s", source.Engine)
	}
}

func dialDatabase(host string, port int) error {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), 5*time.Second)
	if err != nil {
		return err
	}
	return connection.Close()
}

func destinationCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("destination commands: test, capabilities")
		return
	}
	fs := flag.NewFlagSet("destination", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	id := fs.String("id", "", "destination id")
	_ = fs.Parse(args[1:])
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Println("config error:", err)
		os.Exit(1)
	}
	var destination config.DestinationConfig
	for _, candidate := range cfg.Destinations {
		if candidate.ID == *id || (*id == "" && destination.ID == "") {
			destination = candidate
		}
	}
	if destination.ID == "" {
		fmt.Println("destination not found")
		os.Exit(1)
	}
	switch args[0] {
	case "capabilities":
		fmt.Printf("destination=%s driver=%s profile=%s multipart=%v path_style=%v conditional_create=%s\n", destination.ID, destination.Driver, destination.Profile, destination.S3 != nil, destination.S3 != nil && destination.S3.Addressing.PathStyle, inferredConditionalCreate(destination))
	case "test":
		if err := testDestination(cfg, destination); err != nil {
			fmt.Println("destination test failed:", err)
			os.Exit(1)
		}
		fmt.Printf("destination %s configuration and endpoint are reachable\n", destination.ID)
	default:
		fmt.Println("unknown destination command")
	}
}

func inferredConditionalCreate(destination config.DestinationConfig) string {
	if destination.S3 != nil && destination.S3.CapabilityHints.ConditionalCreate != nil {
		return fmt.Sprint(*destination.S3.CapabilityHints.ConditionalCreate)
	}
	switch destination.Profile {
	case "cloudflare-r2", "minio":
		return "probe"
	case "contabo":
		return "conservative"
	default:
		return "probe"
	}
}

func testDestination(cfg config.Config, destination config.DestinationConfig) error {
	switch destination.Driver {
	case "filesystem":
		if destination.Filesystem == nil {
			return fmt.Errorf("filesystem configuration missing")
		}
		if err := os.MkdirAll(destination.Filesystem.Root, 0700); err != nil {
			return err
		}
		probe := filepath.Join(destination.Filesystem.Root, ".dbvault-configuration-probe")
		if err := os.WriteFile(probe, []byte("ok"), 0600); err != nil {
			return err
		}
		return os.Remove(probe)
	case "s3":
		if destination.S3 == nil {
			return fmt.Errorf("s3 configuration missing")
		}
		if _, err := cfg.ResolveSecret(destination.S3.Credentials.AccessKeyID, false); err != nil {
			return fmt.Errorf("access key: %w", err)
		}
		if _, err := cfg.ResolveSecret(destination.S3.Credentials.SecretAccessKey, false); err != nil {
			return fmt.Errorf("secret key: %w", err)
		}
		u, err := url.Parse(destination.S3.Endpoint)
		if err != nil {
			return err
		}
		port := u.Port()
		if port == "" {
			if strings.EqualFold(u.Scheme, "https") {
				port = "443"
			} else {
				port = "80"
			}
		}
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(u.Hostname(), port), 5*time.Second)
		if err != nil {
			return err
		}
		return connection.Close()
	default:
		return fmt.Errorf("unsupported driver %s", destination.Driver)
	}
}

func repositoryCmd(args []string) {
	if len(args) == 0 || args[0] != "explain" {
		fmt.Println("repository command: explain")
		return
	}
	fs := flag.NewFlagSet("repository explain", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	id := fs.String("id", "", "repository id")
	_ = fs.Parse(args[1:])
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if *id == "" && len(cfg.Repositories) > 0 {
		*id = cfg.Repositories[0].ID
	}
	explanation, err := cfg.ExplainRepository(*id)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	fmt.Println(explanation)
}

func serverCmd(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	cfgPath := fs.String("config", "", "config path")
	listen := fs.String("listen", "", "listen address")
	publicURL := fs.String("public-url", "", "public URL")
	dataDir := fs.String("data-dir", "/var/lib/dbvault", "data directory")
	setupToken := fs.String("setup-token", "", "setup token path")
	token := fs.String("token", os.Getenv("DBVAULT_CONTROLLER_TOKEN"), "API bearer token")
	demo := fs.Bool("demo", false, "start with safe demo data and no external credentials")
	_ = fs.Parse(args)

	// 1. Resolve DBVault Internal App Database from environment
	if *dataDir == "/var/lib/dbvault" {
		if envDir := os.Getenv("DBVAULT_DATA_DIR"); envDir != "" {
			*dataDir = envDir
		} else if envDir := os.Getenv("DATA_DIR"); envDir != "" {
			*dataDir = envDir
		}
	}
	appDbURL := os.Getenv("DBVAULT_DATABASE_URL")
	if appDbURL == "" {
		appDbURL = os.Getenv("DATABASE_URL")
	}
	if appDbURL != "" {
		if u, err := url.Parse(appDbURL); err == nil && u.User != nil {
			if pass, ok := u.User.Password(); ok && os.Getenv("PGPASSWORD") == "" {
				_ = os.Setenv("PGPASSWORD", pass)
			}
			if user := u.User.Username(); user != "" && os.Getenv("PGUSER") == "" {
				_ = os.Setenv("PGUSER", user)
			}
		}
	}

	var cfg config.Config
	allowReveal := false
	var metrics *prometheus.Metrics
	metricsPath := ""
	if *cfgPath != "" {
		loaded, err := config.Load(*cfgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "config error:", err)
			os.Exit(1)
		}
		cfg = loaded
		cfg.Normalize()
		if *listen == "" {
			*listen = cfg.Server.Listen
		}
		if *dataDir == "/var/lib/dbvault" && cfg.Server.DataDirectory != "" {
			*dataDir = cfg.Server.DataDirectory
		}
		allowReveal = cfg.Security.AllowMasterKeyReveal
		if cfg.Metrics.Enabled {
			metrics = prometheus.New()
			metricsPath = cfg.Metrics.Path
			if metricsPath == "" {
				metricsPath = "/metrics"
			}
		}
	}
	api := controllerapi.New(controlplane.NewStore(), *token).Handler()
	appliance, err := dbvserver.New(dbvserver.Config{Listen: *listen, PublicURL: *publicURL, DataDirectory: *dataDir, SetupTokenPath: *setupToken, Version: "phase8-dev", Demo: *demo, AllowMasterKeyReveal: allowReveal, MetricsPath: metricsPath, MetricsHandler: metricsHandlerOrNil(metrics)}, api, slog.Default())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if token, err := appliance.EnsureSetupToken(); err == nil && token != "" {
		fmt.Println("setup token:", token)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	stopJobs := startJobRuntime(ctx, cfg, *dataDir, !*demo, metrics)
	defer stopJobs()

	if appDbURL != "" {
		fmt.Println("DBVault App System Database (env):", appDbURL)
	} else {
		fmt.Println("DBVault App System Database (sqlite):", filepath.Join(*dataDir, "dbvault.db"))
	}
	fmt.Println("Target Backup Databases: Managed dynamically via Web UI & Discovery Probe")

	if *demo {
		fmt.Println("demo mode enabled: sample protection, alert, timeline and sandbox data are served without credentials")
	}
	fmt.Println("DBVault appliance server listening on", appliance.HTTPServer().Addr)
	if err := dbvserver.Run(ctx, appliance); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// startJobRuntime wires the durable queue, scheduler and worker pool behind
// --config when schedules resolve. Demo mode (and the no-config path) keep
// their simulated data flow untouched. When metrics is non-nil the runtime
// records job outcomes, durations, bytes backed up, queue depth and
// lease-expiry events into it.
func startJobRuntime(ctx context.Context, cfg config.Config, dataDir string, enabled bool, metrics *prometheus.Metrics) (stop func()) {
	noop := func() {}
	if !enabled {
		return noop
	}
	specs := scheduler.SpecsFromConfig(cfg)
	if len(specs) == 0 {
		return noop
	}
	logger := slog.Default()
	q, err := jobqueue.Open(filepath.Join(dataDir, "jobs.db"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "job queue open failed:", err)
		os.Exit(1)
	}
	app, err := bootstrap.Build(ctx, cfg, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "scheduler bootstrap failed:", err)
		os.Exit(1)
	}
	gcSvc := &garbagecollection.Service{Catalogue: app.Catalogue, Store: app.ObjectStore, Clock: ports.SystemClock{}}
	executor := scheduler.ExecutorFunc(func(ctx context.Context, job domain.Job) ([]byte, error) {
		if job.Type == domain.JobGarbageCollection {
			plan, err := gcSvc.Plan(ctx, domain.RepositoryID(job.ResourceID))
			if err != nil {
				return nil, err
			}
			if err := gcSvc.Run(ctx, plan); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]any{"plan_id": plan.ID, "reclaimable_bytes": plan.ReclaimableBytes, "deleted": len(plan.DeleteKeys)})
		}
		src, err := scheduledSource(app, job.ResourceID)
		if err != nil {
			return nil, err
		}
		res, err := app.Backup.Create(ctx, backup.Command{Source: src, Trigger: domain.TriggerSchedule})
		if err != nil {
			return nil, err
		}
		snapshotID := ""
		if res.Snapshot != nil {
			snapshotID = string(res.Snapshot.ID)
		}
		return json.Marshal(map[string]any{"run_id": res.Run.ID, "snapshot_id": snapshotID, "duplicate": res.Duplicate, "bytes_source": res.Run.SourceSize, "bytes_stored": res.Run.UniqueBytes})
	})

	var jobHooks scheduler.Hooks
	jobHooks.Add(gcTrigger(ctx, cfg, q).AfterJob)

	var notifier *webhook.Notifier
	if cfg.Notifications.Webhook.Enabled && cfg.Notifications.Webhook.URL.Literal != "" {
		notifier = webhook.New(cfg.Notifications.Webhook.URL.Literal, cfg.Notifications.Webhook.SigningSecret.Literal)
		jobHooks.Add(webhookHook(notifier))
	}

	sched := scheduler.New(scheduler.Options{Queue: q, Specs: specs, Tick: time.Second, Logger: logger,
		OnRecoverExpired: func(count int) {
			if metrics != nil {
				metrics.AddCounter("dbvault_lease_expired_events_total", nil, float64(count))
			}
		}})
	workers := cfg.Schedule.Workers
	if workers <= 0 {
		workers = 1
	}
	pool := scheduler.NewWorkerPool(scheduler.WorkerOptions{Queue: q, Executor: executor, Workers: workers, Logger: logger,
		Types: []domain.JobType{domain.JobBackup, domain.JobGarbageCollection},
		OnSettled: func(job domain.Job, execErr error, result []byte) {
			sched.Settle(job)
			recordJobMetrics(metrics, job, execErr, result)
			jobHooks.Run(context.Background(), logger, job, execErr)
		}})

	if metrics != nil {
		stopDepthGauge := queueDepthGauge(ctx, q, metrics)
		defer stopDepthGauge()
	}

	schedDone := make(chan struct{})
	go func() {
		defer close(schedDone)
		sched.Run(ctx)
	}()
	pool.Start(ctx)
	logger.Info("backup scheduler started", "schedules", len(specs), "workers", workers, "gc_hook", true, "webhook", notifier != nil)
	return func() {
		<-schedDone
		pool.Wait()
		_ = q.Close()
		_ = app.Close(context.Background())
	}
}

// gcTrigger builds the post-backup GC hook from retention.gc_sweep_interval.
func gcTrigger(ctx context.Context, cfg config.Config, q ports.JobQueue) *scheduler.GCTrigger {
	repositoryID := cfg.Repository.ID
	sweepRaw := cfg.Repository.Retention.GCSweepInterval
	if len(cfg.Repositories) > 0 {
		repositoryID = cfg.Repositories[0].ID
		if v := cfg.Repositories[0].Retention.GCSweepInterval; v != "" {
			sweepRaw = v
		}
	}
	interval := time.Duration(0)
	if sweepRaw != "" {
		d, err := time.ParseDuration(sweepRaw)
		switch {
		case err != nil:
			slog.Warn("invalid repository.retention.gc_sweep_interval; periodic sweep disabled", "value", sweepRaw, "error", err)
		case d > 0:
			interval = d
		}
	}
	gc := &scheduler.GCTrigger{Queue: q, Repository: repositoryID, Interval: interval}
	if interval > 0 {
		go gc.SweepLoop(ctx, slog.Default())
	}
	return gc
}

// webhookHook adapts the webhook notifier to the post-job hook signature.
// Delivery failures are logged, never fatal to job settlement.
func webhookHook(n *webhook.Notifier) scheduler.Hook {
	return func(ctx context.Context, job domain.Job, execErr error) {
		event := ports.Event{
			ID:        string(job.ID),
			Type:      string(job.Type),
			Resource:  job.ResourceID,
			CreatedAt: time.Now().UTC(),
			Metadata:  map[string]string{"status": "failed"},
		}
		if execErr == nil {
			event.Metadata["status"] = "succeeded"
			if job.Type != domain.JobBackup {
				return // notify only backup outcomes for now; GC noise stays local
			}
		}
		event.Severity = "info"
		if execErr != nil {
			event.Severity = "warning"
			event.Metadata["error"] = execErr.Error()
		}
		if err := n.Notify(ctx, event); err != nil {
			slog.Warn("webhook notification not delivered", "job_id", job.ID, "error", err)
		}
	}
}

func recordJobMetrics(m *prometheus.Metrics, job domain.Job, execErr error, result []byte) {
	if m == nil {
		return
	}
	status := "succeeded"
	if execErr != nil {
		status = "failed"
	}
	m.IncCounter("dbvault_jobs_total", map[string]string{"type": string(job.Type), "status": status})
	if execErr != nil {
		return
	}
	var payload struct {
		BytesSource int64 `json:"bytes_source"`
		BytesStored int64 `json:"bytes_stored"`
	}
	_ = json.Unmarshal(result, &payload)
	m.AddCounter("dbvault_bytes_backed_up_total", map[string]string{"source": job.ResourceID}, float64(payload.BytesSource))
	m.ObserveDuration("dbvault_job_duration_seconds", time.Since(job.CreatedAt), map[string]string{"type": string(job.Type)})
}

// queueDepthGauge refreshes the queue-depth gauge every 15s until ctx ends.
func queueDepthGauge(ctx context.Context, q ports.JobQueue, m *prometheus.Metrics) (stop func()) {
	dr, ok := q.(ports.QueueDepthReporter)
	if !ok || m == nil {
		return func() {}
	}
	done := make(chan struct{})
	ticker := time.NewTicker(15 * time.Second)
	go func() {
		defer close(done)
		refresh := func() {
			if d, err := dr.Depth(ctx); err == nil {
				m.SetGauge("dbvault_queue_depth", float64(d), nil)
			}
		}
		refresh()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			}
		}
	}()
	return func() { ticker.Stop(); <-done }
}

func metricsHandlerOrNil(m *prometheus.Metrics) http.Handler {
	if m == nil {
		return nil
	}
	return m.Handler()
}

// scheduledSource maps a scheduled job's resource ID onto the configured
// source. SQLite sources are executable anywhere; MySQL/MariaDB sources must
// be the source this runtime bound at bootstrap (its dump driver carries that
// source's connection and secret references). Anything else fails the job
// with a clear reason instead of a driver-layer surprise.
func scheduledSource(app *bootstrap.Application, resourceID string) (domain.Source, error) {
	sc, ok := config.SourceByID(app.Config, resourceID)
	if !ok {
		return domain.Source{}, fmt.Errorf("source %q is not configured for scheduled backups", resourceID)
	}
	switch {
	case sc.SQLite != nil:
		return domain.Source{ID: domain.SourceID(sc.ID), Name: sc.ID, Driver: "sqlite", Enabled: sc.IsEnabled(), Path: sc.SQLite.Path, RepositoryID: domain.RepositoryID(sc.Repository)}, nil
	case (sc.Engine == "mysql" || sc.Engine == "mariadb") && sc.MySQL != nil:
		if sc.ID != app.Binding.Source.ID {
			return domain.Source{}, fmt.Errorf("source %q uses engine %q but this runtime bound source %q; run one appliance instance per database source", sc.ID, sc.Engine, app.Binding.Source.ID)
		}
		return domain.Source{ID: domain.SourceID(sc.ID), Name: sc.ID, Driver: sc.Engine, Enabled: sc.IsEnabled(), RepositoryID: domain.RepositoryID(sc.Repository)}, nil
	default:
		return domain.Source{}, fmt.Errorf("source %q uses engine %q; scheduled execution currently supports sqlite and mysql/mariadb sources", sc.ID, sc.Engine)
	}
}

func installCmd(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	root := fs.String("root", "/", "installation root")
	domain := fs.String("domain", "", "public domain or URL")
	selfSigned := fs.Bool("self-signed", false, "use self-signed TLS mode")
	offline := fs.String("offline-bundle", "", "offline release bundle")
	dryRun := fs.Bool("dry-run", false, "print plan without writing")
	force := fs.Bool("force", false, "overwrite compatible generated files")
	_ = fs.Parse(args)
	res, err := installer.Apply(installer.Options{Root: *root, Domain: *domain, SelfSigned: *selfSigned, OfflineBundle: *offline, DryRun: *dryRun, Force: *force})
	if err != nil {
		fmt.Fprintln(os.Stderr, "install failed:", err)
		os.Exit(1)
	}
	for _, step := range res.Steps {
		fmt.Printf("%s\t%s\n", step.Status, step.Name)
	}
	if *dryRun {
		fmt.Println("dry-run complete; no files written")
		return
	}
	fmt.Println("setup URL:", res.SetupURL)
	fmt.Println("setup token:", res.SetupToken)
	fmt.Println("config:", res.ConfigPath)
	fmt.Println("systemd unit:", res.UnitPath)
}

func upgradeCmd(args []string) {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	check := fs.Bool("check", false, "check only")
	rollback := fs.Bool("rollback", false, "rollback to previous version")
	target := fs.String("target", "latest", "target version")
	_ = fs.Parse(args)
	if *rollback {
		fmt.Println("upgrade rollback plan: stop service, restore previous binary, restore compatible config, restart service")
		return
	}
	plan := installer.PlanUpgrade("current", *target)
	if *check {
		fmt.Println("upgrade available check scaffold; target", *target)
	}
	for _, step := range plan.Steps {
		fmt.Println(step)
	}
}

func uninstallCmd(args []string) {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	purge := fs.Bool("purge-data", false, "remove /var/lib/dbvault and /etc/dbvault")
	_ = fs.Parse(args)
	plan := installer.PlanUninstall(*purge)
	for _, step := range plan.Steps {
		fmt.Println(step)
	}
	if !*purge {
		fmt.Println("data and configuration are preserved by default")
	}
}

func agentCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("agent commands: install, enrol, status")
		return
	}
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	controller := fs.String("controller", "", "controller URL")
	token := fs.String("token", "", "one-time enrolment token")
	_ = fs.Parse(args[1:])
	switch args[0] {
	case "install":
		fmt.Printf("agent install plan controller=%s token_present=%v stages=create-user,install-binary,write-service,enrol,start,verify\n", *controller, *token != "")
	case "enrol":
		fmt.Printf("agent enrol controller=%s token_present=%v status=scaffolded\n", *controller, *token != "")
	case "status":
		fmt.Println("agent status: local appliance agent scaffold ready")
	default:
		fmt.Println("unknown agent command")
	}
}

func protectionCmd(args []string) {
	if len(args) == 0 || args[0] != "status" {
		fmt.Println("protection command: status --source <id> --data-dir <dir>")
		return
	}
	fs := flag.NewFlagSet("protection status", flag.ExitOnError)
	source := fs.String("source", "production-postgres", "source id")
	dataDir := fs.String("data-dir", filepath.Join(os.TempDir(), "dbvault-product-experience"), "data dir")
	output := fs.String("output", "text", "text|json")
	_ = fs.Parse(args[1:])
	svc, err := productexperience.New(filepath.Join(*dataDir, "product-experience"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	status := svc.ProtectionSummary(context.Background(), domain.SourceID(*source))
	if *output == "json" {
		_ = json.NewEncoder(os.Stdout).Encode(status)
		return
	}
	fmt.Printf("source=%s status=%s score=%d summary=%s\n", status.SourceID, status.Status, status.Score, status.Summary)
	for _, action := range status.SuggestedActions {
		fmt.Printf("action=%s safe=%v\n", action.Label, action.Safe)
	}
}

func bundleCmd(args []string) {
	if len(args) < 2 || args[1] != "create" {
		fmt.Println("bundle commands: support create --data-dir <dir>, recovery create --data-dir <dir>")
		return
	}
	fs := flag.NewFlagSet("bundle", flag.ExitOnError)
	dataDir := fs.String("data-dir", filepath.Join(os.TempDir(), "dbvault-product-experience"), "data dir")
	_ = fs.Parse(args[2:])
	svc, err := productexperience.New(filepath.Join(*dataDir, "product-experience"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var path string
	switch args[0] {
	case "support":
		path, err = svc.CreateSupportBundle(context.Background())
	case "recovery":
		path, err = svc.CreateRecoveryBundle(context.Background())
	default:
		fmt.Println("unknown bundle type")
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(path)
}

func probeCmd(args []string) {
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	engine := fs.String("engine", "postgres", "database engine (postgres, mysql, sqlite)")
	host := fs.String("host", "127.0.0.1", "database host")
	port := fs.Int("port", 0, "database port (defaults: 5432 for postgres, 3306 for mysql)")
	username := fs.String("user", "", "database username")
	password := fs.String("password", "", "database password")
	path := fs.String("path", "", "sqlite database file or directory path")
	uri := fs.String("uri", "", "database connection URI (e.g. postgres://user:pass@host:5432/dbname)")
	jsonOut := fs.Bool("json", false, "output result as JSON")
	_ = fs.Parse(args)

	if *uri != "" {
		if u, err := url.Parse(*uri); err == nil {
			if u.Hostname() != "" {
				*host = u.Hostname()
			}
			if u.Port() != "" {
				if p, err := strconv.Atoi(u.Port()); err == nil {
					*port = p
				}
			}
			if u.User != nil {
				*username = u.User.Username()
				if pass, ok := u.User.Password(); ok {
					*password = pass
				}
			}
			if strings.HasPrefix(u.Scheme, "post") {
				*engine = "postgres"
			} else if strings.HasPrefix(u.Scheme, "my") {
				*engine = "mysql"
			} else if strings.HasPrefix(u.Scheme, "sqlite") {
				*engine = "sqlite"
				*path = u.Path
			}
		}
	}

	px, err := productexperience.New(os.TempDir())
	if err != nil {
		fmt.Println("error initializing probe engine:", err)
		os.Exit(1)
	}

	res, err := px.ProbeDatabaseEngine(context.Background(), productexperience.EngineProbeRequest{
		Engine:   *engine,
		Host:     *host,
		Port:     *port,
		Username: *username,
		Password: *password,
		Path:     *path,
	})
	if err != nil {
		fmt.Println("probe error:", err)
		os.Exit(1)
	}

	if *jsonOut {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Printf("=== DBVault Engine Probe (%s) ===\n", strings.ToUpper(res.Engine))
	fmt.Printf("Status:        %s\n", res.Status)
	fmt.Printf("Engine Info:   %s\n", res.ServerVersion)
	fmt.Printf("Latency:       %dms\n", res.LatencyMs)
	fmt.Printf("Total DBs:     %d\n", res.TotalDatabases)
	fmt.Printf("Total Size:    %d bytes\n\n", res.TotalSizeBytes)
	if len(res.Databases) > 0 {
		fmt.Println("Discovered Databases:")
		for i, db := range res.Databases {
			fmt.Printf("  [%d] %s (Size: %d bytes, Tables: %d, Encoding: %s)\n", i+1, db.Name, db.SizeBytes, db.TableCount, db.Encoding)
			if db.Path != "" {
				fmt.Printf("      Path: %s\n", db.Path)
			}
		}
	}
}
