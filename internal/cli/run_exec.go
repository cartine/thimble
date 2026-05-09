package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/dotenv"
	"github.com/cartine/thimble/internal/store"
)

// ExitCodeError carries a non-zero child exit code up to main.go so
// `thimble exec` can mirror the child's status. main.go detects this
// type and calls os.Exit(Code) instead of the default 1.
type ExitCodeError struct{ Code int }

// Error reports the carried exit code in a one-line form.
func (e *ExitCodeError) Error() string {
	return fmt.Sprintf("child exited with status %d", e.Code)
}

// runExec is K-58: decrypt a whole namespace and fork+exec a child
// with the values delivered either as a dotenv body on stdin (default)
// or as an env block (--env). Never writes to the filesystem.
func runExec(
	ctx context.Context, st *store.Store, cfg cliConfig,
	args []string, stdout, stderr io.Writer,
) error {
	useEnv, allowShellEnv, rest, err := parseExecFlags(args, stderr)
	if err != nil {
		return err
	}
	if len(rest) < 4 {
		return errors.New(
			"usage: thimble exec [--env] [--allow-shell-env] " +
				"<app> <env> -- <command> [args...]",
		)
	}
	app, env := rest[0], rest[1]
	cmdArgs, err := commandAfterDash(rest[2:])
	if err != nil {
		return err
	}
	if useEnv && !allowShellEnv {
		if err := guardShellEnvAll(cmdArgs); err != nil {
			return err
		}
	}
	values, _, err := st.ReadEnv(app, env)
	if err != nil {
		return err
	}
	runErr := runNamespaceConsumer(ctx, cmdArgs, values, useEnv, stdout, stderr)
	auditExec(cfg, st, app, env, cmdArgs[0], stderr)
	return runErr
}

// parseExecFlags splits the K-58 flag set off the front of args.
func parseExecFlags(args []string, stderr io.Writer) (bool, bool, []string, error) {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	useEnv := fs.Bool("env", false,
		"populate child env block instead of piping dotenv to stdin")
	allowShellEnv := fs.Bool("allow-shell-env", false,
		"allow --env when the child is a shell or docker run (K-24)")
	if err := fs.Parse(args); err != nil {
		return false, false, nil, err
	}
	return *useEnv, *allowShellEnv, fs.Args(), nil
}

// runNamespaceConsumer forks cmdArgs[0] with cmdArgs[1:] as argv. If
// useEnv is false the dotenv-encoded values arrive on the child's
// stdin and the child inherits the parent env. If useEnv is true the
// values are appended to the child's env block and stdin is left
// detached. Mutually exclusive — never both at once.
func runNamespaceConsumer(
	ctx context.Context, cmdArgs []string,
	values map[string]string, useEnv bool,
	stdout, stderr io.Writer,
) error {
	// #nosec G204 -- cmdArgs is the operator-supplied argv after `--`.
	// Running operator-supplied consumers is the documented design of
	// `thimble exec`.
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if useEnv {
		envBlock, err := buildEnvBlock(values)
		if err != nil {
			return err
		}
		cmd.Env = envBlock
	} else {
		cmd.Stdin = strings.NewReader(dotenv.Encode(values))
	}
	return runChildAndPropagate(cmd)
}

// buildEnvBlock returns os.Environ() extended with every value in
// values formatted as "KEY=value". Keys are validated defense-in-depth
// even though the dotenv layer should have rejected invalid ones.
func buildEnvBlock(values map[string]string) ([]string, error) {
	out := append([]string{}, os.Environ()...)
	for key, value := range values {
		if err := dotenv.ValidateKey(key); err != nil {
			return nil, fmt.Errorf("invalid namespace key: %w", err)
		}
		out = append(out, key+"="+value)
	}
	return out, nil
}

// runChildAndPropagate runs cmd and converts a non-zero exit into an
// ExitCodeError so main.go can mirror the status. Other errors (e.g.
// command not found) bubble up unchanged.
func runChildAndPropagate(cmd *exec.Cmd) error {
	err := cmd.Run()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &ExitCodeError{Code: exitErr.ExitCode()}
	}
	return err
}

// guardShellEnvAll reuses K-24's shell guard. The blacklisted child
// basenames refuse outright; docker/podman refuse unless the run uses
// `--env-file=-` (the only stdin form that does not leak env from the
// operator). Per-key `-e KEY` scoping is irrelevant here because we
// inject many keys, so we ignore that branch by passing an empty
// envVar and relying on the basename check + --env-file=- detection.
func guardShellEnvAll(cmdArgs []string) error {
	if len(cmdArgs) == 0 {
		return nil
	}
	base := strings.ToLower(filepath.Base(cmdArgs[0]))
	if shellChildren[base] {
		return errors.New(
			"use stdin (drop --env) or --allow-shell-env; " +
				"child shell will export every value",
		)
	}
	if base == "docker" || base == "podman" {
		if !dockerRunReadsEnvFromStdin(cmdArgs[1:]) {
			return errors.New(
				"use stdin (drop --env) or --allow-shell-env or " +
					"`--env-file=-`; the container would inherit operator env",
			)
		}
	}
	return nil
}

// dockerRunReadsEnvFromStdin reports whether a `docker run` argv reads
// its env block from stdin via `--env-file=-`. That is the only form
// we accept under K-58's --env flavor without --allow-shell-env.
func dockerRunReadsEnvFromStdin(args []string) bool {
	if len(args) == 0 || args[0] != "run" {
		return true
	}
	for _, a := range args[1:] {
		if a == "--env-file=-" {
			return true
		}
	}
	return false
}

// auditExec appends a single audit entry for the K-58 exec op. The
// Subject is the child's basename (never the full path). Audit IO
// failure logs to stderr but never blocks the exec — the child has
// already run.
func auditExec(
	cfg cliConfig, st *store.Store, app, env, child string, stderr io.Writer,
) {
	logger := audit.New(cfg.storeDir, stderr)
	_ = logger.Append(audit.Event{
		Operator: execOperatorThumbprint(cfg, st),
		Op:       audit.OpExec,
		App:      app,
		Env:      env,
		Subject:  filepath.Base(child),
	})
}

// execOperatorThumbprint resolves the operator thumbprint from the
// configured identity file. Mirrors the lazy pattern in
// internal/store/audit_hook.go but is computed inline because runExec
// goes around the Store.recordEvent path (exec is a read, not a
// mutation, so it does not flow through rewriteEnv).
func execOperatorThumbprint(cfg cliConfig, st *store.Store) string {
	_ = st
	if cfg.identity == "" {
		return audit.UnknownOperator
	}
	rec, err := audit.PublicRecipientFromIdentityFile(cfg.identity)
	if err != nil {
		return audit.UnknownOperator
	}
	return audit.Thumbprint(rec)
}
