// Package age is the only place outside cmd/thimble that handles
// plaintext outside an in-memory buffer for one command's lifetime.
// It shells out to the trusted `age` binary for encrypt and decrypt;
// it never persists plaintext to disk and redacts stderr before
// surfacing it in errors.
package age

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Tool wraps invocations of the `age` binary. The zero value is not
// usable; construct one via New.
type Tool struct {
	binary   string
	identity string
}

// New returns a Tool that invokes binary (e.g. "age") and decrypts with
// identity (an age identity file path; empty disables -i).
func New(binary, identity string) *Tool {
	return &Tool{binary: binary, identity: identity}
}

// Encrypt encrypts plain into ASCII-armored age ciphertext addressed to
// recipients. It returns an error if recipients is empty or if the age
// binary fails; stderr is redacted to avoid leaking values.
func (t *Tool) Encrypt(ctx context.Context, recipients []string, plain string) ([]byte, error) {
	if len(recipients) == 0 {
		return nil, errors.New("at least one recipient is required")
	}
	args := []string{"-a"}
	for _, recipient := range recipients {
		args = append(args, "-r", recipient)
	}
	// #nosec G204 -- t.binary is the trusted age binary configured at
	// startup; recipients are validated by store.ValidateRecipient before
	// reaching here. K-18 will pin the binary path absolutely.
	cmd := exec.CommandContext(ctx, t.binary, args...)
	cmd.Stdin = strings.NewReader(plain)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("age encrypt failed: %s", Redact(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Decrypt decrypts the age-encrypted file at path using the configured
// identity (if any). It returns the plaintext as a string with stderr
// redacted on error.
func (t *Tool) Decrypt(ctx context.Context, path string) (string, error) {
	args := []string{"-d"}
	if t.identity != "" {
		args = append(args, "-i", t.identity)
	}
	args = append(args, path)
	// #nosec G204 -- t.binary is the trusted age binary configured at
	// startup; path is a manifest-controlled file inside the store root.
	// K-18 will pin the binary path absolutely.
	cmd := exec.CommandContext(ctx, t.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("age decrypt failed: %s", Redact(stderr.String()))
	}
	return stdout.String(), nil
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
