# DBVault Phase 8 Implementation Report

Phase 8 implements the self-contained Go appliance architecture.

## Added packages

- `internal/server`: embedded UI, API mount, agent routes, installer routes, setup token, health/ready, SSE and security headers.
- `internal/install`: install layout, systemd unit, setup token, upgrade and uninstall plans.
- `internal/provisioning`: Forge-like deployment job stages and secret-redacting logs.
- `internal/release`: signed release-manifest and checksum verification.
- `internal/secrets/local`: encrypted local secret store for self-contained deployments.

## Added commands

- `dbvault server`
- `dbvault install`
- `dbvault upgrade`
- `dbvault uninstall`
- `dbvault agent install|enrol|status`

## Added assets and deployment files

- Embedded UI under `internal/server/web/dist`.
- systemd service at `deploy/systemd/dbvault.service`.
- Phase 8 example config at `configs/dbvault.phase8.example.yaml`.
- Phase 8 docs at `docs/phase8/self-contained-appliance.md`.
- Release-bundle script at `scripts/release-bundle.sh`.

## Verified here

```bash
make test-phase8
go vet -tags=restricted ./...
CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd ./cmd/dbvault-controller ./cmd/dbvault-agent ./cmd/dbvault-operator
```

## Not verified here

Production dependency-backed builds still require `go mod download` in an internet-enabled Go environment.
