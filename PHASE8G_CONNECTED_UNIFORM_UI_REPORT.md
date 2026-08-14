# DBVault Phase 8G Connected and Uniform UI Report

## Goal

Connect visible console actions to appliance APIs and unify the interface around one emerald product accent with a neutral charcoal sidebar.

## Connection work completed

- Overview uses live overview, inventory, recovery timeline, jobs, and alert data.
- Database rows and repository topology come from `/api/v1/inventory`.
- Backup, restore-drill, destination-test, and replication actions create live jobs.
- Discovery supports scan, adopt, and ignore operations.
- Setup step identifiers match backend state and persist across navigation.
- Doctor and policy simulation call their real appliance endpoints.
- Sandbox restore creation returns and opens its live job.
- Alert actions execute, acknowledge, or resolve through the API.
- Production restore review creates a pending approval request through `/api/v1/approvals`.
- Recovery and support bundles call their appliance endpoints.
- Update checks read the embedded release manifest.
- Job state remains connected through REST and SSE.

## Visual system

- Product accent: emerald `#0f766e`.
- Sidebar: neutral charcoal `#171c19`.
- Sidebar active state uses the same emerald accent.
- Blue, indigo, violet, purple, and sky accent tokens were removed.
- Amber and red remain limited to warning and destructive semantic states.
- Card surfaces retain `rounded-md` or greater.

## Verification

- `make test-phase8g`
- Browser rendering at 1440px and 390px
- Zero horizontal overflow at 390px
- Mobile hamburger and drawer verified
- Runtime smoke tests for inventory, jobs, discovery, alerts, setup, sandbox, approvals, SSE, UI, and embedded assets

## Remaining production boundary

The UI is connected to the current appliance API and alpha durable-job model. Some backend resources still use Phase 8 alpha evidence and in-memory stores. Production database workers, persistent hosted control-plane storage, and real provider evidence must replace those alpha implementations before a production release.
