# DBVault Alpha 0.1 Hardening Checklist

## Release-blocking for public alpha

- [ ] `make alpha-check` passes.
- [ ] Docker demo image builds.
- [ ] Demo Compose starts from a clean checkout.
- [ ] Embedded UI loads without external frontend service.
- [ ] Setup token is generated and not reused after completion.
- [ ] Overview page works in demo mode.
- [ ] Recovery timeline works in demo mode.
- [ ] Doctor route returns actionable checks.
- [ ] Policy simulation route returns estimated storage.
- [ ] Support bundle excludes secrets.
- [ ] Recovery bundle excludes unencrypted master keys.
- [ ] README has clear alpha warning.
- [ ] Install guide has native and Docker paths.
- [ ] Known gaps are documented honestly.

## Production-readiness blockers

- [ ] Production `go mod download` works.
- [ ] `go.sum` committed.
- [ ] CGO SQLite production build passes.
- [ ] `go test -race ./...` passes.
- [ ] Real R2 put/head/get/list/delete test passes.
- [ ] Real Contabo put/head/get/list/delete test passes.
- [ ] Real PostgreSQL dump/restore passes.
- [ ] Real MySQL/MariaDB dump/restore passes.
- [ ] Restore drill verifies restored DB contents.
- [ ] Protection status uses real catalogue evidence.
- [ ] Recovery timeline uses real backup history.
- [ ] Playwright setup wizard test passes.
- [ ] Accessibility checks pass.
- [ ] Security review of installer and agent enrolment complete.
- [ ] Release signing keys created.
- [ ] SBOM generated.
- [ ] Vulnerability scan performed.

## Pilot readiness

- [ ] At least one staging PostgreSQL pilot completes a restore drill.
- [ ] At least one staging SQLite pilot completes a restore drill.
- [ ] Support bundle provides enough information to debug failures.
- [ ] Recovery bundle can be inspected on another machine.
- [ ] Upgrade rollback has been tested.
- [ ] Documentation explains how to uninstall safely.
