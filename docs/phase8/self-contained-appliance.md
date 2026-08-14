# DBVault Phase 8: Self-contained appliance

DBVault now has a single-binary appliance path. The same `dbvault` binary can run
as the web console, API, embedded reverse proxy, installer, upgrade planner,
agent installer and CLI.

## Target experience

```bash
sudo ./dbvault install --domain backup.example.com
systemctl start dbvault
```

Then open:

```text
https://backup.example.com/setup
```

The web UI is compiled into static files and embedded into the Go binary. Target
servers do not need Node.js, npm, Next.js, Redis, Nginx or Caddy.

## Routes

```text
/                         embedded UI
/assets/*                 embedded UI assets
/api/v1/*                 control-plane API
/events/jobs              Server-Sent Events job stream
/agent/v1/*               agent protocol surface
/install/agent.sh         one-line remote agent installer
/install/checksums.txt    release checksums
/health                   liveness
/ready                    readiness
```

## Installation files

```text
/usr/local/bin/dbvault
/etc/dbvault/dbvault.yaml
/etc/dbvault/tls/
/var/lib/dbvault/
/var/lib/dbvault/catalogue/
/var/lib/dbvault/scratch/
/var/lib/dbvault/log-spool/
/var/log/dbvault/
```

## Implemented in this phase

- Embedded UI assets through `embed.FS`
- Integrated HTTP edge server
- Security headers and SPA fallback
- Setup-token lifecycle
- Agent installer endpoint
- Self-contained install planner
- systemd unit generation
- Upgrade and uninstall plans
- Local encrypted secret store
- Release-manifest verification
- Provisioning job stages and log redaction

## Production seams remaining

- ACME certificate issuance is a declared mode but not yet wired to a certificate client.
- SSH provisioning is represented as durable job stages, with the actual SSH executor still to be attached.
- Controller persistence remains a Phase 7 seam unless a production metadata store is configured.
