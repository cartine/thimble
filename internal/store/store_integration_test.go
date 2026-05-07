//go:build integration

// Package store_test (integration variant): exercises store.Store against
// the *real* age and age-keygen binaries on PATH. Unit tests substitute
// a ROT13 fake (writeFakeAge in store_test.go); this file is what
// catches drift in real age's flags or output format.
package store_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/store"
)

// TestRealAgeLifecycle runs the full single-recipient lifecycle
// (init → set → list → render) and the recipient-rotation flow
// (add → render → remove) against the real `age` binary. The test
// is tagged `integration` so it only runs when invoked as
// `go test -tags integration ./...`.
func TestRealAgeLifecycle(t *testing.T) {
	requireBinaries(t, "age", "age-keygen")

	root := t.TempDir()
	idPath, pubA := generateIdentity(t, filepath.Join(root, "id-a.txt"))
	_, pubB := generateIdentity(t, filepath.Join(root, "id-b.txt"))

	st := store.New(filepath.Join(root, "secrets"), idPath)
	// Real `age` and `age-keygen` produce real ciphertext.
	st.SetAge(age.New("age", idPath))

	if err := st.Init("svc", "prod", []string{pubA}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "API_TOKEN", "s3cr3t value"); err != nil {
		t.Fatalf("set API_TOKEN: %v", err)
	}
	if err := st.CreateSecret("svc", "prod", "DB_URL", "postgres://x"); err != nil {
		t.Fatalf("create DB_URL: %v", err)
	}

	keys, err := st.ListSecrets("svc", "prod")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got, want := strings.Join(keys, ","), "API_TOKEN,DB_URL"; got != want {
		t.Fatalf("keys = %q, want %q", got, want)
	}

	rendered, err := st.Render("svc", "prod")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, `API_TOKEN="s3cr3t value"`) {
		t.Fatalf("rendered missing API_TOKEN: %q", rendered)
	}
	if !strings.Contains(rendered, `DB_URL=postgres://x`) {
		t.Fatalf("rendered missing DB_URL: %q", rendered)
	}

	if err := st.AddRecipient("svc", "prod", pubB); err != nil {
		t.Fatalf("add recipient B: %v", err)
	}
	rendered2, err := st.Render("svc", "prod")
	if err != nil {
		t.Fatalf("render after add: %v", err)
	}
	if rendered2 != rendered {
		t.Fatalf("render changed after AddRecipient: %q vs %q", rendered2, rendered)
	}
	if err := st.RemoveRecipient("svc", "prod", pubB); err != nil {
		t.Fatalf("remove recipient B: %v", err)
	}
	if err := st.RemoveRecipient("svc", "prod", pubA); err == nil {
		t.Fatalf("RemoveRecipient on last recipient must error")
	}
}

// TestRealAgeBundleSHA256Mismatch covers K-22 against real age: a
// post-write tamper on the ciphertext must cause the next decrypt
// to be rejected with the SHA-mismatch error before age runs.
func TestRealAgeBundleSHA256Mismatch(t *testing.T) {
	requireBinaries(t, "age", "age-keygen")

	root := t.TempDir()
	idPath, pubA := generateIdentity(t, filepath.Join(root, "id-a.txt"))

	st := store.New(filepath.Join(root, "secrets"), idPath)
	st.SetAge(age.New("age", idPath))

	if err := st.Init("svc", "prod", []string{pubA}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "API_TOKEN", "value"); err != nil {
		t.Fatalf("set: %v", err)
	}
	bundlePath := filepath.Join(root, "secrets", "svc", "prod.env.age")
	b, err := os.ReadFile(bundlePath) // #nosec G304 -- test-only path.
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	b = append(b, 'X')
	if err := os.WriteFile(bundlePath, b, 0o600); err != nil {
		t.Fatalf("tamper write: %v", err)
	}
	_, err = st.Render("svc", "prod")
	if err == nil {
		t.Fatalf("Render after tamper succeeded; want SHA-256 mismatch")
	}
	if !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("error %q missing SHA-256 mismatch text", err)
	}
}

// requireBinaries skips the test if any of the named binaries cannot
// be resolved on PATH. Keeps `go test ./...` clean in dev.
func requireBinaries(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("integration test needs %q on PATH: %v", name, err)
		}
	}
}

// generateIdentity runs age-keygen, writes the identity file at path,
// and returns (path, public-key string). The public key is parsed from
// the `# public key:` header that age-keygen writes to the file.
func generateIdentity(t *testing.T, path string) (string, string) {
	t.Helper()
	// #nosec G204 -- age-keygen is the trusted upstream binary; path
	// is a t.TempDir() child controlled by the test.
	cmd := exec.Command("age-keygen", "-o", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("age-keygen: %v: %s", err, out)
	}
	pubRE := regexp.MustCompile(`age1[ac-hj-np-z02-9]+`)
	pub := pubRE.FindString(string(out))
	if pub == "" {
		t.Fatalf("age-keygen did not print a recipient: %s", out)
	}
	return path, pub
}
