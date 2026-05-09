// Package age is the only place outside cmd/thimble that handles
// plaintext outside an in-memory buffer for one command's lifetime.
// It shells out to the trusted `age` binary for encrypt and decrypt;
// it never persists plaintext to disk and redacts stderr before
// surfacing it in errors.
package age

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout is how long Thimble waits on an age subprocess
// before cancelling it. THIMBLE_AGE_TIMEOUT (seconds) overrides.
const DefaultTimeout = 10 * time.Second

// EnvTimeoutVar is the env var operators set to widen the timeout
// for slow hardware or unusually large bundles.
const EnvTimeoutVar = "THIMBLE_AGE_TIMEOUT"

// Tool wraps invocations of the `age` binary. The zero value is not
// usable; construct one via New.
type Tool struct {
	binary    string
	identity  string
	sha256Pin string

	// allowUnsafeIdentityMode disables the 0o077 mode check on the
	// identity file. K-19 wires this via the
	// --unsafe-allow-identity-mode CLI flag.
	allowUnsafeIdentityMode bool
	unsafeWarn              io.Writer

	// verbose, if non-nil, receives a one-shot "using age binary: <path>"
	// announcement on first encrypt or decrypt. Wired by the CLI when
	// --verbose is set.
	verboseMu     sync.Mutex
	verbose       io.Writer
	verboseLogged bool
}

// New returns a Tool that invokes binary (e.g. "age") and decrypts with
// identity (an age identity file path; empty disables -i).
func New(binary, identity string) *Tool {
	return &Tool{binary: binary, identity: identity}
}

// SetSHA256Pin records an optional hex-encoded SHA-256 that the resolved
// binary must match. An empty pin disables verification.
func (t *Tool) SetSHA256Pin(pin string) { t.sha256Pin = pin }

// Identity returns the identity file path the Tool was constructed
// with (empty if none). K-27 reads this to derive an opaque operator
// thumbprint for the audit log.
func (t *Tool) Identity() string { return t.identity }

// Binary returns the configured age binary path. K-29 (`thimble
// doctor`) reads this so it can print the trust anchor without
// re-resolving PATH.
func (t *Tool) Binary() string { return t.binary }

// SetVerbose installs a writer that receives a single
// "thimble: using age binary: <path>" line the first time the Tool
// invokes the age binary. nil disables the announcement.
func (t *Tool) SetVerbose(w io.Writer) { t.verbose = w }

// AllowUnsafeIdentityMode disables the K-19 0o077 mode check on the
// identity file. The supplied writer (typically stderr) receives a
// one-line warning the first time the file is read.
func (t *Tool) AllowUnsafeIdentityMode(warn io.Writer) {
	t.allowUnsafeIdentityMode = true
	t.unsafeWarn = warn
}

// CheckIdentityMode verifies that path is mode 0600 (no group or
// world bits). If allowUnsafe is true the check is skipped and a
// single warning line is written to warn.
func CheckIdentityMode(path string, allowUnsafe bool, warn io.Writer) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("identity file %q: %w", path, err)
	}
	mode := info.Mode().Perm()
	if mode&0o077 == 0 {
		return nil
	}
	if allowUnsafe {
		if warn != nil {
			fmt.Fprintf(warn,
				"thimble: warning: identity file %s is mode 0%o; "+
					"--unsafe-allow-identity-mode in effect\n",
				path, mode)
		}
		return nil
	}
	return fmt.Errorf(
		"identity file %s is mode 0%o; expected 0600 "+
			"(run `chmod 0600 %s` and retry)",
		path, mode, path,
	)
}

// Resolve looks up binary on PATH (or accepts an absolute path) and, if
// sha256Pin is non-empty, verifies the file's SHA-256 matches. The
// resolved absolute path is returned. K-29 (`thimble doctor`) will
// consume Resolve's output to surface the trust anchor on demand.
func Resolve(binary, sha256Pin string) (string, error) {
	if binary == "" {
		binary = "age"
	}
	resolved, err := exec.LookPath(binary)
	if err != nil {
		return "", fmt.Errorf("age binary %q not found on PATH: %w", binary, err)
	}
	if sha256Pin == "" {
		return resolved, nil
	}
	want := strings.ToLower(strings.TrimSpace(sha256Pin))
	got, err := fileSHA256(resolved)
	if err != nil {
		return "", fmt.Errorf("hashing age binary %q: %w", resolved, err)
	}
	if got != want {
		return "", fmt.Errorf(
			"age binary %q sha256 = %s; THIMBLE_AGE_SHA256 = %s; refusing to run",
			resolved, got, want,
		)
	}
	return resolved, nil
}

func fileSHA256(path string) (string, error) {
	// #nosec G304 -- path is the resolved age binary; the caller is
	// pinning it on purpose.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// announceBinary writes the "using age binary" notice exactly once. It
// is a no-op when SetVerbose has not been called.
func (t *Tool) announceBinary() {
	if t.verbose == nil {
		return
	}
	t.verboseMu.Lock()
	defer t.verboseMu.Unlock()
	if t.verboseLogged {
		return
	}
	t.verboseLogged = true
	fmt.Fprintf(t.verbose, "thimble: using age binary: %s\n", t.binary)
}

// Encrypt encrypts plain into ASCII-armored age ciphertext addressed to
// recipients. It returns an error if recipients is empty or if the age
// binary fails; stderr is redacted to avoid leaking values. K-26
// applies a per-call timeout (THIMBLE_AGE_TIMEOUT, default 10s) and
// honors cancellation of ctx (e.g. SIGINT propagated by the CLI).
func (t *Tool) Encrypt(ctx context.Context, recipients []string, plain string) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, errors.New("at least one recipient is required")
	}
	t.announceBinary()
	args := []string{"-a"}
	for _, recipient := range recipients {
		args = append(args, "-r", recipient)
	}
	withTimeout, cancel, deadline := contextWithTimeout(ctx)
	defer cancel()
	// #nosec G204 -- t.binary is the trusted age binary configured at
	// startup; recipients are validated by store.ValidateRecipient before
	// reaching here. Resolve() pins the absolute path with optional
	// SHA-256 verification (K-18).
	cmd := exec.CommandContext(withTimeout, t.binary, args...)
	cmd.Stdin = strings.NewReader(plain)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, wrapAgeError(withTimeout, "encrypt", deadline, stderr.String())
	}
	return stdout.Bytes(), nil
}

// Decrypt decrypts the age-encrypted file at path using the configured
// identity (if any). It returns the plaintext as a string with stderr
// redacted on error. K-26 enforces THIMBLE_AGE_TIMEOUT and propagates
// ctx cancellation to the child.
func (t *Tool) Decrypt(ctx context.Context, path string) (string, error) {
	t.announceBinary()
	args := []string{"-d"}
	if t.identity != "" {
		if err := CheckIdentityMode(
			t.identity, t.allowUnsafeIdentityMode, t.unsafeWarn,
		); err != nil {
			return "", err
		}
		args = append(args, "-i", t.identity)
	}
	args = append(args, path)
	withTimeout, cancel, deadline := contextWithTimeout(ctx)
	defer cancel()
	// #nosec G204 -- t.binary is the trusted age binary configured at
	// startup; path is a manifest-controlled file inside the store root.
	// Resolve() pins the absolute path with optional SHA-256
	// verification (K-18).
	cmd := exec.CommandContext(withTimeout, t.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", wrapAgeError(withTimeout, "decrypt", deadline, stderr.String())
	}
	return stdout.String(), nil
}

// contextWithTimeout layers DefaultTimeout (or THIMBLE_AGE_TIMEOUT)
// onto parent and returns the wrapped context, its cancel function,
// and the timeout duration in seconds for use in error messages.
func contextWithTimeout(
	parent context.Context,
) (context.Context, context.CancelFunc, time.Duration) {
	d := DefaultTimeout
	if v := os.Getenv(EnvTimeoutVar); v != "" {
		if parsed, err := time.ParseDuration(v + "s"); err == nil && parsed > 0 {
			d = parsed
		}
	}
	ctx, cancel := context.WithTimeout(parent, d)
	return ctx, cancel, d
}

// wrapAgeError shapes the error returned to callers so a context
// timeout is distinguishable from a non-zero exit. Cancelled-by-
// caller errors propagate ctx.Err() unchanged so signal handling
// reads cleanly higher up. The op string is "encrypt" or "decrypt"
// so the message points at the failing direction.
func wrapAgeError(ctx context.Context, op string, deadline time.Duration, stderr string) error {
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf(
			"age timed out after %ds; rerun with %s=N if your hardware "+
				"is slow or the bundle is large",
			int(deadline.Seconds()), EnvTimeoutVar,
		)
	}
	if ctx.Err() == context.Canceled {
		return fmt.Errorf("age %s cancelled: %w", op, ctx.Err())
	}
	return fmt.Errorf("age %s failed: %s", op, Redact(stderr))
}

// Redact trims and truncates stderr from the age binary (or any other
// untrusted producer) so it does not leak secret values into Thimble's
// error messages.
func Redact(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no details"
	}
	const maxLen = 240
	if len(s) > maxLen {
		s = s[:maxLen] + "..."
	}
	return s
}
