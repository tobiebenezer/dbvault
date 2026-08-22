package s3

import (
	"strings"

	"github.com/dbvault/dbvault/internal/ports"
)

type Config struct {
	Endpoint, Region, Bucket, Prefix, AccessKeyID, SecretAccessKey string
	UsePathStyle                                                   bool
}
type Profile struct {
	Name, DefaultRegion string
	ForcePathStyle      bool
	Capabilities        ports.StorageCapabilities
}

func BuiltinProfile(name string) Profile {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "r2") || strings.Contains(lower, "cloudflare"):
		return Profile{Name: "cloudflare-r2", DefaultRegion: "auto", ForcePathStyle: true, Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, PathStyleAddressing: true}}
	case strings.Contains(lower, "contabo"):
		return Profile{Name: "contabo", DefaultRegion: "default", ForcePathStyle: true, Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, PathStyleAddressing: true}}
	case strings.Contains(lower, "minio"):
		return Profile{Name: "minio", DefaultRegion: "us-east-1", ForcePathStyle: true, Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, PathStyleAddressing: true}}
	case strings.Contains(lower, "wasabi"):
		return Profile{Name: "wasabi", DefaultRegion: "us-east-1", ForcePathStyle: true, Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, PathStyleAddressing: true}}
	default:
		return Profile{Name: "generic-s3", DefaultRegion: "us-east-1", ForcePathStyle: true, Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, PathStyleAddressing: true}}
	}
}
