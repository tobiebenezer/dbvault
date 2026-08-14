# Security Policy

DBVault is backup and recovery software, so security defects can become data-loss defects.

## Alpha warning

DBVault v0.1-alpha is not production-ready. Do not rely on it as your only backup for irreplaceable production data.

## Never include in reports

Do not include:

- database rows
- storage secret keys
- database passwords
- private keys
- setup tokens
- unredacted configuration files
- production backup archives

## Report privately

Until a public security contact is configured, open a private maintainer channel before disclosing sensitive issues publicly.

## High-priority issue classes

- backup corruption
- restore failure after successful verification
- cross-tenant access
- secret leakage
- setup token reuse
- unsigned release acceptance
- malicious repository object execution
- path traversal or symlink escape
- destructive-action bypass
