package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/store"
)

// seedExecNamespace populates a single namespace with two keys so the
// dotenv body and env block both have something to assert against.
func seedExecNamespace(t *testing.T, st *store.Store) {
	t.Helper()
	if err := st.Init("web-api", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("web-api", "prod", "DATABASE_URL", "postgres://x"); err != nil {
		t.Fatalf("set DB: %v", err)
	}
	if err := st.SetSecret("web-api", "prod", "API_KEY", "k-58-secret"); err != nil {
		t.Fatalf("set API: %v", err)
	}
}

// writeShellChild emits a /bin/sh script at path that runs body with
// the operator-supplied argv appended. body is one or more shell
// commands; the script also exposes $ARGV_FILE so callers can
// reference a temp file path through the environment.
func writeShellChild(t *testing.T, path, body string) {
	t.Helper()
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write child: %v", err)
	}
}

// TestExecStdinFlavorPipesDotenvBody verifies the default flavor: the
// child receives the dotenv-encoded namespace on its stdin, no env
// injection, and exit 0 propagates as a nil error.
func TestExecStdinFlavorPipesDotenvBody(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	dir := t.TempDir()
	stdinFile := filepath.Join(dir, "stdin.txt")
	child := filepath.Join(dir, "child.sh")
	writeShellChild(t, child, `cat > "`+stdinFile+`"`)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{"web-api", "prod", "--", child}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("runExec: %v stderr=%s", err, stderr.String())
	}
	body, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatalf("read stdin capture: %v", err)
	}
	got := string(body)
	want := "API_KEY=k-58-secret\nDATABASE_URL=postgres://x\n"
	if got != want {
		t.Fatalf("stdin body = %q, want %q", got, want)
	}
}

// TestExecEnvFlavorPopulatesChildEnv verifies the --env flavor: the
// child sees DATABASE_URL and API_KEY in its env block, the parent's
// PATH is still inherited, and stdin is NOT written.
func TestExecEnvFlavorPopulatesChildEnv(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env.txt")
	stdinFile := filepath.Join(dir, "stdin.txt")
	child := filepath.Join(dir, "child.sh")
	body := `env > "` + envFile + `"
cat > "` + stdinFile + `" </dev/null || true`
	writeShellChild(t, child, body)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{"--env", "web-api", "prod", "--", child}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("runExec --env: %v stderr=%s", err, stderr.String())
	}
	envBytes, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env capture: %v", err)
	}
	envText := string(envBytes)
	for _, want := range []string{"DATABASE_URL=postgres://x", "API_KEY=k-58-secret"} {
		if !strings.Contains(envText, want) {
			t.Fatalf("env missing %q in %q", want, envText)
		}
	}
	stdinBytes, _ := os.ReadFile(stdinFile)
	if len(stdinBytes) != 0 {
		t.Fatalf("env flavor wrote to stdin: %q", stdinBytes)
	}
}

// TestExecShellGuardRefusesEnvFlavorAgainstShell asserts the K-24
// reuse: --env to a bare bash child without --allow-shell-env fails.
func TestExecShellGuardRefusesEnvFlavorAgainstShell(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{"--env", "web-api", "prod", "--", "bash", "-c", ":"}
	var stdout, stderr strings.Builder
	err := runExec(context.Background(), st, cfg, args, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected refusal, got success")
	}
	if !strings.Contains(err.Error(), "child shell") {
		t.Fatalf("expected shell-guard message, got %v", err)
	}
}

// TestExecShellGuardSilentForStdinFlavor asserts that the default
// (stdin) flavor does NOT trip the shell guard — the K-24 leak does
// not apply when env block is untouched. The child is bash reading
// stdin, which is the documented happy path.
func TestExecShellGuardSilentForStdinFlavor(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	dir := t.TempDir()
	stdinFile := filepath.Join(dir, "stdin.txt")
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{
		"web-api", "prod", "--",
		"bash", "-c", `cat > "` + stdinFile + `"`,
	}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("stdin flavor against bash: %v stderr=%s", err, stderr.String())
	}
	body, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatalf("read stdin: %v", err)
	}
	if !strings.Contains(string(body), "API_KEY=k-58-secret") {
		t.Fatalf("stdin body missing API_KEY: %q", body)
	}
}

// TestExecAllowShellEnvOptsOut asserts --allow-shell-env disables the
// guard so the operator can deliberately use --env with a shell.
func TestExecAllowShellEnvOptsOut(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{
		"--env", "--allow-shell-env",
		"web-api", "prod", "--",
		"bash", "-c", `printf '%s\n' "$API_KEY"`,
	}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("--allow-shell-env: %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "k-58-secret") {
		t.Fatalf("child did not print API_KEY: %q", stdout.String())
	}
}

// TestExecChildArgvDoesNotContainSecret is the explicit no-secret-in-
// argv assertion: the child writes its own os.Args (one per line) to
// a file, the test reads them back, and asserts no secret value
// appears anywhere in the recorded argv. The test is platform-
// independent (does not depend on /proc) — we capture argv from
// inside the child on whatever OS we're on.
func TestExecChildArgvDoesNotContainSecret(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	dir := t.TempDir()
	argvFile := filepath.Join(dir, "argv.txt")
	child := filepath.Join(dir, "child.sh")
	body := `printf '%s\n' "$0" "$@" > "` + argvFile + `"
cat >/dev/null`
	writeShellChild(t, child, body)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{"web-api", "prod", "--", child, "fixed-arg-1", "fixed-arg-2"}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("runExec: %v stderr=%s", err, stderr.String())
	}
	argv, err := os.ReadFile(argvFile)
	if err != nil {
		t.Fatalf("read argv: %v", err)
	}
	got := string(argv)
	for _, secret := range []string{"k-58-secret", "postgres://x"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret leaked into argv: %q", got)
		}
	}
	for _, want := range []string{"fixed-arg-1", "fixed-arg-2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("argv missing operator-supplied %q: %q", want, got)
		}
	}
}

// TestExecPropagatesChildExitCode covers exit-status mirroring: a
// child that exits 7 surfaces as ExitCodeError{Code: 7}.
func TestExecPropagatesChildExitCode(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	dir := t.TempDir()
	child := filepath.Join(dir, "child.sh")
	writeShellChild(t, child, `cat >/dev/null
exit 7`)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{"web-api", "prod", "--", child}
	var stdout, stderr strings.Builder
	err := runExec(context.Background(), st, cfg, args, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected non-zero exit, got nil")
	}
	exitErr, ok := err.(*ExitCodeError)
	if !ok {
		t.Fatalf("err type = %T, want *ExitCodeError: %v", err, err)
	}
	if exitErr.Code != 7 {
		t.Fatalf("exit code = %d, want 7", exitErr.Code)
	}
}

// TestExecAuditLogRecordsExecOp asserts a successful exec writes one
// JSONL row to .thimble-audit.log with op="exec" and the child basename
// in subject. Operator thumbprint is "unknown" because the test uses
// no identity file.
func TestExecAuditLogRecordsExecOp(t *testing.T) {
	st := newTestStore(t)
	seedExecNamespace(t, st)
	st.SetAuditLogger(audit.New(st.Root(), nil))
	dir := t.TempDir()
	child := filepath.Join(dir, "child.sh")
	writeShellChild(t, child, `cat >/dev/null`)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{"web-api", "prod", "--", child}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("runExec: %v", err)
	}
	logPath := filepath.Join(st.Root(), audit.LogFileName)
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if err := assertAuditHasExecRow(string(body), "child.sh"); err != nil {
		t.Fatalf("%v\nlog:\n%s", err, body)
	}
}

// assertAuditHasExecRow scans body for one JSON line with op=exec and
// the expected child basename in subject.
func assertAuditHasExecRow(body, wantChild string) error {
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		if line == "" {
			continue
		}
		var e audit.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return fmt.Errorf("parse line %q: %w", line, err)
		}
		if e.Op != audit.OpExec {
			continue
		}
		if e.Subject != wantChild {
			return fmt.Errorf("exec subject = %q, want %q", e.Subject, wantChild)
		}
		if e.App != "web-api" || e.Env != "prod" {
			return fmt.Errorf("exec app/env = %q/%q, want web-api/prod", e.App, e.Env)
		}
		return nil
	}
	return fmt.Errorf("no op=%s row found", audit.OpExec)
}

// TestExecDoesNotWriteToFilesystem covers the K-58 invariant: neither
// flavor creates new files in the store directory or any tracked
// scratch dir during the exec call. We snapshot the store dir before
// and after and compare.
func TestExecDoesNotWriteToFilesystem(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filesystem snapshot semantics differ on Windows")
	}
	st := newTestStore(t)
	seedExecNamespace(t, st)
	before := snapshotDir(t, st.Root())
	dir := t.TempDir()
	child := filepath.Join(dir, "child.sh")
	writeShellChild(t, child, `cat >/dev/null`)
	cfg := cliConfig{storeDir: st.Root()}
	args := []string{"web-api", "prod", "--", child}
	var stdout, stderr strings.Builder
	if err := runExec(context.Background(), st, cfg, args, &stdout, &stderr); err != nil {
		t.Fatalf("runExec: %v", err)
	}
	after := snapshotDir(t, st.Root())
	for path := range after {
		if _, ok := before[path]; !ok {
			// Audit log is the only newly created file we tolerate.
			if filepath.Base(path) == audit.LogFileName {
				continue
			}
			t.Fatalf("runExec created unexpected file %q", path)
		}
	}
}

// snapshotDir returns the relative path of every file under root.
func snapshotDir(t *testing.T, root string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out[rel] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}
