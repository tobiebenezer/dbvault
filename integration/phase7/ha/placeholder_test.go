//go:build integration
package ha
import "testing"
func TestPhase7HAEnvironment(t *testing.T) { t.Skip("run multiple controller replicas with leader election") }
