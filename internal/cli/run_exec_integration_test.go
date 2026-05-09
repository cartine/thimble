//go:build integration

// K-58 integration coverage: drive `thimble exec` end-to-end through
// the real `age` binary so we catch drift in real ciphertext format
// or argv plumbing that the ROT13 fake would mask.
package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/store"
)

// TestExecRealAgeStdinFlavor: real age, real namespace, real exec of
// `cat -` to consume stdin. The captured stdout must be the dotenv
// body the K-58 default flavor pipes in.
func TestExecRealAgeStdinFlavor(t *testing.T) {
	requireBinariesExec(t, "age", "age-keygen", "cat")
	root := t.TempDir()
	idPath, pub := makeIdentity(t, filepath.Join(root, "id.txt"))
	st := store.New(filepath.Join(root, "secrets"), idPath)
	st.SetAge(age.New("age", idPath))
	if err := st.Init("svc", "prod", []string{pub}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "DATABASE_URL", "postgres://x"); err != nil {
		t.Fatalf("set DATABASE_URL: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "API_KEY", "k-58-real"); err != nil {
		t.Fatalf("set API_KEY: %v", err)
	}
	cfg := cliConfig{storeDir: st.Root(), identity: idPath}
	args := []string{"svc", "prod", "--", "cat", "-"}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("runExec: %v stderr=%s", err, stderr.String())
	}
	want := "API_KEY=k-58-real\nDATABASE_URL=postgres://x\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

// requireBinariesExec is a local sibling of the store package's
// requireBinaries helper. Skips the test if any binary is missing.
func requireBinariesExec(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("integration test needs %q on PATH: %v", name, err)
		}
	}
}

// makeIdentity runs age-keygen, returns (path, public-key).
func makeIdentity(t *testing.T, path string) (string, string) {
	t.Helper()
	// #nosec G204 -- age-keygen is the trusted upstream binary; path
	// is a t.TempDir() child controlled by the test.
	cmd := exec.Command("age-keygen", "-o", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("age-keygen: %v: %s", err, out)
	}
	pub := regexp.MustCompile(`age1[ac-hj-np-z02-9]+`).FindString(string(out))
	if pub == "" {
		t.Fatalf("age-keygen output missing recipient: %s", out)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("chmod identity: %v", err)
	}
	return path, pub
}
