//go:build !restricted

package e2e

import (
	"context"
	"testing"
	"time"
)

// TestE2ESuiteSanity verifies the overall test environment sanity and test harness integrity.
func TestE2ESuiteSanity(t *testing.T) {
	env := NewTestEnv(t)
	if env.TempDir == "" {
		t.Fatal("expected non-empty temp directory")
	}
	if len(env.MasterKey) != 32 {
		t.Fatalf("expected 32-byte master key, got %d", len(env.MasterKey))
	}
	if env.Signer.KeyID == "" {
		t.Fatal("expected non-empty signer key ID")
	}

	caps, err := env.Store.Capabilities(context.Background())
	if err != nil {
		t.Fatalf("failed to query storage capabilities: %v", err)
	}
	if !caps.ConditionalCreate || !caps.UserMetadata {
		t.Fatalf("expected storage capabilities conditional_create and user_metadata, got %+v", caps)
	}
}

// TestE2ESuiteTimeoutPolicy ensures test context propagation behaves properly.
func TestE2ESuiteTimeoutPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("context timed out prematurely")
	case <-time.After(10 * time.Millisecond):
		// Expected behavior
	}
}
