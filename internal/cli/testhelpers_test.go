package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/store"
)

// newTestStore returns a Store backed by a fake `age` binary that
// reverses ROT13 instead of doing real encryption. Deterministic
// timestamps are pinned via SetClock so test output stays stable.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	st := store.New(filepath.Join(root, "secrets"), "")
	st.SetAge(age.New(fakeAge, ""))
	st.SetClock(func() time.Time {
		return time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	})
	return st
}

// newRawStore returns a Store whose age binary path comes from $PATH;
// used by tests that exercise the public Run entry point and rely on
// the binary lookup.
func newRawStore(root string) *store.Store {
	return store.New(root, "")
}

func writeFakeAge(t *testing.T, root string) string {
	t.Helper()
	fakeAge := filepath.Join(root, "age")
	script := `#!/bin/sh
set -eu
if [ "${1:-}" = "-d" ]; then
  for last do :; done
  sed '1d' "$last" | tr 'A-Za-z' 'N-ZA-Mn-za-m'
else
  printf 'FAKE AGE CIPHERTEXT\n'
  tr 'A-Za-z' 'N-ZA-Mn-za-m'
fi
`
	if err := os.WriteFile(fakeAge, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake age: %v", err)
	}
	return fakeAge
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	old := os.Stdin
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := write.WriteString(input); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := write.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	os.Stdin = read
	defer func() {
		os.Stdin = old
		read.Close()
	}()
	fn()
}
