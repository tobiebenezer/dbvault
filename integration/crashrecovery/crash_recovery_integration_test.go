//go:build integration

package crashrecovery

import "testing"

func TestCrashRecoveryFaultMatrixPlaceholder(t *testing.T) {
	t.Skip("requires process restart harness and fault injector")
}
