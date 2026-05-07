package audit_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/audit"
)

// TestAppendCreatesLogWithMode0640 covers K-27 #2: file mode is 0640
// on first creation, opened with O_APPEND so concurrent writers do
// not interleave.
func TestAppendCreatesLogWithMode0640(t *testing.T) {
	root := t.TempDir()
	logger := audit.New(root, nil)
	if err := logger.Append(audit.Event{
		Operator: "abc123", Op: audit.OpInit, App: "svc", Env: "prod",
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, audit.LogFileName))
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("audit log mode = %#o, want 0o640", got)
	}
}

// TestAppendIsAppendOnly covers K-27: each Append adds exactly one
// JSON line; a second Append does not truncate the first.
func TestAppendIsAppendOnly(t *testing.T) {
	root := t.TempDir()
	logger := audit.New(root, nil)
	if err := logger.Append(audit.Event{Op: audit.OpInit, App: "a", Env: "e"}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	ev2 := audit.Event{Op: audit.OpSet, App: "a", Env: "e", Subject: "K"}
	if err := logger.Append(ev2); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, audit.LogFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines; want 2: %q", len(lines), b)
	}
}

// TestAppendNeverContainsRecipientFullValue covers K-27's
// "thumbprints only" constraint: the log must never carry a full age
// recipient string.
func TestAppendNeverContainsRecipientFullValue(t *testing.T) {
	const recipient = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
	thumb := audit.Thumbprint(recipient)
	root := t.TempDir()
	logger := audit.New(root, nil)
	if err := logger.Append(audit.Event{
		Operator: thumb, Op: audit.OpRecipientAdd, App: "svc", Env: "prod",
		Subject: thumb,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, audit.LogFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if strings.Contains(string(b), recipient) {
		t.Fatalf("audit log contains full recipient: %q", b)
	}
	if !strings.Contains(string(b), thumb) {
		t.Fatalf("audit log missing thumbprint: %q", b)
	}
}

// TestThumbprintIsOpaque covers K-27 #6: the thumbprint must not be
// the recipient string itself, and two different recipients must
// give different thumbprints.
func TestThumbprintIsOpaque(t *testing.T) {
	const a = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
	const b = "age1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
	tA := audit.Thumbprint(a)
	tB := audit.Thumbprint(b)
	if tA == a || tB == b {
		t.Fatalf("thumbprint equal to recipient")
	}
	if tA == tB {
		t.Fatalf("thumbprints collided: %s == %s", tA, tB)
	}
	if len(tA) != audit.ThumbprintLen {
		t.Fatalf("thumbprint length = %d, want %d", len(tA), audit.ThumbprintLen)
	}
}

// TestPublicRecipientFromIdentityFile covers parsing a real
// age-keygen identity file: the `# public key:` line is read and
// nothing else is leaked.
func TestPublicRecipientFromIdentityFile(t *testing.T) {
	root := t.TempDir()
	idPath := filepath.Join(root, "id.txt")
	contents := "# created: 2026-01-01T00:00:00Z\n" +
		"# public key: age1example7777777777777777777777777777777777777777777\n" +
		"AGE-SECRET-KEY-1XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\n"
	if err := os.WriteFile(idPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write id: %v", err)
	}
	got, err := audit.PublicRecipientFromIdentityFile(idPath)
	if err != nil {
		t.Fatalf("parse id: %v", err)
	}
	want := "age1example7777777777777777777777777777777777777777777"
	if got != want {
		t.Fatalf("public = %q, want %q", got, want)
	}
}

// TestReadFilterSkipsMalformed covers Read's malformed-line
// tolerance: a junk line in the middle of the log does not break
// reading the surrounding entries.
func TestReadFilterSkipsMalformed(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, audit.LogFileName)
	contents := `{"op":"init","app":"a","env":"e"}` + "\n" +
		"this is not json\n" +
		`{"op":"set","app":"a","env":"e","subject":"K"}` + "\n"
	if err := os.WriteFile(path, []byte(contents), 0o640); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := audit.Read(root)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	if got[0].Op != audit.OpInit || got[1].Op != audit.OpSet {
		t.Fatalf("ops = %v %v", got[0].Op, got[1].Op)
	}
	filtered := audit.Filter(got, "a", "e")
	if len(filtered) != 2 {
		t.Fatalf("filter dropped events: %v", filtered)
	}
	other := audit.Filter(got, "z", "e")
	if len(other) != 0 {
		t.Fatalf("filter passed wrong app: %v", other)
	}
}

// TestAppendReportsWarnOnFailure covers K-27 #5: a failed Append
// emits a single line to the warn writer ("audit append failed:
// ...") and returns an error to the caller (who is expected to
// continue without aborting the user's mutation).
func TestAppendReportsWarnOnFailure(t *testing.T) {
	var warn strings.Builder
	// Point at a path that cannot be written to (a directory).
	logger := audit.New(t.TempDir(), &warn)
	// Make the audit log path actually be a directory.
	if err := os.Mkdir(filepath.Join(logger.Path()), 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	err := logger.Append(audit.Event{Op: audit.OpInit, App: "a", Env: "e"})
	if err == nil {
		t.Fatalf("Append on directory path succeeded")
	}
	if !strings.Contains(warn.String(), "audit append failed") {
		t.Fatalf("warn missing prefix: %q", warn.String())
	}
	// Sanity: we got a real os error type, not just a string sniff.
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("err = %v; want *os.PathError", err)
	}
}
