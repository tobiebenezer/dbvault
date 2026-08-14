# DBVault Alpha Install Guide

## Demo mode

```bash
make web-build
CGO_ENABLED=0 go build -tags=restricted -o bin/dbvault ./cmd/dbvault
bin/dbvault server --demo --listen 127.0.0.1:8080 --data-dir /tmp/dbvault-demo
```

Open:

```text
http://127.0.0.1:8080
```

## Install layout dry run

```bash
bin/dbvault install --root /tmp/dbvault-install --domain backup.example.test --self-signed
```

This creates the same files a real system install would create, but under `/tmp/dbvault-install`.

## Real install direction

The intended production command is:

```bash
sudo dbvault install --domain backup.example.com
```

For alpha testing, use `--root` first and inspect the generated files before installing on a real host.

## Uninstall plan

```bash
dbvault uninstall
```

By default, uninstall must preserve `/etc/dbvault` and `/var/lib/dbvault`.

A destructive purge requires an explicit purge flag.
