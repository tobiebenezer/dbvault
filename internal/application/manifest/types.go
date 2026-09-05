package manifest

type SnapshotManifest struct {
	Format        string       `json:"format"`
	FormatVersion int          `json:"format_version"`
	RepositoryID  string       `json:"repository_id"`
	SnapshotID    string       `json:"snapshot_id"`
	SourceID      string       `json:"source_id"`
	RootDigest    string       `json:"root_digest,omitempty"`
	Database      DatabaseInfo `json:"database"`
	Snapshot      SnapshotInfo `json:"snapshot"`
	Chunks        []ChunkInfo  `json:"chunks"`
}

type DatabaseInfo struct {
	Engine        string `json:"engine"`
	SQLiteVersion string `json:"sqlite_version,omitempty"`
	PageSize      int    `json:"page_size"`
	PageCount     int64  `json:"page_count,omitempty"`
	LogicalSize   int64  `json:"logical_size"`
	SchemaDigest  string `json:"schema_digest,omitempty"`
}

type DatabaseMeta = DatabaseInfo

type SnapshotInfo struct {
	Mode       string         `json:"mode"`
	CreatedAt  string         `json:"created_at"`
	RootDigest string         `json:"root_digest"`
	Chunking   map[string]any `json:"chunking,omitempty"`
}

type ChunkInfo struct {
	Index          int    `json:"index,omitempty"`
	Sequence       int    `json:"sequence"`
	ChunkID        string `json:"chunk_id"`
	ObjectKey      string `json:"object_key"`
	PageStart      int64  `json:"page_start"`
	PageCount      int    `json:"page_count"`
	PlaintextSize  int64  `json:"plaintext_size"`
	CompressedSize int64  `json:"compressed_size,omitempty"`
	StoredSize     int64  `json:"stored_size"`
	Compression    string `json:"compression"`
	KeyID          string `json:"key_id"`
}

type Completion struct {
	Format         string `json:"format"`
	FormatVersion  int    `json:"format_version"`
	SnapshotID     string `json:"snapshot_id"`
	ManifestDigest string `json:"manifest_digest"`
	CommittedAt    string `json:"committed_at"`
}
