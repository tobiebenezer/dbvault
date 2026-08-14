//go:build external_r2

package r2

import (
	"os"
	"testing"
)

func TestR2CredentialsPresent(t *testing.T) {
	for _, k := range []string{"DBVAULT_R2_ENDPOINT", "DBVAULT_R2_BUCKET", "DBVAULT_R2_ACCESS_KEY_ID", "DBVAULT_R2_SECRET_ACCESS_KEY"} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s is required for external_r2 tests", k)
		}
	}
}
