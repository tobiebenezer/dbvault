# DBVault Docker Guide

Docker is an optional DBVault install path.

Use Docker for:

- demos
- local development
- CI smoke tests
- staging control-plane trials
- future Kubernetes packaging

Use native systemd for early real VPS pilots where DBVault must access local database sockets, filesystem paths, native backup tools, and operating-system permissions.

---

## Demo

```bash
docker compose -f deploy/compose/demo.yml up --build
```

Open:

```text
http://127.0.0.1:8080
```

---

## Build image

```bash
make docker-build
```

Run:

```bash
docker run --rm -p 8080:8080 \
  -v dbvault-data:/var/lib/dbvault \
  dbvault:0.1.0-alpha \
  server --demo --listen 0.0.0.0:8080 --data-dir /var/lib/dbvault
```

---

## Image policy

The image contains:

- DBVault binary
- embedded UI inside the binary
- CA certificates
- non-root runtime

The image must not contain:

- Node.js runtime
- Nginx
- Caddy
- Redis
- PostgreSQL server
- MySQL server
- hardcoded secrets

