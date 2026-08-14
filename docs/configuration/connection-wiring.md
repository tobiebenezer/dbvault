# Source, repository and storage wiring

DBVault resolves a source through explicit IDs:

`source.repository -> repository.primary/mirrors/replicas -> destination`.

Database and storage credentials are `SecretReference` values. File-provider references are resolved below the configured provider root, environment references are read at runtime, and literal values are rejected by production policy. PostgreSQL native tools receive an ephemeral `PGPASSFILE`; MySQL and MariaDB receive an ephemeral `--defaults-extra-file`. Both files use mode `0600` and are removed after the process exits.

Use:

```sh
dbvault config validate --config /etc/dbvault/dbvault.yaml
dbvault source test --config /etc/dbvault/dbvault.yaml --id application-postgres
dbvault destination test --config /etc/dbvault/dbvault.yaml --id r2-primary
dbvault repository explain --config /etc/dbvault/dbvault.yaml --id production
```
