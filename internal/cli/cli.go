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
	cfg, rest, err := parseTopFlags(args, stderr)
	if err != nil {
		return err
	}
	if rest == nil {
		printUsage(stdout)
		return nil
	}
	st, err := buildStore(cfg, stderr)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()
	st.SetContext(ctx)
	return dispatch(st, rest, stdout, stderr)
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

// buildStore resolves the age binary path (and optional SHA-256 pin)
// once per command, then constructs a Store backed by an age.Tool with
// that path. Verbose mode wires stderr through so the trust anchor is
// visible at first use.
func buildStore(cfg cliConfig, stderr io.Writer) (*store.Store, error) {
	resolved, err := age.Resolve(cfg.ageBinary, cfg.ageSHA256)
	if err != nil {
		return nil, err
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
	return store.NewWithAge(cfg.storeDir, tool), nil
}

func dispatch(st *store.Store, args []string, stdout, stderr io.Writer) error {
	switch args[0] {
	case "init":
		return runInit(st, args[1:], stdout, stderr)
	case "recipient":
		return runRecipient(st, args[1:], stdout)
	case "create":
		return runWrite(st, args[1:], stdout, stderr, false)
	case "update":
		return runWrite(st, args[1:], stdout, stderr, true)
	case "set":
		return runSet(st, args[1:], stdout, stderr)
	case "provision":
		return runProvision(args[1:], stdout, stderr)
	case "and-set":
		return runAndSet(st, args[1:], stdout, stderr)
	case "and-get":
		return runAndGet(st, args[1:], stdout, stderr)
	case "delete", "rm":
		return runDelete(st, args[1:], stdout)
	case "list", "ls":
		return runList(st, args[1:], stdout)
	case "render":
		return runRender(st, args[1:], stdout, stderr)
	case "web":
		return runWeb(st, args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
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
  init <app> <env> --recipient age1...    create an encrypted namespace
  recipient add <app> <env> age1...       grant a recipient and re-encrypt
  recipient remove <app> <env> age1...    remove a recipient and re-encrypt
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
  web [--addr 127.0.0.1:8787]             run the local web UI

Secret values are never accepted as command arguments.`
