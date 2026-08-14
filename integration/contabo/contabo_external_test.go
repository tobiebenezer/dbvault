//go:build external_contabo

package contabo

import (
	"os"
	"testing"
)

func TestContaboCredentialsPresent(t *testing.T) {
	for _, k := range []string{"DBVAULT_CONTABO_ENDPOINT", "DBVAULT_CONTABO_BUCKET", "DBVAULT_CONTABO_ACCESS_KEY_ID", "DBVAULT_CONTABO_SECRET_ACCESS_KEY"} {
		if os.Getenv(k) == "" {
			t.Fatalf("%s is required for external_contabo tests", k)
		}
	}
}
