# Database Driver API v1

Phase 4 introduces `ports.DatabaseDriver`. Drivers produce one or more artifacts through `ArtifactSink`; application services own chunking, compression, encryption, storage, signing, replication, retention and GC.

Rules:

- Drivers must not import storage adapters.
- Drivers must not write plaintext dumps to long-lived paths.
- Native database tools must be invoked through `ports.ProcessRunner`.
- Credentials must be supplied through temporary protected files or sensitive environment variables, never command arguments.
- Every backup set must record engine, driver API, backup mode, backup format, toolchain and artifact ordering.
