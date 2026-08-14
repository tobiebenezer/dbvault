# Phase 8D Live Workers and UI Wiring

Phase 8D wires the DBVault appliance UI to live job evidence.

Implemented scope:

- `/api/v1/jobs` list and create scaffold jobs.
- `/api/v1/jobs/{id}` detail.
- `/api/v1/jobs/{id}/cancel` safe cancellation scaffold.
- `/api/v1/jobs/{id}/retry` creates a new attempt.
- `/api/v1/jobs/{id}/logs` returns redacted job log lines.
- `/api/v1/events/jobs` streams Server-Sent Events.
- `/events/jobs` remains as a compatibility route.
- Frontend `EventSource` client reconnects with bounded backoff.
- Frontend job store ignores duplicate events and prevents terminal jobs from reverting to running.
- Jobs page uses live state, not static rows.
- Job detail view includes stages and a log viewer.
- Log search worker, timeline worker, and schema-layout worker are included as browser workers.
- Card-like UI surfaces use rounded-md-equivalent or greater border radius.

Current limitation:

The real durable worker pool is still scaffolded in restricted builds. The current server generates demo progress events so the browser integration, SSE, reconnect, cancellation, retry, and UI state mechanics can be tested without production database credentials.

Production connection points:

1. Replace `productexperience.GenerateDemoProgress` with durable job event publication from Go backup, restore, drill, sandbox, verification, replication, doctor, bundle, provisioning, and upgrade workers.
2. Persist job events in the production catalogue using migrations 090 to 094.
3. Add cursor-aware replay by authenticated user and tenant.
4. Add browser tests for reconnect, refresh, cancellation, retry, and log search.
