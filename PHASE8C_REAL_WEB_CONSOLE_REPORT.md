# DBVault Phase 8C Real Web Console Report

## Purpose

Phase 8C replaces the placeholder embedded UI shell with a real source-backed web console under `web/`, while preserving the self-contained Go appliance model.

The browser console now has a source tree, a build pipeline, generated assets, and Go-embedded release artifacts.

## New frontend layout

```text
web/
├── package.json
├── index.html
├── scripts/
│   ├── build-web.mjs
│   ├── test-web.mjs
│   └── dev-server.mjs
├── src/
│   ├── api.js
│   ├── format.js
│   ├── state.js
│   ├── main.js
│   ├── components/
│   │   ├── layout.js
│   │   ├── timeline.js
│   │   └── ui.js
│   ├── pages/
│   │   ├── overview.js
│   │   ├── setup.js
│   │   ├── databases.js
│   │   ├── repositories.js
│   │   ├── recovery.js
│   │   ├── jobs.js
│   │   ├── alerts.js
│   │   └── settings.js
│   └── styles/
│       └── app.css
└── dist/
    ├── index.html
    ├── app.js
    └── app.css
```

The Go server embeds the release assets from:

```text
internal/server/web/dist/
├── index.html
├── app.js
└── app.css
```

## Build flow

```text
web/src/*
    ↓ npm run build
web/dist/*
    ↓ copied by build script
internal/server/web/dist/*
    ↓ Go embed.FS
single dbvault binary
```

No Node.js runtime is required on the customer server.

## Implemented UI areas

- Overview dashboard
- Protection status panel
- Recovery timeline component
- First-run setup wizard scaffold
- Database list and discovery actions
- Repository graph
- Recovery centre and sandbox restore action
- Jobs page
- Alerts page
- Settings page with recovery/support bundle actions
- Command palette with Ctrl/Cmd+K
- Toast notifications
- Responsive layout
- Accessible landmarks and status text

## API client coverage

The web app calls the existing Phase 8B appliance routes:

- `GET /api/v1/status`
- `GET /api/v1/overview`
- `GET /api/v1/protection-summary`
- `GET /api/v1/setup`
- `POST /api/v1/setup/steps/{step}`
- `POST /api/v1/setup/finish`
- `GET /api/v1/sources/{source_id}/recovery-timeline`
- `POST /api/v1/doctor/run`
- `POST /api/v1/policies/simulate`
- `POST /api/v1/agents/{agent_id}/discover`
- `GET /api/v1/discoveries`
- `GET /api/v1/alerts`
- `POST /api/v1/sandboxes`
- `POST /api/v1/recovery-bundles`
- `POST /api/v1/support-bundles`

## Makefile updates

Added real web build/test behaviour:

```makefile
web-build:
	cd web && npm run build

web-test:
	cd web && npm test

test-phase8c:
	$(MAKE) web-test
	$(MAKE) web-build
	$(MAKE) test-phase8b
	$(MAKE) embed-check
```

## Verification performed

```bash
cd web && npm test
cd web && npm run build
make test-phase8c
node --check web/dist/app.js
CGO_ENABLED=0 go build -tags=restricted ./cmd/dbvault ./cmd/dbvaultd ./cmd/dbvault-controller ./cmd/dbvault-agent ./cmd/dbvault-operator
```

Appliance smoke test:

```bash
bin/dbvault server --listen 127.0.0.1:18082 --data-dir /tmp/dbvault-phase8c-smoke --demo
curl http://127.0.0.1:18082/
curl http://127.0.0.1:18082/assets/app.js
curl http://127.0.0.1:18082/api/v1/overview
```

The embedded index, JavaScript asset, and overview API responded correctly.

## Honest limitations

This is now a real source-backed web console, but still not the final commercial-grade frontend.

Remaining work:

- Replace demo/scaffold API data with real catalogue evidence.
- Add real form validation for every setup step.
- Add auth/session-aware UI states.
- Add full browser automation with Playwright.
- Add axe accessibility tests.
- Add visual regression tests.
- Add a typed API client once the OpenAPI contract stabilises.
- Replace the dependency-light build with React/Vite/Tailwind if the project chooses that stack for production.

The important correction is complete: `web/` is no longer empty, and the embedded UI now comes from a real frontend source tree.
