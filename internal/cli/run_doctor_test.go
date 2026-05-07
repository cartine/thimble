package cli

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/doctor"
)

// TestRunDoctorTabularPasses covers K-29: a healthy setup runs to
// completion and the tabular output mentions every check name.
func TestRunDoctorTabularPasses(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	tool := age.New("age", "")
	addr := freeAddr(t)
	var stdout, stderr strings.Builder
	cfg := cliConfig{}
	// runDoctor returns an error when any check fails; we don't
	// require the age binary on test PATH, so failures here are
	// expected. We only assert that every check name appears in
	// the report.
	_ = runDoctor(
		context.Background(), st, tool,
		[]string{"--addr", addr},
		&stdout, &stderr, cfg,
	)
	out := stdout.String()
	for _, name := range []string{"age", "identity", "store", "manifest", "bundles", "web port"} {
		if !strings.Contains(out, name) {
			t.Fatalf("doctor output missing %q: %q", name, out)
		}
	}
}

// TestRunDoctorJSONEmitsValidJSON covers --json: the output parses
// as a doctor.Report.
func TestRunDoctorJSONEmitsValidJSON(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	tool := age.New("age", "")
	addr := freeAddr(t)
	var stdout, stderr strings.Builder
	cfg := cliConfig{}
	// runDoctor may return a non-nil error in tests without a real
	// age binary on PATH; that's expected. The JSON output is
	// produced regardless so we can decode and check structure.
	_ = runDoctor(
		context.Background(), st, tool,
		[]string{"--json", "--addr", addr},
		&stdout, &stderr, cfg,
	)
	var report doctor.Report
	if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
		t.Fatalf("decode json: %v\noutput=%s", err, stdout.String())
	}
	if len(report.Checks) == 0 {
		t.Fatalf("no checks in JSON report: %s", stdout.String())
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()
	return addr
}
