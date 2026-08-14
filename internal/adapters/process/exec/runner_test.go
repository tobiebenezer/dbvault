package exec

import "testing"

func TestSafeArgsRedactsSecrets(t *testing.T) {
	got := SafeArgs([]string{"--user", "dbvault", "--password=secret"}, []string{"secret"})
	if got == "--user dbvault --password=secret" {
		t.Fatal("secret was not redacted")
	}
}
