# Phase 7 control-plane scaffold

The repository now includes `dbvault-controller`, `dbvault-agent`, and `dbvault-operator` binaries. The controller enforces organisation headers at the API boundary and the in-memory control-plane store repeats tenant checks at the data boundary. Agent tasks use Ed25519 signatures, expiry times, nonces, and replay protection. Destructive approval logic prevents requesters from approving their own action and supports multi-person approval.

The current operator is a dependency-light reconciliation scaffold. Production Kubernetes deployment should bind the resource models to controller-runtime informers and Kubernetes Lease leader election.
