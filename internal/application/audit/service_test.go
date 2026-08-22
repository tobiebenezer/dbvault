package audit

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAuditServiceRecordAndVerify(t *testing.T) {
	svc := New()

	ctx := context.Background()
	e1 := svc.Record(ctx, "backup.completed", "system", "cron", "database", "pg-prod", "success", map[string]string{"size": "1000"})
	if e1.EventHash == "" {
		t.Fatalf("expected event hash to be computed")
	}

	e2 := svc.Record(ctx, "restore_drill.passed", "operator", "admin@corp", "sandbox", "drill-01", "passed", nil)
	if e2.PreviousHash != e1.EventHash {
		t.Fatalf("expected e2 previous hash to match e1 event hash")
	}

	valid, err := svc.VerifyIntegrity()
	if err != nil || !valid {
		t.Fatalf("expected audit chain to be valid, got valid=%v, err=%v", valid, err)
	}

	list := svc.List(10, "")
	if len(list) != 2 {
		t.Fatalf("expected 2 events, got %d", len(list))
	}
}

func TestAuditServiceTamperDetection(t *testing.T) {
	svc := New()
	ctx := context.Background()

	svc.Record(ctx, "backup.completed", "system", "cron", "database", "pg-prod", "success", nil)
	svc.Record(ctx, "approval.granted", "approver", "lead@corp", "database", "pg-prod", "approved", nil)

	// Tamper with the internal event in the chain
	svc.events[0].Outcome = "failed"

	valid, err := svc.VerifyIntegrity()
	if valid || err == nil {
		t.Fatalf("expected tamper detection error, got valid=%v, err=%v", valid, err)
	}
}

func TestAuditServiceExportCSV(t *testing.T) {
	svc := New()
	ctx := context.Background()

	svc.Record(ctx, "setup.completed", "admin", "setup", "appliance", "node-1", "configured", nil)

	var buf bytes.Buffer
	if err := svc.ExportCSV(&buf); err != nil {
		t.Fatalf("failed to export CSV: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "setup.completed") || !strings.Contains(out, "node-1") {
		t.Fatalf("expected CSV output to contain event data, got:\n%s", out)
	}
}
