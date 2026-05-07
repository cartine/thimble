package age_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/age"
)

func TestResolveLookPath(t *testing.T) {
	bin := writeFakeBinary(t, t.TempDir(), "echo hi")
	dir := filepath.Dir(bin)
	prependPath(t, dir)

	got, err := age.Resolve(filepath.Base(bin), "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("Resolve = %q, want prefix %q", got, dir)
	}
}

func TestResolveAbsolutePath(t *testing.T) {
	bin := writeFakeBinary(t, t.TempDir(), "echo hi")
	got, err := age.Resolve(bin, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != bin {
		t.Fatalf("Resolve = %q, want %q", got, bin)
	}
}

func TestResolveMatchingSHA256(t *testing.T) {
	bin := writeFakeBinary(t, t.TempDir(), "echo hi")
	want := sha256OfFile(t, bin)
	got, err := age.Resolve(bin, want)
	if err != nil {
		t.Fatalf("Resolve with pin: %v", err)
	}
	if got != bin {
		t.Fatalf("Resolve = %q, want %q", got, bin)
	}
}

func TestResolveSHA256Mismatch(t *testing.T) {
	bin := writeFakeBinary(t, t.TempDir(), "echo hi")
	bad := strings.Repeat("0", 64)
	_, err := age.Resolve(bin, bad)
	if err == nil {
		t.Fatalf("Resolve accepted mismatched SHA-256")
	}
	if !strings.Contains(err.Error(), "refusing to run") {
		t.Fatalf("Resolve error = %v, want 'refusing to run'", err)
	}
}

func TestResolveMissingBinary(t *testing.T) {
	_, err := age.Resolve("/no/such/binary/abcdef", "")
	if err == nil {
		t.Fatalf("Resolve accepted missing binary")
	}
}

func writeFakeBinary(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	bin := filepath.Join(dir, "age")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return bin
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func prependPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
