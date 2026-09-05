package productexperience

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/dbvault/dbvault/internal/adapters/warehouse"
)

// TestWarehouseSensitiveRuleMatrix locks the classification vocabulary every
// export masking decision depends on: pattern match per column family, empty
// classification for harmless columns.
func TestWarehouseSensitiveRuleMatrix(t *testing.T) {
	tests := []struct {
		column string
		want   string
	}{
		{"email", "rule-email"},
		{"Email", "rule-email"},
		{"contact_email_address", "rule-email"},
		{"password", "rule-password"},
		{"pass_hash", "rule-password"},
		{"api_secret", "rule-password"},
		{"phone", "rule-phone"},
		{"mobile_number", "rule-phone"},
		{"telephone", "rule-phone"},
		{"card_num", "rule-card"},
		{"cc_number", "rule-card"},
		{"cvv", "rule-card"},
		{"api_key", "rule-token"},
		{"auth_token", "rule-token"},
		{"jwt", "rule-token"},
		{"name", ""},
		{"id", ""},
		{"created_at", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := warehouseSensitiveRule(tt.column); got != tt.want {
			t.Fatalf("warehouseSensitiveRule(%q)=%q want %q", tt.column, got, tt.want)
		}
	}
}

// TestMaskWarehouseValueStrategies asserts each strategy's exact output,
// including determinism for the hash-based email and token strategies.
func TestMaskWarehouseValueStrategies(t *testing.T) {
	emailSum := sha256.Sum256([]byte("ada@example.test"))
	wantEmail := fmt.Sprintf("user_%s@anonymized.internal", hex.EncodeToString(emailSum[:4]))

	tokenSum := sha256.Sum256([]byte("tok-abc123"))
	wantToken := "tk_" + hex.EncodeToString(tokenSum[:4])

	tests := []struct {
		rule  string
		value string
		want  string
	}{
		{"rule-password", "hunter2", "[REDACTED]"},
		{"rule-card", "4111 1111 1111 1111", "[REDACTED]"},
		{"rule-email", "Ada@Example.test", wantEmail},
		{"rule-phone", "+1 (555) 867-5309", "+1 (555) 010-0000"},
		{"rule-token", "tok-abc123", wantToken},
		{"", "plain-value", "plain-value"},
	}
	for _, tt := range tests {
		if got := maskWarehouseValue(tt.rule, tt.value); got != tt.want {
			t.Fatalf("maskWarehouseValue(%q,%q)=%q want %q", tt.rule, tt.value, got, tt.want)
		}
	}
	// Determinism: same input, same masked output (stable across exports).
	if maskWarehouseValue("rule-email", "ada@example.test") != maskWarehouseValue("rule-email", "ada@example.test") {
		t.Fatal("email masking is not deterministic")
	}
}

// TestApplyWarehouseMaskingMatrix covers the applier end to end: nil input,
// untouched results with no sensitive columns, multi-column masking, nil-cell
// preservation and the guarantee that the original result is never mutated.
func TestApplyWarehouseMaskingMatrix(t *testing.T) {
	if masked, res := applyWarehouseMasking(nil); masked != nil || res != nil {
		t.Fatalf("nil result must pass through, got masked=%v res=%v", masked, res)
	}

	clean := &warehouse.QueryResult{
		Columns: []warehouse.ColumnMeta{{Name: "id"}, {Name: "name"}},
		Rows:    [][]any{{1, "Ada"}},
	}
	masked, res := applyWarehouseMasking(clean)
	if masked != nil || res != clean {
		t.Fatalf("result without sensitive columns must be untouched, masked=%v", masked)
	}

	emailSum := sha256.Sum256([]byte("ada@example.test"))
	wantEmail := fmt.Sprintf("user_%s@anonymized.internal", hex.EncodeToString(emailSum[:4]))
	original := &warehouse.QueryResult{
		Columns: []warehouse.ColumnMeta{{Name: "email"}, {Name: "password"}, {Name: "name"}},
		Rows: [][]any{
			{"ada@example.test", "hunter2", "Ada"},
			{nil, nil, "Bob"},
		},
	}
	cols, out := applyWarehouseMasking(original)
	if len(cols) != 2 || cols[0] != "email" || cols[1] != "password" {
		t.Fatalf("masked column list=%v want [email password] (sorted)", cols)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("row count=%d", len(out.Rows))
	}
	if out.Rows[0][0] != wantEmail || out.Rows[0][1] != "[REDACTED]" || out.Rows[0][2] != "Ada" {
		t.Fatalf("masked row=%v", out.Rows[0])
	}
	if out.Rows[1][0] != nil || out.Rows[1][1] != nil || out.Rows[1][2] != "Bob" {
		t.Fatalf("nil cells must stay nil, plain cells untouched: %v", out.Rows[1])
	}
	// The original result is never mutated.
	if original.Rows[0][0] != "ada@example.test" || original.Rows[0][1] != "hunter2" {
		t.Fatalf("applyWarehouseMasking mutated its input: %v", original.Rows[0])
	}
}
