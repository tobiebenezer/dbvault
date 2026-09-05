package s3

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbvault/dbvault/internal/ports"
)

func TestJoinObjectKey(t *testing.T) {
	testCases := []struct {
		name        string
		prefix      string
		parts       []string
		expected    string
		expectError bool
	}{
		{
			name:     "BasicJoin",
			prefix:   "repo1",
			parts:    []string{"snapshots", "snap_01", "manifest.json"},
			expected: "repo1/snapshots/snap_01/manifest.json",
		},
		{
			name:     "EmptyPrefix",
			prefix:   "",
			parts:    []string{"chunks", "ab", "cd", "abcd.dvchunk"},
			expected: "chunks/ab/cd/abcd.dvchunk",
		},
		{
			name:     "PrefixWithLeadingAndTrailingSlashes",
			prefix:   "/my-prefix/",
			parts:    []string{"sub/path", "file.txt"},
			expected: "my-prefix/sub/path/file.txt",
		},
		{
			name:        "PathTraversalAttackDotDot",
			prefix:      "repo1",
			parts:       []string{"../../etc/passwd"},
			expectError: true,
		},
		{
			name:        "PathTraversalInMiddle",
			prefix:      "repo1",
			parts:       []string{"snapshots", "..", "secrets.json"},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := JoinObjectKey(tc.prefix, tc.parts...)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for path %v, got %s", tc.parts, res)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if res != tc.expected {
					t.Fatalf("expected %s, got %s", tc.expected, res)
				}
			}
		})
	}
}

func TestBuiltinProfiles(t *testing.T) {
	profiles := []struct {
		name           string
		expectedName   string
		expectedRegion string
		pathStyle      bool
	}{
		{"cloudflare-r2", "cloudflare-r2", "auto", true},
		{"r2", "cloudflare-r2", "auto", true},
		{"minio", "minio", "us-east-1", true},
		{"contabo", "contabo", "default", true},
		{"wasabi", "wasabi", "us-east-1", true},
		{"generic-s3", "generic-s3", "us-east-1", true},
	}

	for _, p := range profiles {
		t.Run(p.name, func(t *testing.T) {
			prof := BuiltinProfile(p.name)
			if prof.Name != p.expectedName {
				t.Fatalf("expected profile name %s, got %s", p.expectedName, prof.Name)
			}
			if prof.DefaultRegion != p.expectedRegion {
				t.Fatalf("expected region %s, got %s", p.expectedRegion, prof.DefaultRegion)
			}
			if prof.ForcePathStyle != p.pathStyle {
				t.Fatalf("expected path style %v, got %v", p.pathStyle, prof.ForcePathStyle)
			}
			if !prof.Capabilities.MultipartUpload {
				t.Fatal("expected MultipartUpload capability enabled")
			}
		})
	}
}

func TestReadSecret(t *testing.T) {
	// 1. Direct string
	s, err := readSecret("my-raw-secret")
	if err != nil || s != "my-raw-secret" {
		t.Fatalf("unexpected direct secret read: %s, %v", s, err)
	}

	// 2. Environment variable
	os.Setenv("TEST_SECRET_ENV_KEY", "env-secret-val")
	defer os.Unsetenv("TEST_SECRET_ENV_KEY")
	s, err = readSecret("env:TEST_SECRET_ENV_KEY")
	if err != nil || s != "env-secret-val" {
		t.Fatalf("unexpected env secret read: %s, %v", s, err)
	}

	// 3. File
	tmpFile := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(tmpFile, []byte("file-secret-val\n"), 0600)
	s, err = readSecret("file:" + tmpFile)
	if err != nil || s != "file-secret-val" {
		t.Fatalf("unexpected file secret read: %s, %v", s, err)
	}
}

func TestObjectLockRequestFields(t *testing.T) {
	expiry := time.Now().Add(48 * time.Hour)
	req := ports.PutObjectRequest{
		Key:         "snapshots/snap1/data.dbv",
		IfNotExists: true,
		LockRetention: &ports.ObjectLockRetention{
			Mode:            "COMPLIANCE",
			RetainUntilDate: expiry,
		},
		LegalHold: true,
		Metadata: map[string]string{
			"dbvault-source": "pg_main",
		},
	}

	if req.LockRetention.Mode != "COMPLIANCE" {
		t.Fatalf("expected retention mode COMPLIANCE, got %s", req.LockRetention.Mode)
	}
	if req.LockRetention.RetainUntilDate != expiry {
		t.Fatalf("expected retention date %v, got %v", expiry, req.LockRetention.RetainUntilDate)
	}
	if !req.LegalHold {
		t.Fatal("expected LegalHold true")
	}
}
