# PostgreSQL Restore Runbook

1. Run `dbvault restore plan` against a staging target.
2. Confirm server version, extensions, roles and tablespaces.
3. Restore globals only when authorised.
4. Restore custom archives with `pg_restore`; restore SQL artifacts with `psql ON_ERROR_STOP=on`.
5. Run engine verification and application probes.
