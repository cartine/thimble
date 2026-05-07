package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/dotenv"
)

// secretInput collects a secret value from either a masked terminal
// prompt or a non-empty pipe on stdin. Argv values are deliberately
// not accepted; callers reject those before reaching here.
func secretInput(key string, stderr io.Writer) (string, error) {
	if stdinIsTerminal() {
		fmt.Fprintf(stderr, "Secret value for %s: ", key)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", err
		}
		value := string(b)
		if value == "" {
			return "", errors.New("empty secret values are not accepted")
		}
		return value, nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	value := strings.TrimRight(string(b), "\r\n")
	if value == "" {
		return "", errors.New(
			"secret value must come from a non-empty pipe or masked prompt",
		)
	}
	return value, nil
}

func commandAfterDash(args []string) ([]string, error) {
	if len(args) == 0 || args[0] != "--" {
		return nil, errors.New("command separator -- is required")
	}
	if len(args) == 1 {
		return nil, errors.New("command after -- is required")
	}
	return args[1:], nil
}

// runSecretProducer runs the user-supplied command and captures its
// stdout as the secret value. Stderr is forwarded to stderr but a copy
// is buffered so failure messages can be redacted before surfacing.
func runSecretProducer(args []string, stderr io.Writer) (string, error) {
	// #nosec G204 -- args is the command the operator placed after `--`
	// on the Thimble CLI. Running operator-supplied commands is the
	// documented design of `thimble set --from-cmd ... --` and friends.
	cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.MultiWriter(stderr, &errOut)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("secret producer failed: %s", age.Redact(errOut.String()))
	}
	value := strings.TrimRight(out.String(), "\r\n")
	if value == "" {
		return "", errors.New("secret producer wrote no secret to stdout")
	}
	return value, nil
}

// runSecretConsumer runs the user-supplied command with the secret
// value piped to its stdin and (optionally) exposed in envVar.
func runSecretConsumer(args []string, value, envVar string, stdout, stderr io.Writer) error {
	if envVar != "" {
		if err := dotenv.ValidateKey(envVar); err != nil {
			return fmt.Errorf("invalid --env name: %w", err)
		}
	}
	// #nosec G204 -- args is the command the operator placed after `--`
	// on the Thimble CLI. Running operator-supplied consumers (e.g.
	// `thimble and-get app/env -- ./run.sh`) is the documented design.
	cmd := exec.CommandContext(context.Background(), args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(value)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if envVar != "" {
		cmd.Env = append(os.Environ(), envVar+"="+value)
	}
	return cmd.Run()
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func writerIsTerminal(w io.Writer) bool {
	file, ok := w.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
