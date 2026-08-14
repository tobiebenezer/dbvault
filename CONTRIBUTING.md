# Contributing to DBVault

Thank you for helping make recovery less wishful.

## Start here

```bash
make web-test
make web-build
make test-restricted
make alpha-smoke
```

## Good first areas

- UI polish in `web/src`
- better doctor checks
- clearer error messages
- demo data improvements
- docs and install guides
- Playwright browser tests
- accessibility checks

## Rules

- Do not weaken backup correctness for convenience.
- Do not log secrets.
- Do not make intelligence/AI part of the critical backup path.
- Prefer deterministic safety checks over magical guesses.
- Every failure should have a user-facing next action.
