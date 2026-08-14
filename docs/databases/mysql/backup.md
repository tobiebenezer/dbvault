# MySQL Backup

Phase 4 uses `mysqldump` with safe defaults: `--single-transaction`, `--quick`, routines, triggers and events. Non-transactional tables produce policy-driven warnings or failures.
