// Package cli is the entry point for the thimble binary. It parses
// the top-level flags, builds an *store.Store, and dispatches to the
// per-subcommand handlers in this package. Trust boundary: this is
// where untrusted argv lands. Subcommand handlers must validate every
// positional value before forwarding it to internal/store or
// internal/web.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/store"
)

const (
	defaultStoreDir = "secrets"
	defaultAddr     = "127.0.0.1:8787"
)

type cliConfig struct {
	storeDir          string
	identity          string
	ageBinary         string
	ageSHA256         string
	verbose           bool
	allowUnsafeIDMode bool
}

// Run is the CLI entry point. argv is the program's args (no exe
// name). stdout and stderr are forwarded to subcommands so tests can
// capture output.
func Run(args []string, stdout, stderr io.Writer) error {
	if isVersionFlag(args) {
		return runVersion(stdout)
	}
	cfg, rest, err := parseTopFlags(args, stderr)
	if err != nil {
		return err
	}
	if rest == nil {
		printUsage(stdout)
		return nil
	}
	if rest[0] == "version" {
		return runVersion(stdout)
	}
	st, tool, err := buildStoreAndTool(cfg, stderr)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()
	st.SetContext(ctx)
	st.SetNoticeWriter(stderr)
	st.SetAuditLogger(audit.New(cfg.storeDir, stderr))
	return dispatch(ctx, st, tool, cfg, rest, stdout, stderr)
}

// isVersionFlag returns true when args is a bare --version / -v
// invocation (handled before flag parsing so ldflags-injected
// metadata is reachable without any other Thimble setup).
func isVersionFlag(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "--version", "-version":
		return len(args) == 1
	}
	return false
}

func parseTopFlags(args []string, stderr io.Writer) (cliConfig, []string, error) {
	cfg := cliConfig{
		storeDir:  envOrDefault("THIMBLE_STORE", defaultStoreDir),
		identity:  os.Getenv("THIMBLE_AGE_IDENTITY"),
		ageBinary: os.Getenv("THIMBLE_AGE_BINARY"),
		ageSHA256: os.Getenv("THIMBLE_AGE_SHA256"),
	}
	fs := flag.NewFlagSet("thimble", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.storeDir, "store", cfg.storeDir, "secrets store directory")
	fs.StringVar(&cfg.identity, "identity", cfg.identity, "age identity file for decrypting")
	fs.StringVar(&cfg.ageBinary, "age-binary", cfg.ageBinary,
		"absolute path to the age binary (overrides $PATH lookup)")
	fs.BoolVar(&cfg.verbose, "verbose", false,
		"announce the resolved age binary path on first use")
	fs.BoolVar(&cfg.allowUnsafeIDMode, "unsafe-allow-identity-mode", false,
		"allow group/world-readable identity files (warns to stderr)")
	if err := fs.Parse(args); err != nil {
		return cliConfig{}, nil, err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return cfg, nil, nil
	}
	return cfg, rest, nil
}

// buildStoreAndTool resolves the age binary path (and optional
// SHA-256 pin) once per command and returns both the Store backed by
// an age.Tool with that path and the Tool itself. Verbose mode wires
// stderr through so the trust anchor is visible at first use. K-29
// (`thimble doctor`) consumes the returned Tool to print the trust
// anchor without re-resolving PATH.
func buildStoreAndTool(cfg cliConfig, stderr io.Writer) (*store.Store, *age.Tool, error) {
	resolved, err := age.Resolve(cfg.ageBinary, cfg.ageSHA256)
	if err != nil {
		return nil, nil, err
	}
	tool := age.New(resolved, cfg.identity)
	if cfg.ageSHA256 != "" {
		tool.SetSHA256Pin(cfg.ageSHA256)
	}
	if cfg.verbose {
		tool.SetVerbose(stderr)
	}
	if cfg.allowUnsafeIDMode {
		tool.AllowUnsafeIdentityMode(stderr)
	}
	return store.NewWithAge(cfg.storeDir, tool), tool, nil
}

func dispatch(
	ctx context.Context, st *store.Store, tool *age.Tool, cfg cliConfig,
	args []string, stdout, stderr io.Writer,
) error {
	cmd := args[0]
	suppressPush, rest := extractNoPeerPush(args[1:])
	err := dispatchCommand(ctx, st, tool, cfg, cmd, rest, stdout, stderr)
	if err != nil {
		return err
	}
	if isMutatingCommand(cmd) {
		maybePushPeers(cfg, suppressPush, stderr)
	}
	return nil
}

// dispatchCommand is the inner switch over the command verb. The
// outer dispatch wrapper handles --no-peer-push extraction and the
// post-success push hook.
func dispatchCommand(
	ctx context.Context, st *store.Store, tool *age.Tool, cfg cliConfig,
	cmd string, rest []string, stdout, stderr io.Writer,
) error {
	switch cmd {
	case "init":
		return runInit(st, rest, stdout, stderr)
	case "recipient":
		return runRecipientV2(st, rest, stdout, stderr)
	case "create":
		return runWrite(st, rest, stdout, stderr, false)
	case "update":
		return runWrite(st, rest, stdout, stderr, true)
	case "set":
		return runSet(st, rest, stdout, stderr)
	case "provision":
		return runProvision(rest, stdout, stderr)
	case "and-set":
		return runAndSet(st, rest, stdout, stderr)
	case "and-get":
		return runAndGet(st, rest, stdout, stderr)
	case "delete", "rm":
		return runDelete(st, rest, stdout)
	case "list", "ls":
		return runList(st, rest, stdout)
	case "render":
		return runRender(st, rest, stdout, stderr)
	case "verify":
		return runVerify(st, rest, stdout, stderr)
	case "audit":
		return runAudit(st, rest, stdout, stderr)
	case "doctor":
		return runDoctor(ctx, st, tool, rest, stdout, stderr, cfg)
	case "web":
		return runWeb(st, rest, stdout, stderr)
	case "peer":
		return runPeer(cfg, rest, stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// isMutatingCommand returns true for the verbs that change on-disk
// state and therefore deserve a peer push.  Read-only commands
// (list, render, verify, audit, doctor, web, provision, and-get)
// return false so they don't trigger broadcast traffic.
func isMutatingCommand(cmd string) bool {
	switch cmd {
	case "init", "create", "update", "set", "delete", "rm",
		"and-set", "recipient":
		return true
	}
	return false
}

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, usageText)
}

const usageText = `Thimble keeps app/environment scoped dotenv secrets encrypted with age.

Usage:
  thimble [--store secrets] [--identity ~/.config/thimble/key.txt]
          [--age-binary /path/to/age] [--verbose] <command>

Top-level flags (or env):
  --age-binary, $THIMBLE_AGE_BINARY   pin the age binary path
  $THIMBLE_AGE_SHA256                 require this SHA-256 of the age binary
  --verbose                           print the resolved age binary on first use
  --unsafe-allow-identity-mode        allow group/world-readable identity files

Commands:
  version, --version                     print version, commit, and Go info
  init <app> <env> --recipient age1...   create an encrypted namespace
  recipient add [--bootstrap] <app> <env> age1...
                                          grant a recipient (quorum-gated when
                                          recipients.signed.toml is present;
                                          --bootstrap is the chicken-and-egg
                                          escape valid only at <2 recipients)
  recipient sign-add <app> <env> age1...  produce one operator signature
                                          (requires THIMBLE_AGE_IDENTITY)
  recipient remove [--rotate|--rotate-randoms-only] <app> <env> age1...
                                          remove a recipient and re-encrypt
                                          (--rotate regenerates every value
                                          provisioned by 'thimble provision';
                                          --rotate-randoms-only is the silent
                                          variant for scripts)
  recipient list <app> <env>              list recipients with thumbprints
  create <app> <env> KEY                  create one secret key from pipe or masked prompt
  update <app> <env> KEY                  update one existing key from pipe or masked prompt
  set <app> <env> KEY                     create or update one key from pipe or masked prompt
  provision [--bytes 32]                  generate a random secret for a pipe
  and-set [--show-stderr] <app> <env> KEY -- <command>
                                          set a key from a command's stdout
                                          (producer stderr is captured by default)
  and-get [--env NAME] [--allow-shell-env] <app> <env> KEY -- <command>
                                          pass a key to a command on stdin
                                          (refuses --env to bare shells unless --allow-shell-env)
  delete <app> <env> KEY                  delete one secret key
  list <app> <env>                        list keys only, never values
  render <app> <env> --format dotenv      render decrypted dotenv to stdout
  verify <app> <env>                      print bundle SHA + recipient list
  audit [--limit N] <app> <env>           print audit log entries for namespace
  doctor [--json] [--addr ...]            run setup/health diagnostics
  web [--addr 127.0.0.1:8787] [--allow-host foo.local:8787]
                                          run the local web UI
                                          (--allow-host is repeatable;
                                           default Hosts cover loopback + --addr)
  peer add <name> <ssh-target>            add a leader to the peers list
  peer remove <name>                      remove a leader from the peers list
  peer list                               list configured peer leaders
  peer join [--replace] <ssh-target>      bootstrap this leader by rsync'ing
                                          secrets/ from an existing peer

Per-mutation flags:
  --no-peer-push                          suppress the K-56 on-mutate broadcast
                                          (also: THIMBLE_PEER_PUSH=off globally)

Secret values are never accepted as command arguments.`
