# Phase 8G: Connected Uniform UI

Phase 8G removes browser-only placeholder actions and applies a single visual identity across the DBVault console.

## Brand tokens

```css
--primary: #0f766e;
--primary-dark: #0b5f59;
--primary-soft: #ecfdf5;
--nav: #171c19;
```

Blue and purple accents are prohibited. Warning amber and danger red are semantic exceptions.

## Connected paths

- Backup and restore-drill jobs
- Destination tests and replication retries
- Inventory-backed databases and repositories
- Discovery adoption and ignore flows
- Setup state and policy simulation
- Doctor checks
- Sandbox restore jobs
- Alert acknowledgement and resolution
- Restore approval requests
- Recovery and support bundles
- Update manifest checks

## Validation

Run:

```bash
make test-phase8g
```
