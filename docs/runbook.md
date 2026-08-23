# DBVault Operations Runbook

Audience: the operator running a DBVault appliance in production (systemd host
or Docker Compose). Every command below assumes the binary is on `PATH` as
`dbvault` and the config lives at `/etc/dbvault/dbvault.yaml` (adjust paths for
Compose, where the example config is mounted from
`deploy/compose/prod.config.yaml`).

Related assets:

| Asset | Path |
|---|---|
| Production compose profile | `deploy/compose/prod.yml` + `prod.config.yaml` |
| systemd service | `deploy/systemd/dbvault.service` |
| Metadata self-backup units | `deploy/systemd/dbvault-metadata-backup.{service,timer}` |
| Doctor checks | `internal/application/doctor/` |

---

## 1. First-boot setup (setup token -> admin credential)

Console/API auth is always on. There is no unauthenticated mode.

1. Start the appliance (see §3). On first boot the server prints:
   ```
   setup token: <one-time token>
   ```
   and writes the same token to `<data_directory>/setup-token` with mode
   `0600`. In Compose, read it with `docker compose -f deploy/compose/prod.yml
   logs dbvault`.
2. Open the console (`http://host:8080`, or your TLS domain) and present the
   token when prompted. Completing setup creates the admin credential and
   **deletes the token file** — the token cannot be replayed afterwards.
3. Record the admin credential in your password manager. If the token file is
   lost before completion, stop the server and delete
   `<data_directory>/setup-token`; the next start mints a fresh one.
4. Verify replay protection: re-running the setup flow after completion must
   fail (HTTP 403). Report any success as a security incident.

## 2. Key ceremony

The vault master key is exactly **32 bytes of entropy** (shorter keys are
rejected by derivation; there is no silent zero-padding anymore). AEAD,
signing and dedup subkeys are derived in memory via HKDF-SHA256 with domain
labels `dbvault/aead-v1`, `dbvault/hmac-v1`, `dbvault/dedup-v1`.

1. Generate on an air-gapped or trusted machine:
   ```bash
   dbvault key generate --out /var/lib/dbvault/keys/master
   # honours DBVAULT_KEY_FILE; DBVAULT_MASTER_KEY supplies/overrides via env
   ```
2. The file must be owned by `dbvault:dbvault` with mode `0600`. It is read by
   the file secret provider configured under `secret_providers:` (root
   `/var/lib/dbvault/keys`).
3. **Disaster-recovery sheet:** print or hand-copy the 64-char hex key and
   store it in two geographically separated safes. The key *is* your data —
   lose it and every snapshot is ciphertext forever; leak it and restore the
   key immediately by rotating to a new repository.
4. Never place the master key inside the same storage bucket as the
   repository it encrypts.
5. Legacy repositories created before HKDF derivation emit a loud warning
   ("legacy single-duty/zero-padded key material"). They remain readable, but
   plan a migration to a new repository with a fresh key (re-key tooling is an
   open roadmap item — track it explicitly).

## 3. Starting: systemd vs Compose

### systemd (preferred for VPS pilots)

```bash
sudo cp deploy/systemd/dbvault.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now dbvault
journalctl -u dbvault -f          # capture the setup token on first boot
```

The unit runs `dbvault server --config /etc/dbvault/dbvault.yaml` as the
dedicated `dbvault` user with a hardened sandbox (`ProtectSystem=strict`,
no new privileges, only `/var/lib/dbvault`, `/var/log/dbvault` and
`/etc/dbvault/tls` writable).

### Compose

```bash
cd deploy/compose
# edit prod.config.yaml first (destinations, sources, schedules)
docker compose -f prod.yml up -d --build
docker compose -f prod.yml logs dbvault    # setup token
```

Named volumes `dbvault-data` (catalogue, keys, jobs.db) and
`dbvault-scratch` (chunk staging) persist across rebuilds;
`restart: unless-stopped` keeps it up. Healthcheck runs
`dbvault doctor --config /etc/dbvault/dbvault.yaml` every 30s — an unhealthy
config marks the container unhealthy instead of silently serving.

### Reverse proxy (automatic TLS)

```bash
PROXY_DOMAIN=backup.example.com docker compose -f prod.yml --profile proxy up -d
```

Caddy obtains/renews certificates for `$PROXY_DOMAIN` and proxies to the
appliance. ACL `/metrics` at this layer to scrape networks only; do not bind
port 8080 publicly without a TLS terminator.

## 4. Verifying backups

```bash
# Truthful preflight: config load, catalogue SQLite open+migrate, destination
# reachability, scratch free-space watermark. Exit non-zero names the failure.
dbvault doctor --config /etc/dbvault/dbvault.yaml

# Snapshot inventory
dbvault backup-list --config /etc/dbvault/dbvault.yaml

# Job status (authenticated console): Jobs view shows queued/leased/done rows,
# or hit the API:
curl -sH "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/v1/jobs

# Metrics (unauthenticated by design; ACL at proxy)
curl -s http://127.0.0.1:8080/metrics | grep '^dbvault_'
```

Metric families:

| Metric | Meaning |
|---|---|
| `dbvault_jobs_total{type,status}` | Settled jobs by type and terminal status |
| `dbvault_bytes_backed_up_total{source}` | Source bytes captured by successful backups |
| `dbvault_job_duration_seconds` | Job execution duration histogram |
| `dbvault_queue_depth` | Live (non-terminal) jobs currently queued |
| `dbvault_lease_expired_events_total` | Leases recovered after worker loss |
| `dbvault_notification_delivery_failures_total` | Webhooks that exhausted retries |

Healthy steady state: nightly schedule produces one completed backup job per
source per day, queue depth returns to 0 between runs,
`lease_expired_events_total` stays flat.

## 5. Restore drill (run weekly — backups are promises until restored)

Automate cadence with `verification.restore_drills` or run manually:

```bash
# 1. Pick the newest snapshot
SNAP=$(dbvault backup-list --config /etc/dbvault/dbvault.yaml | head -1)

# 2. Plan the restore into a scratch target (never over the live DB)
dbvault restore-plan --config /etc/dbvault/dbvault.yaml \
  --snapshot "$SNAP" --target /var/lib/dbvault/drills/restore.sqlite

# 3. Run it (--replace only when intentionally overwriting the drill target)
dbvault restore-run --config /etc/dbvault/dbvault.yaml \
  --snapshot "$SNAP" --target /var/lib/dbvault/drills/restore.sqlite --replace

# 4. Prove the data: open the restored file and check row counts / marker rows
sqlite3 /var/lib/dbvault/drills/restore.sqlite 'PRAGMA integrity_check;'

# 5. Tamper check (quarterly): flip one byte inside a chunk copy and confirm
#    restore fails loudly — manifest signature verification rejects it.
```

Success criteria: restore-run exits 0, integrity_check returns `ok`, and the
drill completes within your RTO. Delete drill artifacts afterwards so they do
not consume scratch space.

## 6. Alert triage

Notifications arrive as HMAC-signed JSON POSTs:

- Header: `X-DBVault-Signature: sha256=<hex hmac>` computed with
  `notifications.webhook.signing_secret` over the raw body. Reject mismatches.
- Body fields:

| Field | Description |
|---|---|
| `id` | Unique event ID |
| `event` | Event type (job success/failure, GC result, …) |
| `severity` | Severity label for routing |
| `resource` | Subject of the event (job/source/repository ID) |
| `metadata` | Structured details: job id/type/status/duration/error, reclaimed bytes for GC |
| `created_at` | RFC3339Nano timestamp — use as anti-replay hint alongside `id` dedup |

Delivery retries with exponential backoff (3 attempts); exhausted deliveries
never fail the underlying pipeline — they increment
`dbvault_notification_delivery_failures_total` and log a warning. Triage:

1. Backup job failure event → `journalctl -u dbvault` (or container logs),
   find job ID, inspect error in metadata; check source reachability and
   scratch free space (`doctor`).
2. Repeated `lease_expired_events_total` growth → workers are dying mid-job
   (OOM, restart loop); lease recovery is automatic but investigate cause.
3. Notification failures with healthy jobs → webhook endpoint or network
   problem; alerts themselves are degraded, backups are not.

## 7. Metadata self-backup

The catalogue SQLite database (`/var/lib/dbvault/catalogue/dbvault.sqlite`) is
the index of all snapshots — losing it loses the map to your vault. A daily
systemd timer backs it up **into its own vault** through the normal hardened
pipeline (chunked, encrypted, signed):

```bash
sudo cp deploy/systemd/dbvault-metadata-backup.{service,timer} /etc/systemd/system/
# create the companion config shown in the .service header comment
sudo systemctl enable --now dbvault-metadata-backup.timer
systemctl list-timers dbvault-metadata-backup.timer   # verify next fire
```

Mechanics: the timer fires daily (+15 min jitter, `Persistent=true` catches up
missed runs) and runs `dbvault backup-create --config
/etc/dbvault/metadata-backup.yaml`, whose single enabled sqlite source points
at the catalogue file. The sqlite driver copies it via SQLite's online backup
API through a read-only connection — safe while the appliance keeps running.
Rotation is not handled by the timer: old metadata snapshots expire through
the repository's own retention policy and scheduled garbage collection.

## 8. Upgrade procedure

1. **Read the release notes** for breaking config-schema changes.
2. Back up first: trigger the metadata self-backup
   (`sudo systemctl start dbvault-metadata-backup.service`) and confirm a new
   snapshot appears in `backup-list`.
3. Preflight the new binary against the current config:
   ```bash
   dbvault-new doctor --config /etc/dbvault/dbvault.yaml
   dbvault-new config validate --config /etc/dbvault/dbvault.yaml
   ```
4. systemd: replace `/usr/local/bin/dbvault`, then
   `sudo systemctl restart dbvault`. Compose: `docker compose -f prod.yml up
   -d --build` (volumes persist).
5. Watch startup logs for the absence of a new setup token (existing install
   reuses the completed-setup state) and confirm health:
   ```bash
   dbvault doctor --config /etc/dbvault/dbvault.yaml
   curl -fsS http://127.0.0.1:8080/health
   curl -fsS http://127.0.0.1:8080/metrics > /dev/null
   ```
6. Rollback: keep the previous binary/image tag; stop, swap back, restart.
   Catalogue migrations may be forward-only — restoring the pre-upgrade
   metadata snapshot (§7) plus the old binary is the full fallback.
