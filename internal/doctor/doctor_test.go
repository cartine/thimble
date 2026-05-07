package doctor_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/doctor"
	"github.com/cartine/thimble/internal/store"
)

const testRecipient = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"

// TestRunHealthySetup covers K-29's happy path: a fresh init in a
// proper 0700 store with a 0600 identity and all bundles intact
// produces no failing checks.
func TestRunHealthySetup(t *testing.T) {
	st, idPath, fakeAge, port := buildHealthySetup(t)
	tool := age.New(fakeAge, idPath)
	report := doctor.Run(context.Background(), st, tool, doctor.Options{
		WebAddr:      port,
		IdentityPath: idPath,
	})
	if report.HasFailures() {
		t.Fatalf("healthy setup had failures: %+v", report.Checks)
	}
	if !findCheck(t, report, "age").Status.IsOK() {
		t.Fatalf("age status not ok: %v", findCheck(t, report, "age"))
	}
	if !findCheck(t, report, "identity").Status.IsOK() {
		t.Fatalf("identity not ok")
	}
	if !findCheck(t, report, "store").Status.IsOK() {
		t.Fatalf("store not ok")
	}
}

// TestRunMissingIdentity covers K-29 #5 deliberately-broken setup:
// no identity configured -> warn, not fail.
func TestRunMissingIdentity(t *testing.T) {
	st, _, fakeAge, port := buildHealthySetup(t)
	tool := age.New(fakeAge, "")
	report := doctor.Run(context.Background(), st, tool, doctor.Options{
		WebAddr: port,
	})
	c := findCheck(t, report, "identity")
	if c.Status != doctor.StatusWarn {
		t.Fatalf("identity status = %s, want warn", c.Status)
	}
}

// TestRunWorldReadableIdentity covers K-29 #5: a 0644 identity file
// is rejected as fail with a chmod hint.
func TestRunWorldReadableIdentity(t *testing.T) {
	st, idPath, fakeAge, port := buildHealthySetup(t)
	if err := os.Chmod(idPath, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	tool := age.New(fakeAge, idPath)
	report := doctor.Run(context.Background(), st, tool, doctor.Options{
		WebAddr:      port,
		IdentityPath: idPath,
	})
	c := findCheck(t, report, "identity")
	if c.Status != doctor.StatusFail {
		t.Fatalf("identity status = %s, want fail", c.Status)
	}
	if !strings.Contains(c.Detail, "chmod 0600") {
		t.Fatalf("missing chmod hint: %q", c.Detail)
	}
}

// TestRunMissingBundle covers K-29 #5: a manifest entry whose
// ciphertext was removed produces a manifest-level fail.
func TestRunMissingBundle(t *testing.T) {
	st, idPath, fakeAge, port := buildHealthySetup(t)
	bundlePath := filepath.Join(st.Root(), "svc", "prod.env.age")
	if err := os.Remove(bundlePath); err != nil {
		t.Fatalf("remove bundle: %v", err)
	}
	tool := age.New(fakeAge, idPath)
	report := doctor.Run(context.Background(), st, tool, doctor.Options{
		WebAddr:      port,
		IdentityPath: idPath,
	})
	c := findCheck(t, report, "manifest")
	if c.Status != doctor.StatusFail {
		t.Fatalf("manifest status = %s, want fail", c.Status)
	}
	if !strings.Contains(c.Detail, "missing") {
		t.Fatalf("manifest detail missing 'missing': %q", c.Detail)
	}
}

// TestRunMismatchedBundleSHA covers K-29 #5: a tampered ciphertext
// produces a per-namespace bundles fail.
func TestRunMismatchedBundleSHA(t *testing.T) {
	st, idPath, fakeAge, port := buildHealthySetup(t)
	bundlePath := filepath.Join(st.Root(), "svc", "prod.env.age")
	b, err := os.ReadFile(bundlePath) // #nosec G304 -- test path.
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if err := os.WriteFile(bundlePath, append(b, 'X'), 0o600); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	tool := age.New(fakeAge, idPath)
	report := doctor.Run(context.Background(), st, tool, doctor.Options{
		WebAddr:      port,
		IdentityPath: idPath,
	})
	c := findCheck(t, report, "bundles")
	if c.Status != doctor.StatusFail {
		t.Fatalf("bundles status = %s, want fail", c.Status)
	}
	if !strings.Contains(c.Detail, "MISMATCH") {
		t.Fatalf("bundles detail missing MISMATCH: %q", c.Detail)
	}
}

// TestRunWebPortInUse covers K-29 #5: a TCP-bound port produces a
// warn (not fail) on the web port check.
func TestRunWebPortInUse(t *testing.T) {
	st, idPath, fakeAge, _ := buildHealthySetup(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	addr := listener.Addr().String()
	tool := age.New(fakeAge, idPath)
	report := doctor.Run(context.Background(), st, tool, doctor.Options{
		WebAddr:      addr,
		IdentityPath: idPath,
	})
	c := findCheck(t, report, "web port")
	if c.Status != doctor.StatusWarn {
		t.Fatalf("web port status = %s, want warn (in-use)", c.Status)
	}
	if !strings.Contains(c.Detail, "in use") {
		t.Fatalf("web port detail missing 'in use': %q", c.Detail)
	}
}

// TestRunRecipientsListed covers K-29 #5: a fresh init has its
// recipient summarized with prefix and thumbprint.
func TestRunRecipientsListed(t *testing.T) {
	st, idPath, fakeAge, port := buildHealthySetup(t)
	tool := age.New(fakeAge, idPath)
	report := doctor.Run(context.Background(), st, tool, doctor.Options{
		WebAddr:      port,
		IdentityPath: idPath,
	})
	var hit bool
	for _, c := range report.Checks {
		if !strings.HasPrefix(c.Name, "recipients[svc/prod]") {
			continue
		}
		if c.Status != doctor.StatusOK {
			t.Fatalf("recipients status = %s, want ok", c.Status)
		}
		if !strings.Contains(c.Detail, "age1(") {
			t.Fatalf("recipient detail missing age1 prefix: %q", c.Detail)
		}
		if strings.Contains(c.Detail, testRecipient) {
			t.Fatalf("recipient detail leaks full recipient: %q", c.Detail)
		}
		hit = true
	}
	if !hit {
		t.Fatalf("no per-namespace recipients check")
	}
}

// TestRunReportHasFailures covers HasFailures helper.
func TestRunReportHasFailures(t *testing.T) {
	r := doctor.Report{Checks: []doctor.CheckResult{
		{Name: "x", Status: doctor.StatusOK},
	}}
	if r.HasFailures() {
		t.Fatalf("report with no fails reported failures")
	}
	r.Checks = append(r.Checks, doctor.CheckResult{
		Name: "y", Status: doctor.StatusFail,
	})
	if !r.HasFailures() {
		t.Fatalf("report with fail did not report failures")
	}
}

// buildHealthySetup constructs a Store + identity + fake age binary
// in a temp dir, with one initialized namespace. Returns the Store,
// the identity file path, the fake-age path, and a free TCP port we
// can probe for the web check (returned as host:port).
func buildHealthySetup(t *testing.T) (*store.Store, string, string, string) {
	t.Helper()
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	idPath := writeIdentity(t, root)
	if err := os.Mkdir(filepath.Join(root, "secrets"), 0o700); err != nil {
		t.Fatalf("mkdir secrets: %v", err)
	}
	st := store.New(filepath.Join(root, "secrets"), idPath)
	st.SetAge(age.New(fakeAge, idPath))
	st.SetClock(func() time.Time {
		return time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	})
	if err := st.Init("svc", "prod", []string{testRecipient}); err != nil {
		t.Fatalf("init: %v", err)
	}
	return st, idPath, fakeAge, freeTCPAddr(t)
}

// freeTCPAddr returns a host:port for a port that was free at the
// instant the test asked. The OS may give the port to someone else
// before the doctor probe runs; that races, so use it for the
// healthy-setup tests only.
func freeTCPAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()
	return addr
}

// writeFakeAge installs a ROT13 stand-in for age in root. Doctor
// only ever calls --version on this binary, so the script returns
// a fixed line for that flag.
func writeFakeAge(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "age")
	const script = `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'fake-age 1.2.3\n'
  exit 0
fi
if [ "$1" = "-d" ]; then
  for last do :; done
  sed '1d' "$last" | tr 'A-Za-z' 'N-ZA-Mn-za-m'
else
  printf 'FAKE AGE CIPHERTEXT\n'
  tr 'A-Za-z' 'N-ZA-Mn-za-m'
fi
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake age: %v", err)
	}
	return path
}

// writeIdentity creates a 0600 file shaped like an age-keygen output.
func writeIdentity(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "id.txt")
	const contents = "# created: 2026-01-01T00:00:00Z\n" +
		"# public key: age1example7777777777777777777777777777777777777777777\n" +
		"AGE-SECRET-KEY-1XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write identity: %v", err)
	}
	return path
}

// findCheck returns the first CheckResult with the given Name,
// failing the test if none is found.
func findCheck(t *testing.T, r doctor.Report, name string) doctor.CheckResult {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q check in report: %+v", name, r.Checks)
	return doctor.CheckResult{}
}
