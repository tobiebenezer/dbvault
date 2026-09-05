package ports

import (
	"context"
	"io"
	"time"
)

type StorageCapabilities struct {
	MultipartUpload       bool
	ConditionalCreate     bool
	BatchDelete           bool
	UserMetadata          bool
	RangeReads            bool
	PathStyleAddressing   bool
	VirtualHostAddressing bool
	ObjectLock            bool
}

type ObjectLockRetention struct {
	Mode            string // "COMPLIANCE" or "GOVERNANCE"
	RetainUntilDate time.Time
}

type PutObjectRequest struct {
	Key             string
	Body            io.Reader
	Size            int64
	ContentType     string
	Metadata        map[string]string
	IfNotExists     bool
	LockRetention   *ObjectLockRetention
	LegalHold       bool
	LockMode        string     // "COMPLIANCE" or "GOVERNANCE"
	RetainUntilDate *time.Time // WORM retention expiration
}

type StoredObject struct {
	Key          string
	Size         int64
	ETag         string
	LastModified time.Time
	Metadata     map[string]string
}

type GetObjectRequest struct{ Key string }

type ObjectReader interface{ io.ReadCloser }

type ListObjectsRequest struct {
	Prefix            string
	ContinuationToken string
	Limit             int
}

type ListObjectsResult struct {
	Objects           []StoredObject
	ContinuationToken string
	HasMore           bool
}

type ObjectStore interface {
	Name() string
	Validate(ctx context.Context) error
	Capabilities(ctx context.Context) (StorageCapabilities, error)
	Put(ctx context.Context, req PutObjectRequest) (StoredObject, error)
	Get(ctx context.Context, req GetObjectRequest) (ObjectReader, StoredObject, error)
	Head(ctx context.Context, key string) (StoredObject, error)
	List(ctx context.Context, req ListObjectsRequest) (ListObjectsResult, error)
	Delete(ctx context.Context, key string) error
}
