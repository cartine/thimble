package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/dotenv"
	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/web"
)

const (
	defaultStoreDir = "secrets"
	defaultAddr     = "127.0.0.1:8787"
)

type cliConfig struct {
	storeDir string
	identity string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "thimble:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	cfg := cliConfig{
		storeDir: envOrDefault("THIMBLE_STORE", defaultStoreDir),
		identity: os.Getenv("THIMBLE_AGE_IDENTITY"),
	}
	fs := flag.NewFlagSet("thimble", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.storeDir, "store", cfg.storeDir, "secrets store directory")
	fs.StringVar(&cfg.identity, "identity", cfg.identity, "age identity file for decrypting")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		printUsage(stdout)
		return nil
	}

	st := newStore(cfg.storeDir, cfg.identity)
	switch rest[0] {
	case "init":
		return runInit(st, rest[1:], stdout, stderr)
	case "recipient":
		return runRecipient(st, rest[1:], stdout, stderr)
	case "create":
		return runWrite(st, rest[1:], stdout, stderr, false)
	case "update":
		return runWrite(st, rest[1:], stdout, stderr, true)
	case "set":
		return runSet(st, rest[1:], stdout, stderr)
	case "provision":
		return runProvision(rest[1:], stdout, stderr)
	case "and-set":
		return runAndSet(st, rest[1:], stdout, stderr)
	case "and-get":
		return runAndGet(st, rest[1:], stdout, stderr)
	case "delete", "rm":
		return runDelete(st, rest[1:], stdout)
	case "list", "ls":
		return runList(st, rest[1:], stdout)
	case "render":
		return runRender(st, rest[1:], stdout, stderr)
	case "web":
		return runWeb(st, rest[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		return fmt.Errorf("unknown command %q", rest[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Thimble keeps app/environment scoped dotenv secrets encrypted with age.

Usage:
  thimble [--store secrets] [--identity ~/.config/thimble/key.txt] <command>

Commands:
  init <app> <env> --recipient age1...    create an encrypted namespace
  recipient add <app> <env> age1...       grant a recipient and re-encrypt
  recipient remove <app> <env> age1...    remove a recipient and re-encrypt
  create <app> <env> KEY                  create one secret key from pipe or masked prompt
  update <app> <env> KEY                  update one existing key from pipe or masked prompt
  set <app> <env> KEY                     create or update one key from pipe or masked prompt
  provision [--bytes 32]                  generate a random secret for a pipe
  and-set <app> <env> KEY -- <command>    set a key from a command's stdout
  and-get <app> <env> KEY -- <command>    pass a key to a command on stdin
  delete <app> <env> KEY                  delete one secret key
  list <app> <env>                        list keys only, never values
  render <app> <env> --format dotenv      render decrypted dotenv to stdout
  web [--addr 127.0.0.1:8787]             run the local web UI

Secret values are never accepted as command arguments.`)
}

func runInit(st *store.Store, args []string, stdout, stderr io.Writer) error {
	var recipients []string
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--recipient":
			if i+1 >= len(args) {
				return errors.New("--recipient requires a value")
			}
			recipients = append(recipients, args[i+1])
			i++
		case strings.HasPrefix(arg, "--recipient="):
			recipients = append(recipients, strings.TrimPrefix(arg, "--recipient="))
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown init flag %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return errors.New("usage: thimble init <app> <env> --recipient age1...")
	}
	if len(recipients) == 0 {
		return errors.New("init requires at least one --recipient")
	}
	app, env := positional[0], positional[1]
	if err := st.Init(app, env, recipients); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "initialized %s/%s\n", app, env)
	return nil
}

func runRecipient(st *store.Store, args []string, stdout, stderr io.Writer) error {
	if len(args) != 4 {
		return errors.New("usage: thimble recipient <add|remove> <app> <env> <age-recipient>")
	}
	action, app, env, recipient := args[0], args[1], args[2], args[3]
	switch action {
	case "add":
		if err := st.AddRecipient(app, env, recipient); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "added recipient to %s/%s\n", app, env)
	case "remove":
		if err := st.RemoveRecipient(app, env, recipient); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "removed recipient from %s/%s\n", app, env)
	default:
		return errors.New("usage: thimble recipient <add|remove> <app> <env> <age-recipient>")
	}
	return nil
}

func runWrite(st *store.Store, args []string, stdout, stderr io.Writer, requireExisting bool) error {
	if len(args) > 3 {
		return errors.New("do not pass secret values as arguments; pipe stdin or use the masked prompt")
	}
	if len(args) != 3 {
		if requireExisting {
			return errors.New("usage: thimble update <app> <env> <KEY>")
		}
		return errors.New("usage: thimble create <app> <env> <KEY>")
	}
	app, env, key := args[0], args[1], args[2]
	value, err := secretInput(key, stderr)
	if err != nil {
		return err
	}
	if requireExisting {
		err = st.UpdateSecret(app, env, key, value)
	} else {
		err = st.CreateSecret(app, env, key, value)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s\n", key, app, env)
	return nil
}

func runSet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	if len(args) > 3 {
		return errors.New("do not pass secret values as arguments; pipe stdin or use the masked prompt")
	}
	if len(args) != 3 {
		return errors.New("usage: thimble set <app> <env> <KEY>")
	}
	app, env, key := args[0], args[1], args[2]
	value, err := secretInput(key, stderr)
	if err != nil {
		return err
	}
	if err := st.SetSecret(app, env, key, value); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s\n", key, app, env)
	return nil
}

func runProvision(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("provision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	byteCount := fs.Int("bytes", 32, "random byte count before encoding")
	show := fs.Bool("show", false, "allow writing the generated secret to a terminal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: thimble provision [--bytes 32]")
	}
	if *byteCount < 16 {
		return errors.New("provision requires at least 16 bytes")
	}
	if writerIsTerminal(stdout) && !*show {
		return errors.New("refusing to print a new secret to the terminal; pipe it or pass --show")
	}
	b := make([]byte, *byteCount)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	_, err := fmt.Fprintln(stdout, base64.RawURLEncoding.EncodeToString(b))
	return err
}

func runAndSet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	if len(args) < 5 {
		return errors.New("usage: thimble and-set <app> <env> <KEY> -- <command> [args...]")
	}
	app, env, key := args[0], args[1], args[2]
	cmdArgs, err := commandAfterDash(args[3:])
	if err != nil {
		return err
	}
	value, err := runSecretProducer(cmdArgs, stderr)
	if err != nil {
		return err
	}
	if err := st.SetSecret(app, env, key, value); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s from command output\n", key, app, env)
	return nil
}

func runAndGet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("and-get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	envVar := fs.String("env", "", "also expose the secret as this environment variable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 5 {
		return errors.New("usage: thimble and-get [--env NAME] <app> <env> <KEY> -- <command> [args...]")
	}
	app, env, key := rest[0], rest[1], rest[2]
	cmdArgs, err := commandAfterDash(rest[3:])
	if err != nil {
		return err
	}
	values, _, err := st.ReadEnv(app, env)
	if err != nil {
		return err
	}
	value, ok := values[key]
	if !ok {
		return fmt.Errorf("%s does not exist", key)
	}
	return runSecretConsumer(cmdArgs, value, *envVar, stdout, stderr)
}

func runDelete(st *store.Store, args []string, stdout io.Writer) error {
	if len(args) != 3 {
		return errors.New("usage: thimble delete <app> <env> <KEY>")
	}
	app, env, key := args[0], args[1], args[2]
	if err := st.DeleteSecret(app, env, key); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deleted %s from %s/%s\n", key, app, env)
	return nil
}

func runList(st *store.Store, args []string, stdout io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: thimble list <app> <env>")
	}
	keys, err := st.ListSecrets(args[0], args[1])
	if err != nil {
		return err
	}
	for _, key := range keys {
		fmt.Fprintln(stdout, key)
	}
	return nil
}

func runRender(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "dotenv", "output format; only dotenv is supported")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: thimble render <app> <env> --format dotenv")
	}
	if *format != "dotenv" {
		return errors.New("only dotenv render format is supported")
	}
	plain, err := st.Render(fs.Arg(0), fs.Arg(1))
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, plain)
	return err
}

func runWeb(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("web", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", defaultAddr, "listen address")
	token := fs.String("token", os.Getenv("THIMBLE_WEB_TOKEN"), "web UI token")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: thimble web [--addr 127.0.0.1:8787]")
	}
	if *token == "" {
		if !isLoopbackAddr(*addr) {
			return errors.New("non-loopback web UI requires --token or THIMBLE_WEB_TOKEN")
		}
		generated, err := randomToken()
		if err != nil {
			return err
		}
		*token = generated
	}
	server := web.New(st, *token)
	mux := http.NewServeMux()
	server.Routes(mux)
	httpServer := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Fprintf(stdout, "Thimble web UI: http://%s/?token=%s\n", *addr, *token)
	return httpServer.ListenAndServe()
}

func newStore(root, identity string) *store.Store {
	return store.New(root, identity)
}

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
		return "", errors.New("secret value must come from a non-empty pipe or masked prompt")
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

func runSecretProducer(args []string, stderr io.Writer) (string, error) {
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

func runSecretConsumer(args []string, value, envVar string, stdout, stderr io.Writer) error {
	if envVar != "" {
		if err := dotenv.ValidateKey(envVar); err != nil {
			return fmt.Errorf("invalid --env name: %w", err)
		}
	}
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

func envOrDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

