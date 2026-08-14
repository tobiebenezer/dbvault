# Production test checklist

The Phase 7 restricted profile has been tested in this environment. The production dependency download could not run because outbound DNS/network access to `proxy.golang.org` is blocked.

Run these commands in a network-enabled Linux environment with GCC, SQLite development headers, Docker and a Kubernetes test cluster:

```sh
go mod download
go mod verify
CGO_ENABLED=1 go test ./...
CGO_ENABLED=1 go test -race ./...
make build
make build-platform
make test-integration
make test-postgres
make test-mysql
make test-mariadb
make test-phase5
```

Phase 7 platform checks:

```sh
make test-tenant-isolation
make test-agent-protocol
make test-operator
make test-plugins
CGO_ENABLED=1 go test -tags=integration ./integration/phase7/...
```

Kubernetes checks:

```sh
kubectl apply --dry-run=server -f deploy/kubernetes/crds/
helm lint deploy/helm/dbvault
helm template dbvault deploy/helm/dbvault
```

Configuration checks:

```sh
dbvault config validate --config configs/dbvault.phase7.example.yaml
dbvault config print-effective --config configs/dbvault.phase7.example.yaml
dbvault repository explain --config configs/dbvault.phase7.example.yaml --id production
dbvault source test --config configs/dbvault.phase7.example.yaml --id application-postgres
dbvault destination test --config configs/dbvault.phase7.example.yaml --id r2-primary
```

External storage contract tests require dedicated test buckets and credentials. Never point destructive integration tests at a production prefix.
