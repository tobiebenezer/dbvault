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
Agent and control-plane foundations
    ↓
User-owned storage such as R2, Contabo, S3 or MinIO
    ↓
Restore drills
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
/tmp/dbvault-install/etc/dbvault/dbvault.yaml
/tmp/dbvault-install/etc/systemd/system/dbvault.service
/tmp/dbvault-install/var/lib/dbvault/setup-token
```

---

## Current install paths

| Path | Status | Use for |
|---|---:|---|
| `dbvault server --demo` | Works | Product demo and UI exploration |
| `dbvault install --root ...` | Works | Safe install-layout testing |
| Docker Compose demo | Works when Docker is available | Contributors and demos |
| Native systemd install | Scaffolded | Early controlled pilots after review |
| Kubernetes | Scaffolded | Future platform work |

---

## Current product state

Implemented foundations include:

- Core SQLite backup/restore components
- Repository, chunking, encryption and manifest foundations
- Multi-engine PostgreSQL/MySQL/MariaDB seams
- PITR and log-collection domain scaffolding
- Tenant/control-plane/agent scaffolding
- Kubernetes CRD/operator scaffolding
- Self-contained Go appliance server
- Embedded source-backed web console
- Setup token flow
- Product-experience APIs
- Protection-status scaffold
- Recovery timeline scaffold
- Preflight doctor scaffold
- Policy simulation scaffold
- Alerts scaffold
- Sandbox restore lifecycle scaffold
- Support and recovery bundle scaffolds
- Docker demo packaging
- Alpha runbook and smoke tests

---

## What is not production-ready yet

The following must be completed before DBVault can protect real customer data as a primary backup system:

- Production dependency download and `go.sum` generation
- CGO SQLite production validation
- Real Cloudflare R2 contract tests
- Real Contabo contract tests
- Real PostgreSQL dump/restore tests
- Real MySQL/MariaDB dump/restore tests
- Real restore drills connected to catalogue evidence
- Protection status calculated from real backup/restore evidence
- Recovery timeline calculated from real snapshot and WAL/binlog history
- Browser automation with Playwright or equivalent
- Accessibility automation
- Installer and agent-enrolment security review
- Signed releases, SBOM and vulnerability scan

---

## Useful commands

```bash
make web-test
make web-build
make test-restricted
make build-restricted
make test-phase8c
make alpha-smoke
make alpha-package
```

Production profile, in an internet-enabled environment:

```bash
./scripts/bootstrap-production-deps.sh
make build
make test-race
```

---

## Documentation

Start here:

- `docs/alpha/ALPHA_0_1_RUNBOOK.md`
- `docs/alpha/ALPHA_0_1_CHECKLIST.md`
- `docs/alpha/INSTALL.md`
- `docs/alpha/DOCKER.md`
- `docs/phase8/self-contained-appliance.md`
- `docs/product/outstanding-product-experience.md`
- `docs/configuration/connection-wiring.md`

---

## Configuration model

The runtime graph is explicit:

```text
source.repository
  -> repository.primary / mirrors / replicas
  -> destination
  -> secret provider references
```

Useful commands:

```bash
dbvault config validate --config /etc/dbvault/dbvault.yaml
dbvault config print-effective --config /etc/dbvault/dbvault.yaml
dbvault source test --config /etc/dbvault/dbvault.yaml --id application-postgres
dbvault destination test --config /etc/dbvault/dbvault.yaml --id r2-primary
dbvault repository explain --config /etc/dbvault/dbvault.yaml --id production
```

---

## Product promise

Backups are promises. DBVault tests the promise.

One Go binary. Your database. Your storage. Verified restores.

## Phase 8I UI cleanup

The embedded console now uses a single all-white surface system with compact navigation, restrained typography, simple bordered rows, outlined status chips, and emerald reserved for primary actions and healthy state. Phase 8I preserves the connected Phase 8G actions and Phase 8D live job/SSE wiring while removing residual prototype styling.
