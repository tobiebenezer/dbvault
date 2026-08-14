# DBVault Phase 8 Production Test Checklist

Run in a CGO, Docker and internet-enabled build environment.

## Dependencies

```bash
go mod download
go mod verify
```

This environment could not run these because `go.sum` entries for production
modules were unavailable locally and network access to module proxies is blocked.

## Appliance build

```bash
make web-build
CGO_ENABLED=1 go build -o bin/dbvault ./cmd/dbvault
```

## Appliance tests

```bash
CGO_ENABLED=1 go test ./internal/server ./internal/install ./internal/provisioning ./internal/release ./internal/secrets/local
CGO_ENABLED=1 go test ./cmd/dbvault
```

## Full product tests

```bash
make test
make test-race
make test-phase8
make test-integration
```

## Manual deployment smoke test

```bash
sudo ./bin/dbvault install --domain backup.example.com --self-signed
sudo systemctl start dbvault
curl -k https://backup.example.com/health
curl -k https://backup.example.com/install/agent.sh
```

## Local non-root install-layout test

```bash
./bin/dbvault install --root /tmp/dbvault-install --domain backup.example.test
find /tmp/dbvault-install/etc/dbvault /tmp/dbvault-install/var/lib/dbvault -maxdepth 3 -type f -print
```

## Acceptance checks

- The UI loads from the Go binary.
- `/api/v1/status` returns JSON.
- `/events/jobs` returns an SSE stream.
- `/install/agent.sh` returns a one-line remote agent installer.
- Setup token is one-time use.
- systemd unit starts `dbvault server`.
- No Node.js, Redis, Nginx or Caddy process is required.
- `dbvault upgrade --check` produces a signed-release upgrade plan.
- `dbvault uninstall` preserves data by default.
