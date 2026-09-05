//go:build !restricted

package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

type Store struct {
	client  *s3.Client
	Config  Config
	Profile Profile
}

func normalizeEndpoint(endpoint, bucket string) string {
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		return ""
	}
	if !strings.HasPrefix(ep, "http://") && !strings.HasPrefix(ep, "https://") {
		ep = "https://" + ep
	}
	if u, err := url.Parse(ep); err == nil {
		cleanPath := strings.Trim(u.Path, "/")
		if cleanPath == bucket || strings.HasSuffix(cleanPath, "/"+bucket) || cleanPath == "" || strings.Contains(u.Host, "r2.cloudflarestorage.com") {
			u.Path = ""
			u.RawPath = ""
		}
		return strings.TrimRight(u.String(), "/")
	}
	return strings.TrimRight(ep, "/")
}

func New(cfg Config, profileName string) *Store {
	p := BuiltinProfile(profileName)
	if cfg.Region == "" {
		cfg.Region = p.DefaultRegion
	}
	if p.ForcePathStyle {
		cfg.UsePathStyle = true
	}
	cfg.Endpoint = normalizeEndpoint(cfg.Endpoint, cfg.Bucket)
	access, _ := readSecret(cfg.AccessKeyID)
	secret, _ := readSecret(cfg.SecretAccessKey)
	awsCfg, _ := config.LoadDefaultConfig(context.Background(), config.WithRegion(cfg.Region), config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(access, secret, "")))
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = cfg.UsePathStyle
	})
	return &Store{client: client, Config: cfg, Profile: p}
}
func (s *Store) Name() string { return "s3:" + s.Profile.Name }
func (s *Store) Validate(ctx context.Context) error {
	if s.Config.Endpoint == "" || s.Config.Bucket == "" {
		return fmt.Errorf("s3 endpoint and bucket are required")
	}
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.Config.Bucket)})
	return classify(err)
}
func (s *Store) Capabilities(ctx context.Context) (ports.StorageCapabilities, error) {
	return s.Profile.Capabilities, nil
}
func (s *Store) Put(ctx context.Context, req ports.PutObjectRequest) (ports.StoredObject, error) {
	key, err := JoinObjectKey(s.Config.Prefix, req.Key)
	if err != nil {
		return ports.StoredObject{}, err
	}
	body := req.Body
	if body == nil {
		body = bytes.NewReader(nil)
	}
	in := &s3.PutObjectInput{
		Bucket:      aws.String(s.Config.Bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(req.ContentType),
		Metadata:    req.Metadata,
	}
	if req.IfNotExists {
		in.IfNoneMatch = aws.String("*")
	}

	// Object Lock Retention Mode & Retain Until Date
	if req.LockRetention != nil {
		if req.LockRetention.Mode != "" {
			in.ObjectLockMode = s3types.ObjectLockMode(strings.ToUpper(req.LockRetention.Mode))
		}
		if !req.LockRetention.RetainUntilDate.IsZero() {
			t := req.LockRetention.RetainUntilDate.UTC()
			in.ObjectLockRetainUntilDate = &t
		}
	} else if req.LockMode != "" {
		in.ObjectLockMode = s3types.ObjectLockMode(strings.ToUpper(req.LockMode))
		if req.RetainUntilDate != nil && !req.RetainUntilDate.IsZero() {
			t := req.RetainUntilDate.UTC()
			in.ObjectLockRetainUntilDate = &t
		}
	}

	// Check Metadata fallbacks for Object Lock if not explicitly set
	if in.ObjectLockMode == "" && req.Metadata != nil {
		if m, ok := req.Metadata["dbvault-lock-mode"]; ok && m != "" {
			in.ObjectLockMode = s3types.ObjectLockMode(strings.ToUpper(m))
		}
	}
	if in.ObjectLockRetainUntilDate == nil && req.Metadata != nil {
		if u, ok := req.Metadata["dbvault-lock-until"]; ok && u != "" {
			if parsed, err := time.Parse(time.RFC3339, u); err == nil {
				utc := parsed.UTC()
				in.ObjectLockRetainUntilDate = &utc
			}
		}
	}

	// Object Lock Legal Hold
	if req.LegalHold || (req.Metadata != nil && req.Metadata["dbvault-legal-hold"] == "true") {
		in.ObjectLockLegalHoldStatus = s3types.ObjectLockLegalHoldStatusOn
	}

	out, err := s.client.PutObject(ctx, in)
	if err != nil {
		var app *domain.AppError
		if errors.As(classify(err), &app) {
			if app.Code == domain.ErrStorageUnavailable && strings.Contains(strings.ToLower(app.Message), "precondition") {
				return s.Head(ctx, req.Key)
			}
			return ports.StoredObject{}, app
		}
		return ports.StoredObject{}, err
	}
	etag := ""
	if out.ETag != nil {
		etag = strings.Trim(*out.ETag, "\"")
	}
	return ports.StoredObject{Key: req.Key, Size: req.Size, ETag: etag, LastModified: time.Now().UTC(), Metadata: req.Metadata}, nil
}
func (s *Store) Get(ctx context.Context, req ports.GetObjectRequest) (ports.ObjectReader, ports.StoredObject, error) {
	key, err := JoinObjectKey(s.Config.Prefix, req.Key)
	if err != nil {
		return nil, ports.StoredObject{}, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)})
	if err != nil {
		return nil, ports.StoredObject{}, classify(err)
	}
	obj := ports.StoredObject{Key: req.Key, Metadata: out.Metadata}
	if out.ContentLength != nil {
		obj.Size = *out.ContentLength
	}
	if out.ETag != nil {
		obj.ETag = strings.Trim(*out.ETag, "\"")
	}
	if out.LastModified != nil {
		obj.LastModified = out.LastModified.UTC()
	}
	return out.Body, obj, nil
}
func (s *Store) Head(ctx context.Context, key0 string) (ports.StoredObject, error) {
	key, err := JoinObjectKey(s.Config.Prefix, key0)
	if err != nil {
		return ports.StoredObject{}, err
	}
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)})
	if err != nil {
		return ports.StoredObject{}, classify(err)
	}
	obj := ports.StoredObject{Key: key0, Metadata: out.Metadata}
	if out.ContentLength != nil {
		obj.Size = *out.ContentLength
	}
	if out.ETag != nil {
		obj.ETag = strings.Trim(*out.ETag, "\"")
	}
	if out.LastModified != nil {
		obj.LastModified = out.LastModified.UTC()
	}
	return obj, nil
}
func (s *Store) List(ctx context.Context, req ports.ListObjectsRequest) (ports.ListObjectsResult, error) {
	prefix, err := JoinObjectKey(s.Config.Prefix, req.Prefix)
	if err != nil {
		return ports.ListObjectsResult{}, err
	}
	limit := int32(req.Limit)
	if limit <= 0 {
		limit = 1000
	}
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(s.Config.Bucket), Prefix: aws.String(prefix), ContinuationToken: strPtr(req.ContinuationToken), MaxKeys: aws.Int32(limit)})
	if err != nil {
		return ports.ListObjectsResult{}, classify(err)
	}
	objects := []ports.StoredObject{}
	for _, o := range out.Contents {
		k := strings.TrimPrefix(aws.ToString(o.Key), strings.Trim(s.Config.Prefix, "/")+"/")
		objects = append(objects, ports.StoredObject{Key: k, Size: aws.ToInt64(o.Size), LastModified: aws.ToTime(o.LastModified).UTC(), ETag: strings.Trim(aws.ToString(o.ETag), "\"")})
	}
	return ports.ListObjectsResult{Objects: objects, ContinuationToken: aws.ToString(out.NextContinuationToken), HasMore: aws.ToBool(out.IsTruncated)}, nil
}
func (s *Store) Delete(ctx context.Context, key0 string) error {
	key, err := JoinObjectKey(s.Config.Prefix, key0)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.Config.Bucket), Key: aws.String(key)})
	return classify(err)
}

func classify(err error) error {
	if err == nil {
		return nil
	}
	var nf *s3types.NotFound
	if errors.As(err, &nf) {
		return domain.NewError(domain.ErrChunkMissing, "object not found", err)
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		code := api.ErrorCode()
		switch code {
		case "NoSuchKey", "NotFound":
			return domain.NewError(domain.ErrChunkMissing, "object not found", err)
		case "NoSuchBucket":
			return domain.NewError(domain.ErrStorageUnavailable, "bucket not found", err)
		case "AccessDenied":
			return domain.NewError(domain.ErrStorageUnavailable, "permission denied", err)
		case "InvalidAccessKeyId", "SignatureDoesNotMatch":
			return domain.NewError(domain.ErrStorageUnavailable, "authentication failed", err)
		case "RequestTimeout":
			return domain.NewError(domain.ErrStorageUnavailable, "request timeout", err)
		case "SlowDown", "Throttling":
			return domain.NewError(domain.ErrStorageUnavailable, "storage throttled", err)
		case "PreconditionFailed":
			return domain.NewError(domain.ErrStorageUnavailable, "precondition failed", err)
		}
		return domain.NewError(domain.ErrStorageUnavailable, code+": "+api.ErrorMessage(), err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return domain.NewError(domain.ErrStorageUnavailable, "network error", err)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, http.ErrHandlerTimeout) {
		return domain.NewError(domain.ErrStorageUnavailable, "storage connection error", err)
	}
	return err
}
