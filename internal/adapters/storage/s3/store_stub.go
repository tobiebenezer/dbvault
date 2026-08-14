//go:build restricted

package s3

import (
	"context"
	"fmt"
	"github.com/dbvault/dbvault/internal/ports"
)

type Store struct {
	Config  Config
	Profile Profile
}

func New(cfg Config, profileName string) *Store {
	p := BuiltinProfile(profileName)
	if cfg.Region == "" {
		cfg.Region = p.DefaultRegion
	}
	if p.ForcePathStyle {
		cfg.UsePathStyle = true
	}
	return &Store{Config: cfg, Profile: p}
}
func (s *Store) Name() string { return "s3:" + s.Profile.Name }
func (s *Store) Validate(ctx context.Context) error {
	if s.Config.Endpoint == "" || s.Config.Bucket == "" {
		return fmt.Errorf("s3 endpoint and bucket are required")
	}
	return nil
}
func (s *Store) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return s.Profile.Capabilities, nil
}
func (s *Store) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	return ports.StoredObject{}, fmt.Errorf("real S3 adapter not compiled in this dependency-free MVP; use filesystem adapter or add AWS SDK implementation")
}
func (s *Store) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	return nil, ports.StoredObject{}, fmt.Errorf("real S3 adapter not compiled in this dependency-free MVP")
}
func (s *Store) Head(ctx context.Context, key string) (ports.StoredObject, error) {
	return ports.StoredObject{}, fmt.Errorf("real S3 adapter not compiled in this dependency-free MVP")
}
func (s *Store) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	return ports.ListObjectsResult{}, fmt.Errorf("real S3 adapter not compiled in this dependency-free MVP")
}
func (s *Store) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("real S3 adapter not compiled in this dependency-free MVP")
}
