package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
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
)

const (
	defaultStoreDir = "secrets"
	defaultAddr     = "127.0.0.1:8787"
)

type cliConfig struct {
	storeDir string
	identity string
}

type secretEntry struct {
	Key string
	Set bool
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
	server := &webServer{store: st, token: *token, templates: template.Must(template.New("ui").Parse(uiTemplate))}
	mux := http.NewServeMux()
	server.routes(mux)
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

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

type webServer struct {
	store     *store.Store
	token     string
	templates *template.Template
}

type pageData struct {
	Token      string
	Error      string
	Notice     string
	Namespaces []store.NamespaceView
	Selected   *selectedNamespace
}

type selectedNamespace struct {
	App        string
	Env        string
	Keys       []secretEntry
	Recipients []string
}

func (s *webServer) routes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/namespace", s.handleNamespace)
	mux.HandleFunc("/secret", s.handleSecret)
	mux.HandleFunc("/recipient", s.handleRecipient)
}

func (s *webServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.render(w, r, pageData{Token: s.token, Notice: r.URL.Query().Get("notice"), Error: r.URL.Query().Get("error")})
}

func (s *webServer) handleNamespace(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	app, env := r.FormValue("app"), r.FormValue("env")
	recipients := strings.FieldsFunc(r.FormValue("recipients"), func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	if err := s.store.Init(app, env, recipients); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	s.redirectNotice(w, r, "namespace created")
}

func (s *webServer) handleSecret(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	app, env, key := r.FormValue("app"), r.FormValue("env"), r.FormValue("key")
	var err error
	switch r.FormValue("action") {
	case "create":
		err = s.store.CreateSecret(app, env, key, r.FormValue("value"))
	case "update":
		err = s.store.UpdateSecret(app, env, key, r.FormValue("value"))
	case "delete":
		err = s.store.DeleteSecret(app, env, key)
	default:
		err = errors.New("unknown secret action")
	}
	if err != nil {
		s.redirectErr(w, r, err)
		return
	}
	s.redirectNotice(w, r, "secret changed")
}

func (s *webServer) handleRecipient(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	app, env, recipient := r.FormValue("app"), r.FormValue("env"), r.FormValue("recipient")
	var err error
	switch r.FormValue("action") {
	case "add":
		err = s.store.AddRecipient(app, env, recipient)
	case "remove":
		err = s.store.RemoveRecipient(app, env, recipient)
	default:
		err = errors.New("unknown recipient action")
	}
	if err != nil {
		s.redirectErr(w, r, err)
		return
	}
	s.redirectNotice(w, r, "recipient changed")
}

func (s *webServer) authorized(r *http.Request) bool {
	provided := r.URL.Query().Get("token")
	if provided == "" {
		provided = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if provided == "" && r.Method == http.MethodPost {
		_ = r.ParseForm()
		provided = r.FormValue("token")
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) == 1
}

func (s *webServer) render(w http.ResponseWriter, r *http.Request, data pageData) {
	namespaces, err := s.store.ListNamespaces()
	if err != nil {
		data.Error = err.Error()
	} else {
		data.Namespaces = namespaces
	}
	app, env := r.URL.Query().Get("app"), r.URL.Query().Get("env")
	if app != "" && env != "" {
		keys, meta, err := s.selected(app, env)
		if err != nil {
			data.Error = err.Error()
		} else {
			data.Selected = &selectedNamespace{App: app, Env: env, Keys: keys, Recipients: meta.Recipients}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.Execute(w, data); err != nil {
		log.Printf("render: %v", err)
	}
}

func (s *webServer) selected(app, env string) ([]secretEntry, store.EnvManifest, error) {
	keys, err := s.store.ListSecrets(app, env)
	if err != nil {
		return nil, store.EnvManifest{}, err
	}
	meta, err := s.store.Find(app, env)
	if err != nil {
		return nil, store.EnvManifest{}, err
	}
	entries := make([]secretEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, secretEntry{Key: key, Set: true})
	}
	return entries, meta, nil
}

func (s *webServer) redirectNotice(w http.ResponseWriter, r *http.Request, notice string) {
	redirectWith(w, r, "notice", notice)
}

func (s *webServer) redirectErr(w http.ResponseWriter, r *http.Request, err error) {
	redirectWith(w, r, "error", err.Error())
}

func redirectWith(w http.ResponseWriter, r *http.Request, key, value string) {
	q := r.URL.Query()
	q.Set("token", r.FormValue("token"))
	q.Set(key, value)
	if app, env := r.FormValue("app"), r.FormValue("env"); app != "" && env != "" {
		q.Set("app", app)
		q.Set("env", env)
	}
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
}

const uiTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Thimble</title>
  <style>
    :root { color-scheme: light; --ink:#17202a; --muted:#607080; --line:#d8dee5; --accent:#116466; --accent-soft:#e8f3ef; --bg:#f7f8f6; --panel:#ffffff; --warn:#9a3412; }
    * { box-sizing: border-box; }
    body { margin:0; font:14px/1.45 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; color:var(--ink); background:var(--bg); }
    header { display:flex; justify-content:space-between; align-items:center; gap:16px; padding:16px 24px; border-bottom:1px solid var(--line); background:var(--panel); }
    .brand { display:flex; gap:12px; align-items:center; min-width:0; }
    .logo { width:42px; height:42px; flex:0 0 42px; }
    .brand-copy { display:grid; gap:1px; min-width:0; }
    h1 { margin:0; font-size:20px; letter-spacing:0; }
    h2 { margin:0 0 12px; font-size:15px; }
    main { display:grid; grid-template-columns:minmax(240px,320px) 1fr; gap:20px; padding:20px 24px; max-width:1180px; margin:0 auto; }
    section, aside { background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:16px; }
    a { color:var(--accent); text-decoration:none; }
    a:hover { text-decoration:underline; }
    .muted { color:var(--muted); }
    .tip { background:var(--accent-soft); border-color:#b9d8ce; color:#20332f; }
    .tip strong { display:block; margin-bottom:3px; }
    .stack { display:grid; gap:12px; }
    .row { display:flex; gap:8px; align-items:center; flex-wrap:wrap; }
    label { display:grid; gap:5px; font-weight:600; }
    input, textarea { width:100%; min-height:38px; padding:8px 10px; border:1px solid var(--line); border-radius:6px; font:inherit; }
    textarea { min-height:84px; resize:vertical; }
    button { min-height:36px; padding:0 12px; border:0; border-radius:6px; background:var(--accent); color:white; font-weight:700; cursor:pointer; }
    button.secondary { background:#50606f; }
    button.danger { background:var(--warn); }
    ul { list-style:none; padding:0; margin:0; display:grid; gap:8px; }
    li { border-bottom:1px solid var(--line); padding-bottom:8px; }
    table { width:100%; border-collapse:collapse; }
    th, td { text-align:left; border-bottom:1px solid var(--line); padding:9px 6px; vertical-align:top; }
    th { color:var(--muted); font-size:12px; text-transform:uppercase; }
    .notice { border-color:#9bc7a8; background:#edf7ef; }
    .error { border-color:#e0a287; background:#fff3ed; }
    .pill { display:inline-block; padding:2px 8px; border:1px solid var(--line); border-radius:999px; color:var(--muted); font-size:12px; }
    .field-note { margin:4px 0 0; color:var(--muted); font-size:12px; }
    @media (max-width: 820px) { main { grid-template-columns:1fr; padding:14px; } header { padding:14px; } }
  </style>
</head>
<body>
<header>
  <div class="brand">
    <svg class="logo" viewBox="0 0 64 64" role="img" aria-label="Thimble">
      <rect width="64" height="64" rx="14" fill="#eef6f3"/>
      <path d="M20 17c2.5-4 21.5-4 24 0l4 31c.5 3.7-3.2 7-7.1 7H23.1c-3.9 0-7.6-3.3-7.1-7l4-31Z" fill="#116466"/>
      <path d="M23 20c2.1-2.1 15.9-2.1 18 0l3.6 28.2c.2 1.6-1.4 2.8-3.7 2.8H23.1c-2.3 0-3.9-1.2-3.7-2.8L23 20Z" fill="#ffffff" opacity=".88"/>
      <path d="M24 24h16M24.5 30h15M25 36h14M26 42h12" stroke="#116466" stroke-width="2" stroke-linecap="round" opacity=".55"/>
      <path d="M33 11l18 18" stroke="#17202a" stroke-width="3" stroke-linecap="round"/>
      <path d="M49.5 27.5l4 4" stroke="#17202a" stroke-width="5" stroke-linecap="round"/>
      <circle cx="28" cy="27" r="1.8" fill="#116466"/><circle cx="36" cy="27" r="1.8" fill="#116466"/><circle cx="31" cy="34" r="1.8" fill="#116466"/><circle cx="39" cy="34" r="1.8" fill="#116466"/><circle cx="29" cy="41" r="1.8" fill="#116466"/><circle cx="37" cy="41" r="1.8" fill="#116466"/>
    </svg>
    <div class="brand-copy">
      <h1>Thimble</h1>
      <span class="muted">local age-backed secrets</span>
    </div>
  </div>
  <span class="pill">values stay redacted</span>
</header>
<main>
  <aside class="stack">
    {{if .Notice}}<section class="notice">{{.Notice}}</section>{{end}}
    {{if .Error}}<section class="error">{{.Error}}</section>{{end}}
    <section class="stack">
      <h2>Namespaces</h2>
      <ul>
      {{range .Namespaces}}
        <li><a href="/?token={{$.Token}}&app={{.App}}&env={{.Env}}"><strong>{{.App}}</strong> / {{.Env}}</a><br><span class="muted">{{.Recipients}} recipients · {{.UpdatedAt}}</span></li>
      {{else}}
        <li class="muted">No namespaces yet.</li>
      {{end}}
      </ul>
    </section>
    <section class="tip">
      <strong>Namespace model</strong>
      <span>Use one namespace per application and environment. A production bundle and a staging bundle can share key names while keeping recipients separate.</span>
    </section>
    <section class="stack">
      <h2>Create Namespace</h2>
      <form class="stack" action="/namespace" method="post">
        <input type="hidden" name="token" value="{{.Token}}">
        <label>Application <input name="app" autocomplete="off" required></label>
        <label>Environment <input name="env" autocomplete="off" required></label>
        <label>Recipients <textarea name="recipients" spellcheck="false" required></textarea><span class="field-note">Add verified age public recipients only. One per line or comma-separated.</span></label>
        <button>Create</button>
      </form>
    </section>
    <section class="tip">
      <strong>Peer handoff</strong>
      <span>Commit encrypted bundles, never identities. Add a peer by verifying their recipient out of band, then re-encrypt with Recipient Add.</span>
    </section>
  </aside>
  <section class="stack">
    {{if .Selected}}
      <div class="row">
        <h2>{{.Selected.App}} / {{.Selected.Env}}</h2>
        <span class="pill">values redacted</span>
      </div>
      <section class="tip">
        <strong>Safe entry</strong>
        <span>Use the CLI masked prompt or pipe for high-risk values. The browser stores updates but never shows existing secret values back to you.</span>
      </section>
      <form class="row" action="/secret" method="post">
        <input type="hidden" name="token" value="{{$.Token}}">
        <input type="hidden" name="app" value="{{.Selected.App}}">
        <input type="hidden" name="env" value="{{.Selected.Env}}">
        <input type="hidden" name="action" value="create">
        <input name="key" placeholder="NEW_KEY" autocomplete="off" required>
        <input name="value" placeholder="secret value" type="password" required>
        <button>Create</button>
      </form>
      <table>
        <thead><tr><th>Key</th><th>Value</th><th>Actions</th></tr></thead>
        <tbody>
          {{range .Selected.Keys}}
          <tr>
            <td><code>{{.Key}}</code></td>
            <td class="muted">redacted</td>
            <td>
              <form class="row" action="/secret" method="post">
                <input type="hidden" name="token" value="{{$.Token}}">
                <input type="hidden" name="app" value="{{$.Selected.App}}">
                <input type="hidden" name="env" value="{{$.Selected.Env}}">
                <input type="hidden" name="key" value="{{.Key}}">
                <input name="value" placeholder="new value" type="password">
                <button class="secondary" name="action" value="update">Update</button>
                <button class="danger" name="action" value="delete">Delete</button>
              </form>
            </td>
          </tr>
          {{end}}
        </tbody>
      </table>
      <section class="stack">
        <h2>Recipients</h2>
        <p class="field-note">Recipient changes re-encrypt this bundle. Keep at least one offline recovery recipient before removing an operator.</p>
        <ul>
        {{range .Selected.Recipients}}
          <li>
            <form class="row" action="/recipient" method="post">
              <input type="hidden" name="token" value="{{$.Token}}">
              <input type="hidden" name="app" value="{{$.Selected.App}}">
              <input type="hidden" name="env" value="{{$.Selected.Env}}">
              <input type="hidden" name="recipient" value="{{.}}">
              <code>{{.}}</code>
              <button class="danger" name="action" value="remove">Remove</button>
            </form>
          </li>
        {{end}}
        </ul>
        <form class="row" action="/recipient" method="post">
          <input type="hidden" name="token" value="{{$.Token}}">
          <input type="hidden" name="app" value="{{.Selected.App}}">
          <input type="hidden" name="env" value="{{.Selected.Env}}">
          <input name="recipient" placeholder="age1..." required>
          <button name="action" value="add">Add</button>
        </form>
      </section>
    {{else}}
      <h2>Select a Namespace</h2>
      <p class="muted">Create or select an application/environment namespace to manage redacted keys.</p>
      <section class="tip">
        <strong>Good first flow</strong>
        <span>Create a namespace, add one operator recipient and one recovery recipient, then set values through a pipe or masked prompt.</span>
      </section>
    {{end}}
  </section>
</main>
</body>
</html>`
