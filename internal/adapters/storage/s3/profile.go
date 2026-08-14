package s3

import "github.com/dbvault/dbvault/internal/ports"

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
	switch name {
	case "cloudflare-r2":
		return Profile{Name: name, DefaultRegion: "auto", Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, VirtualHostAddressing: true}}
	case "contabo":
		return Profile{Name: name, DefaultRegion: "default", ForcePathStyle: true, Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, PathStyleAddressing: true}}
	case "minio":
		return Profile{Name: name, DefaultRegion: "us-east-1", ForcePathStyle: true, Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true, PathStyleAddressing: true}}
	default:
		return Profile{Name: "generic-s3", DefaultRegion: "us-east-1", Capabilities: ports.StorageCapabilities{MultipartUpload: true, UserMetadata: true, RangeReads: true}}
	}
}
