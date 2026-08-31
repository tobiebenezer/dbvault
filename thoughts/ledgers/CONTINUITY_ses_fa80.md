---
session: ses_fa80
updated: 2026-08-31T13:48:12.105Z
---

Now the schedule validation (operation whitelist + cron-or-every_seconds):[Assistant tool call]: edit({"filePath":"/home/sifu/Documents/dbvault/internal/config/types.go","newString":"\tfor _, s := range c.Schedules {\n\t\tif s.ID == \"\" {\n\t\t\terrs = append(errs, errors.New(\"schedule id is required\"))\n\t\t}\n\t\tif _, ok := sourceIDs[s.Source]; !ok {\n\t\t\terrs = append(errs, fmt.Errorf(\"schedule %s references unknown source %q\", s.ID, s.Source))\n\t\t}\n\t\tswitch s.Operation {\n\t\tcase \"\", \"logical_backup\", \"warehouse_sync\":\n\t\tdefault:\n\t\t\terrs = append(errs, fmt.Errorf(\"schedule %s has unsupported operation %q\", s.ID, s.Operation))\n\t\t}\n\t\tif s.Cron == \"\" && s.EverySeconds <= 0 {\n\t\t\terrs = append(errs, fmt.Errorf(\"schedule %s cron or every_seconds is required\", s.ID))\n\t\t}\n\t}","oldString":"\tfor _, s := range c.Schedules {\n\t\tif s.ID == \"\" {\n\t\t\terrs = append(errs, errors.New(\"schedule id is required\"))\n\t\t}\n\t\tif _, ok := sourceIDs[s.Source]; !ok {\n\t\t\terrs = append(errs, fmt.Errorf(\"schedule %s references unknown source %q\", s.ID, s.Source))\n\t\t}\n\t\tif s.Cron == \"\" {\n\t\t\terrs = append(errs, fmt.Errorf(\"schedule %s cron is required\", s.ID))\n\t\t}\n\t}"})
[Tool result]: Edit applied successfully.
<directory-context>
<context file="./README.md">
# DBVault

**Know that you can recover.**

DBVault is a self-contained Go database backup and verified-recovery appliance. It is designed to install as one binary, serve its own web console, connect databases to user-owned storage, run backups, run restore drills, and show whether a database is actually protected.

> **Alpha status:** DBVault is currently a v0.1-alpha foundation. It is suitable for demos, local development, staging tests, and controlled pilots. Do not use it as the only backup system for irreplaceable production data yet.

---

## What DBVault is trying to become

```text
One Go binary
    ↓
Embedded web UI
    ↓
Backup and restore engine
    ↓
Clear protection status
```

The product goal is not merely to create backups. The goal is to prove recovery.

---

## Fast demo

```bash
make web-build
CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault ./cmd/dbvault
bin/dbvault server --demo --listen 127.0.0.1:8080 --data-dir /tmp/dbvault-demo
```

Open:

```text
http://127.0.0.1:8080
```

---

## Docker demo

```bash
docker compose -f deploy/compose/demo.yml up --build
```

Open:

```text
http://127.0.0.1:8080
```

Docker is optional. Native systemd remains the preferred path for early real VPS pilots because DBVault may need local filesystem, socket and native database-tool access.

---

## Alpha smoke test

```bash
make alpha-smoke
```

This verifies the install layout, embedded UI, overview API, protection API, recovery timeline, doctor, policy simulation, support bundle and recovery bundle routes.

For the broader alpha check:

```bash
make alpha-check
```

---

## Install-layout test

This does not touch the host system:

```bash
bin/dbvault install \
  --root /tmp/dbvault-install \
  --domain backup.example.test \
  --self-signed
```

Inspect:

```text
[truncated]
```
[Assistant]: Now main.go — pass px into startJobRuntime, wire SetJobQueue, add executor case and worker Types:
[Assistant tool call]: read({"filePath":"/home/sifu/Documents/dbvault/cmd/dbvault/main.go","limit":75,"offset":934})
[Tool result]: <path>/home/sifu/Documents/dbvault/cmd/dbvault/main.go</path>
<type>file</type>
<content>
934: 	q, err := jobqueue.Open(filepath.Join(dataDir, "jobs.db"))
935: 	if err != nil {
936: 		fmt.Fprintln(os.Stderr, "job queue open failed:", err)
937: 		os.Exit(1)
938: 	}
939: 	app, err := bootstrap.Build(ctx, cfg, logger)
940: 	if err != nil {
941: 		fmt.Fprintln(os.Stderr, "scheduler bootstrap:", err)
942: 		os.Exit(1)
943: 	}
944: 	maybeStartReplication(ctx, app, cfg, logger)
945: 	defer func() { _ = app.Close() }()
946: 	specs := scheduler.SpecsFromConfig(cfg)
947: 	sched := scheduler.New(scheduler.Options{Queue: q, Specs: specs, Tick: 15 * time.Second, Logger: logger, OnFire: func(job domain.Job) {
948: 		_ = prom.RecordJobEvent("enqueued", job.Type.String(), job.ResourceID, "scheduler")
949: 	}})
950: 	pool := scheduler.NewWorkerPool(scheduler.WorkerOptions{Queue: q, Executor: executor, Workers: workers, Logger: logger,
951: 		Types: []domain.JobType{domain.JobBackup, domain.JobGarbageCollection},
952: 		OnSettled: func(job domain.Job, execErr error, result []byte) {
953: 			prom.RecordJobOutcome(job.Type.String(), execErr == nil)
954: 			prom.ObserveJobDuration(job.Type.String(), execErr == nil, time.Since(startedAt))
955: 		},
956: 	})
957: 	executor := scheduler.ExecutorFunc(func(ctx context.Context, job domain.Job) ([]byte, error) {
958: 		if job.Type == domain.JobGarbageCollection {
959: 			return runGarbageCollectionJob(ctx, app, cfg, logger)
960: 		}
961: 		if job.Type == domain.JobRepositoryScan {
962: 			return runRepositoryScanJob(ctx, app, cfg, logger)
963: 		}
964: 		if job.Type == domain.JobRepositoryRepair {
965: 			return runRepositoryRepairJob(ctx, app, cfg, logger)
966: 		}
967: 		if job.Type == domain.JobRetention {
968: 			return runRetentionJob(ctx, app, cfg, logger)
969: 		}
970: 		if job.Type == domain.JobVerify {
971:			return runVerifyJob(ctx, app, cfg, logger)
972: 		}
973: 		if job.Type == domain.JobReplication {
974: 			return runReplicationJob(ctx, app, cfg, logger)
975: 		}
976: 		return runBackupJob(ctx, app, cfg, logger, job)
977: 	})
978: 	pool.Start(ctx)
979: 	sched.Start(ctx)
980: 	queueDepthGauge(ctx, q, metrics)
981: 	...
[Tool result]: The executor block has more job types than the earlier grep showed (repository scan/repair, retention, verify, replication). Types list at 951 is only backup+GC though — interesting, means other types leased? No — Types filters which jobs workers lease. So verify/replication/retention jobs are enqueued but... maybe other pools elsewhere. Not my concern.

Wait — line 950-956: pool created BEFORE executor defined (line 957)? The pool captures executor in options — but executor is defined after pool. That would be a compile error unless... `Executor: executor` at line 950 references executor declared later? No — Go doesn't allow use-before-declare in same scope for variables. Unless executor is declared elsewhere too. Let me re-read carefully — maybe the read output is showing lines out of order or there are two executors. Actually the tool result might be reconstructing from diff context. Let me grep precisely.
