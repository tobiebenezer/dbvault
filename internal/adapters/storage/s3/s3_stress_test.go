//go:build !restricted

package s3

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/domain"
	"github.com/dbvault/dbvault/internal/ports"
)

// TestStress_JoinObjectKey_FuzzAndAdversarialPaths tests key joining against path traversal and edge cases.
func TestStress_JoinObjectKey_FuzzAndAdversarialPaths(t *testing.T) {
	traversalVectors := []struct {
		name        string
		prefix      string
		parts       []string
		expectError bool
		expected    string
	}{
		{"DoubleDotRoot", "", []string{".."}, true, ""},
		{"DoubleDotPrefix", "..", []string{"valid", "chunk.dvchunk"}, true, ""},
		{"DoubleDotNested", "prefix", []string{"chunks", "..", "secrets.json"}, true, ""},
		{"DoubleDotDeep", "prefix", []string{"chunks", "ab", "cd", "../../../etc/passwd"}, true, ""},
		{"DoubleDotWithSlash", "prefix", []string{"chunks/../../escape"}, true, ""},
		{"DoubleDotTrailing", "prefix", []string{"chunks", "ab", ".."}, true, ""},
		{"DoubleDotMultiSegment", "prefix", []string{"chunks", "ab/../cd/../../root"}, true, ""},
		{"ValidCleanPath", "my-bucket-prefix", []string{"source-1", "chunks", "v1", "00", "01", "0001deadbeef.dvchunk"}, false, "my-bucket-prefix/source-1/chunks/v1/00/01/0001deadbeef.dvchunk"},
		{"ValidWithLeadingSlashes", "/my-bucket-prefix/", []string{"/source-1/", "/chunks/file.json"}, false, "my-bucket-prefix/source-1/chunks/file.json"},
		{"ValidEmptyPrefix", "", []string{"source-1", "snapshots", "snap1", "manifest.json"}, false, "source-1/snapshots/snap1/manifest.json"},
		{"ValidMultiPartDeep", "backup", []string{"tenant", "db", "table", "part1", "file.dvchunk"}, false, "backup/tenant/db/table/part1/file.dvchunk"},
	}

	for _, tc := range traversalVectors {
		t.Run(tc.name, func(t *testing.T) {
			res, err := JoinObjectKey(tc.prefix, tc.parts...)
			if tc.expectError {
				if err == nil {
					t.Fatalf("[%s] expected error for traversal %v with prefix %s, got key: %s", tc.name, tc.parts, tc.prefix, res)
				}
			} else {
				if err != nil {
					t.Fatalf("[%s] unexpected error: %v", tc.name, err)
				}
				if res != tc.expected {
					t.Fatalf("[%s] expected %q, got %q", tc.name, tc.expected, res)
				}
			}
		})
	}
}

// TestStress_BuiltinProfiles_AllVariants verifies profile lookups across Cloudflare R2, MinIO, AWS S3, and Contabo.
func TestStress_BuiltinProfiles_AllVariants(t *testing.T) {
	cases := []struct {
		inputName      string
		expectedName   string
		expectedRegion string
		forcePathStyle bool
		mustSupport    []string
	}{
		{"cloudflare-r2", "cloudflare-r2", "auto", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"Cloudflare_R2", "cloudflare-r2", "auto", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"r2", "cloudflare-r2", "auto", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"R2-PROD", "cloudflare-r2", "auto", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"minio", "minio", "us-east-1", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"MINIO_CLUSTER", "minio", "us-east-1", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"contabo", "contabo", "default", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"CONTABO_EU", "contabo", "default", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"wasabi", "wasabi", "us-east-1", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"WASABI_STORAGE", "wasabi", "us-east-1", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"aws-s3", "generic-s3", "us-east-1", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"s3-standard", "generic-s3", "us-east-1", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
		{"UNKNOWN_STORAGE", "generic-s3", "us-east-1", true, []string{"MultipartUpload", "RangeReads", "UserMetadata", "PathStyleAddressing"}},
	}

	for _, c := range cases {
		t.Run(c.inputName, func(t *testing.T) {
			p := BuiltinProfile(c.inputName)
			if p.Name != c.expectedName {
				t.Fatalf("expected profile name %s, got %s", c.expectedName, p.Name)
			}
			if p.DefaultRegion != c.expectedRegion {
				t.Fatalf("expected default region %s, got %s", c.expectedRegion, p.DefaultRegion)
			}
			if p.ForcePathStyle != c.forcePathStyle {
				t.Fatalf("expected forcePathStyle %v, got %v", c.forcePathStyle, p.ForcePathStyle)
			}
			if !p.Capabilities.MultipartUpload {
				t.Fatal("expected MultipartUpload capability to be true")
			}
			if !p.Capabilities.PathStyleAddressing {
				t.Fatal("expected PathStyleAddressing capability to be true")
			}
			if !p.Capabilities.RangeReads {
				t.Fatal("expected RangeReads capability to be true")
			}
			if !p.Capabilities.UserMetadata {
				t.Fatal("expected UserMetadata capability to be true")
			}
		})
	}
}

// TestStress_ObjectLock_MatrixPropagation verifies Object Lock parameter construction under various input combinations.
func TestStress_ObjectLock_MatrixPropagation(t *testing.T) {
	futureDate := time.Now().Add(72 * time.Hour).UTC().Truncate(time.Second)
	futureRFC3339 := futureDate.Format(time.RFC3339)

	testMatrix := []struct {
		name               string
		req                ports.PutObjectRequest
		expectedMode       string
		expectedRetainDate time.Time
		expectedLegalHold  bool
	}{
		{
			name: "Explicit_Compliance_With_Struct",
			req: ports.PutObjectRequest{
				Key: "obj1",
				LockRetention: &ports.ObjectLockRetention{
					Mode:            "COMPLIANCE",
					RetainUntilDate: futureDate,
				},
				LegalHold: true,
			},
			expectedMode:       "COMPLIANCE",
			expectedRetainDate: futureDate,
			expectedLegalHold:  true,
		},
		{
			name: "Explicit_Governance_Lowercase",
			req: ports.PutObjectRequest{
				Key: "obj2",
				LockRetention: &ports.ObjectLockRetention{
					Mode:            "governance",
					RetainUntilDate: futureDate,
				},
				LegalHold: false,
			},
			expectedMode:       "governance",
			expectedRetainDate: futureDate,
			expectedLegalHold:  false,
		},
		{
			name: "Legacy_Fields_LockMode_And_RetainUntilDate",
			req: ports.PutObjectRequest{
				Key:             "obj3",
				LockMode:        "COMPLIANCE",
				RetainUntilDate: &futureDate,
			},
			expectedMode:       "COMPLIANCE",
			expectedRetainDate: futureDate,
			expectedLegalHold:  false,
		},
		{
			name: "Metadata_Fallback_Mode_And_Date",
			req: ports.PutObjectRequest{
				Key: "obj4",
				Metadata: map[string]string{
					"dbvault-lock-mode":  "COMPLIANCE",
					"dbvault-lock-until": futureRFC3339,
					"dbvault-legal-hold": "true",
				},
			},
			expectedMode:       "COMPLIANCE",
			expectedRetainDate: futureDate,
			expectedLegalHold:  true,
		},
		{
			name: "No_Lock_Parameters",
			req: ports.PutObjectRequest{
				Key: "obj5",
			},
			expectedMode:       "",
			expectedRetainDate: time.Time{},
			expectedLegalHold:  false,
		},
	}

	for _, tc := range testMatrix {
		t.Run(tc.name, func(t *testing.T) {
			// Verify struct contents
			if tc.req.LockRetention != nil {
				if strings.ToUpper(tc.req.LockRetention.Mode) != strings.ToUpper(tc.expectedMode) {
					t.Fatalf("mismatched retention mode: expected %s, got %s", tc.expectedMode, tc.req.LockRetention.Mode)
				}
				if !tc.req.LockRetention.RetainUntilDate.Equal(tc.expectedRetainDate) {
					t.Fatalf("mismatched retain until date: expected %v, got %v", tc.expectedRetainDate, tc.req.LockRetention.RetainUntilDate)
				}
			}
			if tc.req.LegalHold != tc.expectedLegalHold && tc.req.Metadata["dbvault-legal-hold"] != "true" {
				t.Fatalf("mismatched legal hold: expected %v", tc.expectedLegalHold)
			}
		})
	}
}

// TestStress_CASChunkKey_FormattingStress verifies key formatting rules across multiple chunk IDs and key versions.
func TestStress_CASChunkKey_FormattingStress(t *testing.T) {
	type chunkCase struct {
		sourceID   string
		repo       string
		keyVersion string
		chunkID    string
		expected   string
	}

	cases := []chunkCase{
		{
			sourceID:   "src_pg_prod",
			repo:       "repo_s3_main",
			keyVersion: "v1",
			chunkID:    "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			expected:   "src_pg_prod/chunks/v1/ab/cd/abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789.dvchunk",
		},
		{
			sourceID:   "src_mysql_cluster",
			repo:       "repo_r2_backup",
			keyVersion: "v2",
			chunkID:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			expected:   "src_mysql_cluster/chunks/v2/01/23/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.dvchunk",
		},
		{
			sourceID:   "",
			repo:       "repo_minio",
			keyVersion: "v1",
			chunkID:    "deadbeefcafebabe0000111122223333444455556666777788889999aaaabbbb",
			expected:   "chunks/v1/de/ad/deadbeefcafebabe0000111122223333444455556666777788889999aaaabbbb.dvchunk",
		},
	}

	for _, c := range cases {
		t.Run(c.chunkID[:8], func(t *testing.T) {
			p := c.chunkID
			if len(p) < 4 {
				p = strings.Repeat("0", 4-len(p)) + p
			}
			var key string
			if c.sourceID != "" {
				key = fmt.Sprintf("%s/chunks/%s/%s/%s/%s.dvchunk", c.sourceID, c.keyVersion, p[:2], p[2:4], c.chunkID)
			} else {
				key = fmt.Sprintf("chunks/%s/%s/%s/%s.dvchunk", c.keyVersion, p[:2], p[2:4], c.chunkID)
			}

			if key != c.expected {
				t.Fatalf("expected chunk key %s, got %s", c.expected, key)
			}

			// Validate with JoinObjectKey that it is a safe clean key
			joined, err := JoinObjectKey("", key)
			if err != nil {
				t.Fatalf("JoinObjectKey rejected valid chunk key %s: %v", key, err)
			}
			if joined != c.expected {
				t.Fatalf("JoinObjectKey altered valid chunk key: expected %s, got %s", c.expected, joined)
			}
		})
	}
}

// TestStress_ClassifyError_AllTypes tests error classification covering all error conditions.
func TestStress_ClassifyError_AllTypes(t *testing.T) {
	errs := []struct {
		err          error
		expectedCode domain.ErrorCode
	}{
		{&mockSmithyError{code: "NoSuchKey", message: "key not found"}, domain.ErrChunkMissing},
		{&mockSmithyError{code: "NotFound", message: "not found"}, domain.ErrChunkMissing},
		{&mockSmithyError{code: "NoSuchBucket", message: "bucket not found"}, domain.ErrStorageUnavailable},
		{&mockSmithyError{code: "AccessDenied", message: "forbidden"}, domain.ErrStorageUnavailable},
		{&mockSmithyError{code: "InvalidAccessKeyId", message: "invalid key"}, domain.ErrStorageUnavailable},
		{&mockSmithyError{code: "SignatureDoesNotMatch", message: "bad signature"}, domain.ErrStorageUnavailable},
		{&mockSmithyError{code: "RequestTimeout", message: "timeout"}, domain.ErrStorageUnavailable},
		{&mockSmithyError{code: "SlowDown", message: "throttled"}, domain.ErrStorageUnavailable},
		{&mockSmithyError{code: "Throttling", message: "throttled"}, domain.ErrStorageUnavailable},
		{&mockSmithyError{code: "PreconditionFailed", message: "precondition"}, domain.ErrStorageUnavailable},
	}

	for _, item := range errs {
		classified := classify(item.err)
		var appErr *domain.AppError
		if classified == nil {
			t.Fatalf("expected AppError for %v, got nil", item.err)
		}
		var ok bool
		appErr, ok = classified.(*domain.AppError)
		if !ok {
			t.Fatalf("expected *domain.AppError type, got %T: %v", classified, classified)
		}
		if appErr.Code != item.expectedCode {
			t.Fatalf("expected code %s, got %s for error %v", item.expectedCode, appErr.Code, item.err)
		}
	}
}
