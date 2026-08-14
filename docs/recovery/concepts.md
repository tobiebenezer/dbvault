# DBVault Recovery Concepts

Phase 5 adds physical backups, transaction logs, recovery lineages, recovery windows and point-in-time recovery plans. A point is recoverable only when DBVault can prove there is a valid base backup plus every required WAL or binlog segment.
