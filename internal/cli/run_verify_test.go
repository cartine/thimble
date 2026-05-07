package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVerifyMatchesAfterInit covers K-22: a freshly initialized
// namespace's bundle SHA matches the manifest, the runVerify handler
// returns nil, and the report is rendered to stdout.
func TestVerifyMatchesAfterInit(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	var stdout, stderr strings.Builder
	if err := runVerify(st, []string{"svc", "prod"}, &stdout, &stderr); err != nil {
		t.Fatalf("verify: %v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "verdict:      match") {
		t.Fatalf("verify output missing match verdict: %q", out)
	}
	if !strings.Contains(out, "age1") {
		t.Fatalf("verify output missing recipient prefix: %q", out)
	}
}

// TestVerifyDetectsTamperViaCLI covers K-22: a tampered bundle is
// reported with verdict MISMATCH and runVerify returns a non-nil
// error so the CLI exits non-zero.
func TestVerifyDetectsTamperViaCLI(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	bundlePath := filepath.Join(st.Root(), "svc", "prod.env.age")
	b, err := os.ReadFile(bundlePath) // #nosec G304 -- test-only path.
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if err := os.WriteFile(bundlePath, append(b, 'Z'), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	var stdout, stderr strings.Builder
	err = runVerify(st, []string{"svc", "prod"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("verify after tamper succeeded; want non-zero exit")
	}
	if !strings.Contains(err.Error(), "MISMATCH") {
		t.Fatalf("err = %v, want SHA-256 MISMATCH", err)
	}
	if !strings.Contains(stdout.String(), "MISMATCH") {
		t.Fatalf("stdout missing MISMATCH verdict: %q", stdout.String())
	}
}
