# Phase 5 Implementation Notes

This implementation adds the Phase 5 domain and adapter seams for physical backups, transaction logs, recovery windows and PITR. Restricted builds compile and test without native PostgreSQL/MySQL tooling. Production/integration environments should wire the new recovery drivers to real pg_basebackup, WAL collection, mysqlbinlog and PITR restore targets.

Key additions:
- recovery-driver API v1
- PostgreSQL physical backup driver seam
- PostgreSQL WAL archive helper seam
- MySQL/MariaDB binlog collector seam
- PITR plan/result models
- recovery-window calculation
- chain verification
- log-retention planning
- Phase 5 migrations 030-040
- Phase 5 Docker Compose skeleton
- Phase 5 CLI command scaffolds
