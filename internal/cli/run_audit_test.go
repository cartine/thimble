package cli

import (
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/audit"
)

// TestRunAuditPrintsRecentEvents covers K-27 #4: `thimble audit
// <app> <env>` reads the on-disk log, filters to the requested
// namespace, and pretty-prints in tabular form.
func TestRunAuditPrintsRecentEvents(t *testing.T) {
	st := newTestStore(t)
	st.SetAuditLogger(audit.New(st.Root(), nil))
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "K1", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Prepare an event in another namespace; it should be filtered out.
	if err := st.Init("other", "dev", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init other: %v", err)
	}
	var stdout, stderr strings.Builder
	if err := runAudit(st, []string{"svc", "prod"}, &stdout, &stderr); err != nil {
		t.Fatalf("audit: %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "init") {
		t.Fatalf("audit output missing init: %q", out)
	}
	if !strings.Contains(out, "set") {
		t.Fatalf("audit output missing set: %q", out)
	}
	if !strings.Contains(out, "K1") {
		t.Fatalf("audit output missing subject K1: %q", out)
	}
	if strings.Contains(out, "other") {
		t.Fatalf("audit output bled into other namespace: %q", out)
	}
}

// TestRunAuditLimitClipsHead covers --limit: only the most recent
// limit events are printed; older entries are dropped.
func TestRunAuditLimitClipsHead(t *testing.T) {
	st := newTestStore(t)
	st.SetAuditLogger(audit.New(st.Root(), nil))
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	for i, key := range []string{"K1", "K2", "K3", "K4"} {
		if err := st.SetSecret("svc", "prod", key, "v"); err != nil {
			t.Fatalf("set %d: %v", i, err)
		}
	}
	var stdout, stderr strings.Builder
	if err := runAudit(st, []string{"--limit", "2", "svc", "prod"}, &stdout, &stderr); err != nil {
		t.Fatalf("audit: %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "K3") || !strings.Contains(out, "K4") {
		t.Fatalf("missing tail entries: %q", out)
	}
	if strings.Contains(out, "K1") || strings.Contains(out, "K2") {
		t.Fatalf("--limit did not drop older entries: %q", out)
	}
}
