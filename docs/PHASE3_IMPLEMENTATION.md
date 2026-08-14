# DBVault Phase 3 Implementation Notes

This phase adds the production-hardening seams and the first concrete pieces for verified recovery:

- durable job domain model and queue adapter
- retry, lease and expired-job recovery rules
- backup recovery-summary service
- repository scanner result model
- replication service that copies encrypted objects without decrypting payloads
- restore drill service
- restore hardening: completion digest check, signature verification, offset writes, SQLite header validation and rollback-safe replacement
- GC stale-plan protection
- budget evaluator
- audit hash chaining
- Phase 3 integration compose skeleton with two MinIO destinations

Restricted build validation:

```bash
CGO_ENABLED=0 go test -tags=restricted ./...
CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd
```

Production Phase 3 validation in a full environment should run:

```bash
make deps
make build
make test
make test-integration
make test-replication
make test-crash-recovery
make test-security
```

The Phase 3 code still keeps some production integrations as seams where external infrastructure is required, especially the two-MinIO replication and crash-restart harness. Those tests are present under integration build tags so they can be made mandatory in CI without breaking restricted environments.
