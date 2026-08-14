# Kubernetes operator

CRDs are supplied for destinations, repositories, sources, schedules, and restores. The source CRD contains secret references rather than secret values. Discovery is opt-in and should use the `dbvault.io/discover=true` label. CSI snapshots may assist database-native backup flows but do not replace engine consistency rules.
