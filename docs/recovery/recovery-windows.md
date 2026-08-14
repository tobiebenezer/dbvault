# Recovery Windows

A recovery window is a continuous interval backed by a verified base backup and uninterrupted transaction logs. Missing WAL or binlog files split windows; DBVault must never merge discontinuous windows.
