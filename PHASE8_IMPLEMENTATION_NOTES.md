# Phase 8 Implementation Notes

Phase 8 adds the self-contained Go appliance layer requested by the product spec.

Implemented:

- `dbvault server` serves the embedded UI, API mount, agent gateway routes, setup routes, health, readiness, SSE and installer endpoints.
- `dbvault install` writes the default self-contained layout, systemd unit, config and setup token. Use `--root` for tests or offline image assembly.
- `dbvault upgrade` and `dbvault uninstall` expose safe operation plans.
- `dbvault agent install|enrol|status` is available from the same binary.
- Embedded UI assets live under `internal/server/web/dist` and are compiled into the Go binary.
- `internal/install` generates installation plans, systemd units and first-run setup tokens.
- `internal/provisioning` models Forge-like installation stages and redacts deployment logs.
- `internal/secrets/local` provides a local encrypted secret-store foundation.
- `internal/release` verifies release manifests and artifact checksums.

Verified in this environment:

```bash
CGO_ENABLED=0 go test -tags=restricted ./...
CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd ./cmd/dbvault-controller ./cmd/dbvault-agent ./cmd/dbvault-operator
make test-phase8
```

Production still requires a network-enabled Go environment for module download and CGO checks.
