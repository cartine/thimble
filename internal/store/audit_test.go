package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/audit"
)

// TestStoreEmitsAuditEvents covers K-27 #1: every mutating Store
// call appends an entry to the audit log with the right Op, App,
// Env, and Subject (key for secret ops, recipient thumbprint for
// recipient ops). Recipient string itself never appears in the log.
func TestStoreEmitsAuditEvents(t *testing.T) {
	st := newTestStore(t)
	st.SetAuditLogger(audit.New(st.Root(), nil))

	if err := st.Init("svc", "prod", []string{testRecipientAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "TOKEN", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := st.UpdateSecret("svc", "prod", "TOKEN", "v2"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := st.CreateSecret("svc", "prod", "OTHER", "x"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.DeleteSecret("svc", "prod", "OTHER"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := st.AddRecipient("svc", "prod", testRecipientBob); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := st.RemoveRecipient("svc", "prod", testRecipientBob); err != nil {
		t.Fatalf("remove: %v", err)
	}

	events, err := audit.Read(st.Root())
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	wantOps := []string{
		audit.OpInit, audit.OpSet, audit.OpUpdate, audit.OpCreate,
		audit.OpDelete, audit.OpRecipientAdd, audit.OpRecipientRemove,
	}
	if len(events) != len(wantOps) {
		t.Fatalf("got %d audit events, want %d: %+v", len(events), len(wantOps), events)
	}
	for i, ev := range events {
		if ev.Op != wantOps[i] {
			t.Fatalf("event[%d].Op = %q, want %q", i, ev.Op, wantOps[i])
		}
		if ev.App != "svc" || ev.Env != "prod" {
			t.Fatalf("event[%d] namespace = %s/%s; want svc/prod",
				i, ev.App, ev.Env)
		}
		if ev.Operator == "" {
			t.Fatalf("event[%d] missing operator", i)
		}
	}
	wantBobThumb := audit.Thumbprint(testRecipientBob)
	if events[5].Subject != wantBobThumb {
		t.Fatalf("recipient_add subject = %q, want %q", events[5].Subject, wantBobThumb)
	}
	if events[6].Subject != wantBobThumb {
		t.Fatalf("recipient_remove subject = %q, want %q", events[6].Subject, wantBobThumb)
	}

	logRaw, err := readAuditLog(st.Root())
	if err != nil {
		t.Fatalf("read raw log: %v", err)
	}
	if strings.Contains(logRaw, testRecipientBob) {
		t.Fatalf("audit log contains full recipient: %q", logRaw)
	}
	if strings.Contains(logRaw, testRecipientAlice) {
		t.Fatalf("audit log contains full recipient: %q", logRaw)
	}
}

func readAuditLog(root string) (string, error) {
	// #nosec G304 -- root is t.TempDir() controlled by the test.
	b, err := os.ReadFile(filepath.Join(root, audit.LogFileName))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
