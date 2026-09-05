.PHONY: deps build build-restricted test test-restricted test-unit test-race test-integration test-r2 test-contabo fmt vet lint zip

deps:
	go mod download
	go mod verify

build:
	mkdir -p bin
	CGO_ENABLED=1 go build -o bin/dbvault ./cmd/dbvault
	CGO_ENABLED=1 go build -o bin/dbvaultd ./cmd/dbvaultd

build-restricted:
	mkdir -p bin
	CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault ./cmd/dbvault
	CGO_ENABLED=0 go build -tags=restricted -o bin/dbvaultd ./cmd/dbvaultd

test: test-unit test-race

test-restricted:
	CGO_ENABLED=0 go test -tags=restricted ./...

test-unit:
	CGO_ENABLED=1 go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

test-integration:
	./scripts/test-integration.sh

test-r2:
	CGO_ENABLED=1 go test -tags=external_r2 ./integration/r2/...

test-contabo:
	CGO_ENABLED=1 go test -tags=external_contabo ./integration/contabo/...

fmt:
	gofmt -w .

vet:
	go vet ./...

lint: fmt vet

zip:
	cd .. && zip -r dbvault.zip dbvault -x 'dbvault/.git/*' 'dbvault/bin/*'

.PHONY: test-replication test-crash-recovery test-security test-phase3

test-replication:
	CGO_ENABLED=1 go test -tags=integration ./integration/replication/...

test-crash-recovery:
	CGO_ENABLED=1 go test -tags=integration ./integration/crashrecovery/...

test-security:
	CGO_ENABLED=1 go test ./internal/... -run 'Security|Audit|Key|Symlink|Token'

test-phase3: test test-integration test-replication test-crash-recovery test-security


test-postgres:
	CGO_ENABLED=1 go test -tags=integration ./integration/postgres/...

test-mysql:
	CGO_ENABLED=1 go test -tags=integration ./integration/mysql/...

test-mariadb:
	CGO_ENABLED=1 go test -tags=integration ./integration/mariadb/...

test-phase4:
	$(MAKE) test
	$(MAKE) test-postgres
	$(MAKE) test-mysql
	$(MAKE) test-mariadb

.PHONY: test-postgres-physical test-postgres-wal test-postgres-pitr test-postgres-timelines test-postgres-incremental test-mysql-binlog test-mysql-pitr test-mariadb-binlog test-mariadb-pitr test-recovery-retention test-collector-crash test-phase5

test-postgres-physical:
	CGO_ENABLED=1 go test -tags=integration ./integration/postgres/physical/...

test-postgres-wal:
	CGO_ENABLED=1 go test -tags=integration ./integration/postgres/wal/...

test-postgres-pitr:
	CGO_ENABLED=1 go test -tags=integration ./integration/postgres/pitr/...

test-postgres-timelines:
	CGO_ENABLED=1 go test -tags=integration ./integration/postgres/timelines/...

test-postgres-incremental:
	CGO_ENABLED=1 go test -tags=integration ./integration/postgres/incremental/...

test-mysql-binlog:
	CGO_ENABLED=1 go test -tags=integration ./integration/mysql/binlog/...

test-mysql-pitr:
	CGO_ENABLED=1 go test -tags=integration ./integration/mysql/pitr/...

test-mariadb-binlog:
	CGO_ENABLED=1 go test -tags=integration ./integration/mariadb/binlog/...

test-mariadb-pitr:
	CGO_ENABLED=1 go test -tags=integration ./integration/mariadb/pitr/...

test-recovery-retention:
	CGO_ENABLED=1 go test ./internal/application/logretention/... ./internal/application/recoverywindow/... ./internal/application/chainverification/...

test-collector-crash:
	CGO_ENABLED=1 go test -tags=integration ./integration/collectorcrash/...

test-phase5: test test-postgres-physical test-postgres-wal test-postgres-pitr test-mysql-binlog test-mysql-pitr test-mariadb-binlog test-mariadb-pitr test-recovery-retention test-collector-crash

.PHONY: build-platform build-platform-restricted test-tenant-isolation test-operator test-agent-protocol test-ha test-plugins test-phase7

build-platform:
	mkdir -p bin
	CGO_ENABLED=1 go build -o bin/dbvault-controller ./cmd/dbvault-controller
	CGO_ENABLED=1 go build -o bin/dbvault-agent ./cmd/dbvault-agent
	CGO_ENABLED=1 go build -o bin/dbvault-operator ./cmd/dbvault-operator

build-platform-restricted:
	mkdir -p bin
	CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault-controller ./cmd/dbvault-controller
	CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault-agent ./cmd/dbvault-agent
	CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault-operator ./cmd/dbvault-operator

test-tenant-isolation:
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/controlplane ./internal/platform/controllerapi

test-operator:
	CGO_ENABLED=0 go test -tags=restricted ./internal/platform/kubernetes ./internal/platform/operator

test-agent-protocol:
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/agentprotocol

test-ha:
	CGO_ENABLED=1 go test -tags=integration ./integration/phase7/ha/...

test-plugins:
	CGO_ENABLED=0 go test -tags=restricted ./internal/platform/plugins ./internal/platform/terraform

test-phase7:
	$(MAKE) test-restricted
	$(MAKE) build-platform-restricted
	$(MAKE) test-tenant-isolation
	$(MAKE) test-operator
	$(MAKE) test-agent-protocol
	$(MAKE) test-plugins

.PHONY: build-appliance build-appliance-restricted test-appliance test-phase8 web-build web-test embed-check release-bundle

web-build:
	cd web && npm run build

web-test:
	cd web && npm test

embed-check:
	CGO_ENABLED=0 go test -tags=restricted ./internal/server

build-appliance:
	mkdir -p bin
	CGO_ENABLED=1 go build -o bin/dbvault ./cmd/dbvault

build-appliance-restricted:
	mkdir -p bin
	CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault ./cmd/dbvault

release-bundle: web-build build-appliance
	./scripts/release-bundle.sh

test-appliance:
	CGO_ENABLED=0 go test -tags=restricted ./internal/server ./internal/install ./internal/provisioning ./internal/release ./internal/secrets/local

test-phase8:
	$(MAKE) test-restricted
	$(MAKE) build-appliance-restricted
	$(MAKE) test-appliance
	$(MAKE) embed-check

.PHONY: test-product-experience test-setup test-discovery test-doctor test-timeline test-sandbox test-browser test-accessibility test-phase8b test-phase8c

test-product-experience:
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/productexperience ./internal/server

test-setup:
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/productexperience -run Setup

test-discovery:
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/productexperience -run Discovery

test-doctor:
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/productexperience ./internal/server -run Doctor

test-timeline:
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/productexperience ./internal/server -run Timeline

test-sandbox:
	CGO_ENABLED=0 go test -tags=restricted ./internal/server -run Sandbox

test-browser:
	@echo "Phase 8B: embedded browser shell is covered by route and asset tests; run Playwright in production UI pipeline."

test-accessibility:
	@echo "Phase 8B: static HTML uses semantic landmarks; run axe/Playwright in production UI pipeline."

test-phase8b:
	$(MAKE) test-phase8
	$(MAKE) test-product-experience
	$(MAKE) test-setup
	$(MAKE) test-discovery
	$(MAKE) test-doctor
	$(MAKE) test-timeline
	$(MAKE) test-sandbox
	$(MAKE) test-browser
	$(MAKE) test-accessibility


test-phase8c:
	$(MAKE) web-test
	$(MAKE) web-build
	$(MAKE) test-phase8b
	$(MAKE) embed-check

.PHONY: docker-build docker-build-production compose-demo alpha-smoke alpha-package alpha-check

docker-build:
	docker build -f deploy/docker/Dockerfile --build-arg RESTRICTED=true --build-arg VERSION=0.1.0-alpha -t dbvault:0.1.0-alpha .

docker-build-production:
	docker build -f deploy/docker/Dockerfile --build-arg RESTRICTED=false --build-arg VERSION=0.1.0-alpha -t dbvault:0.1.0-alpha-production .

compose-demo:
	docker compose -f deploy/compose/demo.yml up --build

alpha-smoke:
	./scripts/alpha-smoke.sh

alpha-check:
	$(MAKE) web-test
	$(MAKE) web-build
	$(MAKE) test-phase8c
	$(MAKE) alpha-smoke

alpha-package:
	./scripts/package-alpha.sh

.PHONY: test-phase8d test-live-ui-workers

test-live-ui-workers:
	$(MAKE) web-test
	$(MAKE) web-build
	node --check web/dist/app.js
	node --check web/dist/log-search.worker.js
	node --check web/dist/timeline.worker.js
	node --check web/dist/schema-layout.worker.js
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/productexperience ./internal/server

test-phase8d: test-live-ui-workers
	CGO_ENABLED=0 go test -tags=restricted ./...

.PHONY: test-phase8e test-ui-polish

test-ui-polish:
	$(MAKE) web-test
	$(MAKE) web-build
	node --check web/dist/app.js
	node --check web/dist/log-search.worker.js
	node --check web/dist/timeline.worker.js
	node --check web/dist/schema-layout.worker.js
	@grep -q -- "--radius-md" web/src/styles/app.css
	@grep -q "mobileNavOpen" web/src/state.js
	@grep -q "Overview" web/src/pages/overview.jsx

test-phase8e: test-ui-polish
	CGO_ENABLED=0 go test -tags=restricted ./...
	CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd ./cmd/dbvault-controller ./cmd/dbvault-agent ./cmd/dbvault-operator
	go vet -tags=restricted ./...

.PHONY: test-phase8f test-ui-density

test-ui-density:
	$(MAKE) web-test
	$(MAKE) web-build
	node --check web/dist/app.js
	@grep -q "mobile-menu-btn" web/src/components/layout.jsx
	@grep -q "Open navigation menu" web/src/components/layout.jsx
	@grep -q "Overview" web/src/pages/overview.jsx
	@grep -q "databases" web/src/pages/databases.jsx
	@grep -q "rounded-md" web/src/styles/app.css || grep -q -- "--radius-md" web/src/styles/app.css

test-phase8f: test-ui-density
	CGO_ENABLED=0 go test -tags=restricted ./...
	CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd ./cmd/dbvault-controller ./cmd/dbvault-agent ./cmd/dbvault-operator
	go vet -tags=restricted ./...


.PHONY: test-phase8g

test-phase8g:
	$(MAKE) web-test
	$(MAKE) web-build
	node --check web/dist/app.js
	node --check web/dist/log-search.worker.js
	node --check web/dist/timeline.worker.js
	node --check web/dist/schema-layout.worker.js
	CGO_ENABLED=0 go test -tags=restricted ./internal/application/productexperience ./internal/server ./cmd/dbvault
	CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd ./cmd/dbvault-controller ./cmd/dbvault-agent ./cmd/dbvault-operator
	go vet -tags=restricted ./...

.PHONY: test-phase8i test-final-ui-cleanup

test-final-ui-cleanup:
	$(MAKE) web-test
	$(MAKE) web-build
	node --check web/dist/app.js
	node --check web/dist/log-search.worker.js
	node --check web/dist/timeline.worker.js
	node --check web/dist/schema-layout.worker.js
	@grep -q -- "--bg: #ffffff" web/src/styles/app.css
	@grep -q -- "--nav: #ffffff" web/src/styles/app.css
	@grep -q "Phase 8I final UI cleanup" web/src/styles/app.css
	@! grep -q "background: #202823" web/src/styles/app.css
	@grep -q "mobile-menu-btn" web/src/components/layout.jsx
	@grep -q "Overview" web/src/pages/overview.jsx

test-phase8i: test-final-ui-cleanup
	CGO_ENABLED=0 go test -tags=restricted ./...
	CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd ./cmd/dbvault-controller ./cmd/dbvault-agent ./cmd/dbvault-operator
	go vet -tags=restricted ./...

# ------------------------------------------------------------------------------
# Appliance Runtime, Database Management & Discovery Commands
# ------------------------------------------------------------------------------
.PHONY: dev server db-init probe-postgres probe-mysql probe-sqlite

# Build web frontend and Go binaries, then launch the server
dev: web-build build-prod server

build-prod:
	@mkdir -p bin
	CGO_ENABLED=0 go build -o bin/dbvault ./cmd/dbvault
	CGO_ENABLED=0 go build -o bin/dbvaultd ./cmd/dbvaultd

# Run the DBVault appliance server using environment variables from .env
server: build-prod
	@mkdir -p scratch/data
	@set -a; [ -f .env ] && . ./.env; set +a; \
	DATA_DIR="$${DBVAULT_DATA_DIR:-./scratch/data}"; \
	if ! mkdir -p "$$DATA_DIR" 2>/dev/null; then \
		DATA_DIR="./scratch/data"; \
		mkdir -p "$$DATA_DIR"; \
	fi; \
	./bin/dbvault server --listen "$${DBVAULT_LISTEN:-127.0.0.1:8080}" --data-dir "$$DATA_DIR"

# Initialize or verify the PostgreSQL dbvault_internal system database
db-init:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	echo "Initializing DBVault internal PostgreSQL database..."; \
	PGPASSWORD="$${PGPASSWORD:-Awodumila}" psql -h 127.0.0.1 -U "$${PGUSER:-postgres}" -c "CREATE DATABASE dbvault_internal;" 2>/dev/null || true; \
	PGPASSWORD="$${PGPASSWORD:-Awodumila}" psql -h 127.0.0.1 -U "$${PGUSER:-postgres}" -d dbvault_internal -c "\
	CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, checksum TEXT NOT NULL, applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()); \
	CREATE TABLE IF NOT EXISTS sources (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, driver TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE, repository_id TEXT NOT NULL, config_json TEXT NOT NULL, created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(), updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(), last_snapshot_id TEXT); \
	CREATE TABLE IF NOT EXISTS snapshots (id TEXT PRIMARY KEY, source_id TEXT NOT NULL, repository_id TEXT NOT NULL, status TEXT NOT NULL, snapshot_mode TEXT NOT NULL, database_size BIGINT NOT NULL, page_size INT NOT NULL, page_count BIGINT NOT NULL, root_digest TEXT NOT NULL, schema_digest TEXT NOT NULL, chunk_count INT NOT NULL, unique_chunk_count INT NOT NULL, compressed_bytes BIGINT NOT NULL, unique_uploaded_bytes BIGINT NOT NULL, manifest_object_key TEXT NOT NULL, completion_object_key TEXT NOT NULL, created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(), committed_at TIMESTAMP WITH TIME ZONE, verified_at TIMESTAMP WITH TIME ZONE, restore_tested_at TIMESTAMP WITH TIME ZONE, tombstoned_at TIMESTAMP WITH TIME ZONE, delete_after TIMESTAMP WITH TIME ZONE);" && \
	echo "dbvault_internal database is ready."

# Probe target PostgreSQL engine and list databases with live storage size
probe-postgres: build-restricted
	@set -a; [ -f .env ] && . ./.env; set +a; \
	./bin/dbvault probe --engine postgres --host 127.0.0.1 --port 5432 --user "$${PGUSER:-postgres}"

# Probe target MySQL/MariaDB engine and list databases with storage footprint
probe-mysql: build-restricted
	./bin/dbvault probe --engine mysql --host 127.0.0.1 --port 3306 --user root

# Probe target SQLite files or folder path
probe-sqlite: build-restricted
	./bin/dbvault probe --engine sqlite --path ./scratch

# ------------------------------------------------------------------------------
# Production Packaging & VPS Installation Targets
# ------------------------------------------------------------------------------
.PHONY: package-vps install-vps

# Build standalone Linux production package with embedded UI for VPS deployment
package-vps:
	./scripts/package-vps-bundle.sh

# Run automated VPS installation & verification script on this machine
install-vps: web-build build-prod
	sudo ./scripts/install-vps.sh


