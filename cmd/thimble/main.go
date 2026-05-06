package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
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
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultStoreDir = "secrets"
	defaultAddr     = "127.0.0.1:8787"
	manifestName    = "thimble.json"
)

var keyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

type cliConfig struct {
	storeDir string
	identity string
}

type manifest struct {
	Version int                    `json:"version"`
	Apps    map[string]appManifest `json:"apps"`
}

type appManifest struct {
	Environments map[string]envManifest `json:"environments"`
}

type envManifest struct {
	Format     string   `json:"format"`
	File       string   `json:"file"`
	Recipients []string `json:"recipients"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

type store struct {
	root     string
	agePath  string
	identity string
	now      func() time.Time
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
		return runWrite(st, rest[1:], stdout, false)
	case "update":
		return runWrite(st, rest[1:], stdout, true)
	case "set":
		return runSet(st, rest[1:], stdout)
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
  create <app> <env> KEY [VALUE]          create one secret key
  update <app> <env> KEY [VALUE]          update one existing secret key
  set <app> <env> KEY [VALUE]             create or update one secret key
  delete <app> <env> KEY                  delete one secret key
  list <app> <env>                        list keys only, never values
  render <app> <env> --format dotenv      render decrypted dotenv to stdout
  web [--addr 127.0.0.1:8787]             run the local web UI

Values omitted on create/update/set are read from stdin.`)
}

func runInit(st *store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var recipients multiFlag
	fs.Var(&recipients, "recipient", "age recipient; repeatable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: thimble init <app> <env> --recipient age1...")
	}
	if len(recipients) == 0 {
		return errors.New("init requires at least one --recipient")
	}
	app, env := fs.Arg(0), fs.Arg(1)
	if err := st.Init(app, env, recipients); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "initialized %s/%s\n", app, env)
	return nil
}

func runRecipient(st *store, args []string, stdout, stderr io.Writer) error {
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

func runWrite(st *store, args []string, stdout io.Writer, requireExisting bool) error {
	if len(args) != 3 && len(args) != 4 {
		if requireExisting {
			return errors.New("usage: thimble update <app> <env> <KEY> [VALUE]")
		}
		return errors.New("usage: thimble create <app> <env> <KEY> [VALUE]")
	}
	value, err := valueArg(args)
	if err != nil {
		return err
	}
	app, env, key := args[0], args[1], args[2]
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

func runSet(st *store, args []string, stdout io.Writer) error {
	if len(args) != 3 && len(args) != 4 {
		return errors.New("usage: thimble set <app> <env> <KEY> [VALUE]")
	}
	value, err := valueArg(args)
	if err != nil {
		return err
	}
	app, env, key := args[0], args[1], args[2]
	if err := st.SetSecret(app, env, key, value); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s\n", key, app, env)
	return nil
}

func runDelete(st *store, args []string, stdout io.Writer) error {
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

func runList(st *store, args []string, stdout io.Writer) error {
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

func runRender(st *store, args []string, stdout, stderr io.Writer) error {
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

func runWeb(st *store, args []string, stdout, stderr io.Writer) error {
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

func newStore(root, identity string) *store {
	return &store{root: root, agePath: "age", identity: identity, now: time.Now}
}

func (s *store) Init(app, env string, recipients []string) error {
	if err := validateName("app", app); err != nil {
		return err
	}
	if err := validateName("environment", env); err != nil {
		return err
	}
	if err := validateRecipients(recipients); err != nil {
		return err
	}
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	if _, ok := m.Apps[app]; !ok {
		m.Apps[app] = appManifest{Environments: map[string]envManifest{}}
	}
	if _, ok := m.Apps[app].Environments[env]; ok {
		return fmt.Errorf("%s/%s already exists", app, env)
	}
	now := s.now().UTC().Format(time.RFC3339)
	envMeta := envManifest{
		Format:     "dotenv",
		File:       filepath.ToSlash(filepath.Join(app, env+".env.age")),
		Recipients: sortedUnique(recipients),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.encryptAndWrite(envMeta, ""); err != nil {
		return err
	}
	m.Apps[app].Environments[env] = envMeta
	return s.saveManifest(m)
}

func (s *store) AddRecipient(app, env, recipient string) error {
	if err := validateRecipient(recipient); err != nil {
		return err
	}
	return s.rewriteEnv(app, env, func(meta *envManifest, values map[string]string) error {
		meta.Recipients = sortedUnique(append(meta.Recipients, recipient))
		return nil
	})
}

func (s *store) RemoveRecipient(app, env, recipient string) error {
	return s.rewriteEnv(app, env, func(meta *envManifest, values map[string]string) error {
		next := meta.Recipients[:0]
		for _, existing := range meta.Recipients {
			if existing != recipient {
				next = append(next, existing)
			}
		}
		if len(next) == len(meta.Recipients) {
			return fmt.Errorf("recipient not found")
		}
		if len(next) == 0 {
			return errors.New("cannot remove the last recipient")
		}
		meta.Recipients = sortedUnique(next)
		return nil
	})
}

func (s *store) CreateSecret(app, env, key, value string) error {
	return s.rewriteEnv(app, env, func(meta *envManifest, values map[string]string) error {
		if err := validateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; ok {
			return fmt.Errorf("%s already exists; use update or set", key)
		}
		values[key] = value
		return nil
	})
}

func (s *store) UpdateSecret(app, env, key, value string) error {
	return s.rewriteEnv(app, env, func(meta *envManifest, values map[string]string) error {
		if err := validateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; !ok {
			return fmt.Errorf("%s does not exist; use create or set", key)
		}
		values[key] = value
		return nil
	})
}

func (s *store) SetSecret(app, env, key, value string) error {
	return s.rewriteEnv(app, env, func(meta *envManifest, values map[string]string) error {
		if err := validateKey(key); err != nil {
			return err
		}
		values[key] = value
		return nil
	})
}

func (s *store) DeleteSecret(app, env, key string) error {
	return s.rewriteEnv(app, env, func(meta *envManifest, values map[string]string) error {
		if err := validateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; !ok {
			return fmt.Errorf("%s does not exist", key)
		}
		delete(values, key)
		return nil
	})
}

func (s *store) ListSecrets(app, env string) ([]string, error) {
	values, _, err := s.readEnv(app, env)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *store) Render(app, env string) (string, error) {
	values, _, err := s.readEnv(app, env)
	if err != nil {
		return "", err
	}
	return encodeDotenv(values), nil
}

func (s *store) ListNamespaces() ([]namespaceView, error) {
	m, err := s.loadManifest()
	if err != nil {
		return nil, err
	}
	var views []namespaceView
	for app, appMeta := range m.Apps {
		for env, envMeta := range appMeta.Environments {
			views = append(views, namespaceView{
				App:        app,
				Env:        env,
				Recipients: len(envMeta.Recipients),
				UpdatedAt:  envMeta.UpdatedAt,
			})
		}
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].App == views[j].App {
			return views[i].Env < views[j].Env
		}
		return views[i].App < views[j].App
	})
	return views, nil
}

func (s *store) readEnv(app, env string) (map[string]string, envManifest, error) {
	meta, err := s.findEnv(app, env)
	if err != nil {
		return nil, envManifest{}, err
	}
	plain, err := s.decrypt(meta)
	if err != nil {
		return nil, envManifest{}, err
	}
	values, err := parseDotenv(plain)
	if err != nil {
		return nil, envManifest{}, err
	}
	return values, meta, nil
}

func (s *store) rewriteEnv(app, env string, edit func(*envManifest, map[string]string) error) error {
	if err := validateName("app", app); err != nil {
		return err
	}
	if err := validateName("environment", env); err != nil {
		return err
	}
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	appMeta, ok := m.Apps[app]
	if !ok {
		return fmt.Errorf("%s/%s is not initialized", app, env)
	}
	meta, ok := appMeta.Environments[env]
	if !ok {
		return fmt.Errorf("%s/%s is not initialized", app, env)
	}
	plain, err := s.decrypt(meta)
	if err != nil {
		return err
	}
	values, err := parseDotenv(plain)
	if err != nil {
		return err
	}
	if err := edit(&meta, values); err != nil {
		return err
	}
	meta.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	if err := s.encryptAndWrite(meta, encodeDotenv(values)); err != nil {
		return err
	}
	appMeta.Environments[env] = meta
	m.Apps[app] = appMeta
	return s.saveManifest(m)
}

func (s *store) findEnv(app, env string) (envManifest, error) {
	if err := validateName("app", app); err != nil {
		return envManifest{}, err
	}
	if err := validateName("environment", env); err != nil {
		return envManifest{}, err
	}
	m, err := s.loadManifest()
	if err != nil {
		return envManifest{}, err
	}
	appMeta, ok := m.Apps[app]
	if !ok {
		return envManifest{}, fmt.Errorf("%s/%s is not initialized", app, env)
	}
	meta, ok := appMeta.Environments[env]
	if !ok {
		return envManifest{}, fmt.Errorf("%s/%s is not initialized", app, env)
	}
	return meta, nil
}

func (s *store) loadManifest() (manifest, error) {
	m := manifest{Version: 1, Apps: map[string]appManifest{}}
	path := filepath.Join(s.root, manifestName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if m.Apps == nil {
		m.Apps = map[string]appManifest{}
	}
	return m, nil
}

func (s *store) saveManifest(m manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return atomicWrite(filepath.Join(s.root, manifestName), b, 0o600)
}

func (s *store) encryptAndWrite(meta envManifest, plain string) error {
	if err := validateRecipients(meta.Recipients); err != nil {
		return err
	}
	var out bytes.Buffer
	args := []string{"-a"}
	for _, recipient := range meta.Recipients {
		args = append(args, "-r", recipient)
	}
	cmd := exec.CommandContext(context.Background(), s.agePath, args...)
	cmd.Stdin = strings.NewReader(plain)
	cmd.Stdout = &out
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("age encrypt failed: %s", redact(stderr.String()))
	}
	return atomicWrite(filepath.Join(s.root, filepath.FromSlash(meta.File)), out.Bytes(), 0o600)
}

func (s *store) decrypt(meta envManifest) (string, error) {
	args := []string{"-d"}
	if s.identity != "" {
		args = append(args, "-i", s.identity)
	}
	args = append(args, filepath.Join(s.root, filepath.FromSlash(meta.File)))
	cmd := exec.CommandContext(context.Background(), s.agePath, args...)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("age decrypt failed: %s", redact(stderr.String()))
	}
	return out.String(), nil
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func parseDotenv(input string) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(input))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid dotenv line %d", lineNo)
		}
		key := strings.TrimSpace(parts[0])
		if err := validateKey(key); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		value, err := parseDotenvValue(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func parseDotenvValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] != '"' {
		if strings.ContainsAny(value, "\r\n") {
			return "", errors.New("unquoted multiline values are not supported")
		}
		return value, nil
	}
	var out strings.Builder
	escaped := false
	for i := 1; i < len(value); i++ {
		ch := value[i]
		if escaped {
			switch ch {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '\\', '"':
				out.WriteByte(ch)
			default:
				out.WriteByte(ch)
			}
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			if strings.TrimSpace(value[i+1:]) != "" {
				return "", errors.New("trailing content after quoted value")
			}
			return out.String(), nil
		}
		out.WriteByte(ch)
	}
	return "", errors.New("unterminated quoted value")
}

func encodeDotenv(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	for _, key := range keys {
		out.WriteString(key)
		out.WriteByte('=')
		out.WriteString(quoteDotenvValue(values[key]))
		out.WriteByte('\n')
	}
	return out.String()
}

func quoteDotenvValue(value string) string {
	if value == "" {
		return `""`
	}
	safe := true
	for _, r := range value {
		if !(r == '_' || r == '-' || r == '.' || r == '/' || r == ':' || r == '@' || r == '%' || r == '+' || r == ',' || r == '=' || r == '~' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			safe = false
			break
		}
	}
	if safe {
		return value
	}
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + replacer.Replace(value) + `"`
}

func valueArg(args []string) (string, error) {
	if len(args) == 4 {
		return args[3], nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func validateName(kind, name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("invalid %s %q; use letters, digits, dot, underscore, or dash", kind, name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid %s %q", kind, name)
	}
	return nil
}

func validateKey(key string) error {
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("invalid key %q; use dotenv-style uppercase names", key)
	}
	return nil
}

func validateRecipients(recipients []string) error {
	if len(recipients) == 0 {
		return errors.New("at least one recipient is required")
	}
	for _, recipient := range recipients {
		if err := validateRecipient(recipient); err != nil {
			return err
		}
	}
	return nil
}

func validateRecipient(recipient string) error {
	if strings.TrimSpace(recipient) != recipient || recipient == "" {
		return errors.New("recipient cannot be empty or padded")
	}
	if strings.ContainsAny(recipient, "\r\n\t ") {
		return errors.New("recipient cannot contain whitespace")
	}
	return nil
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func redact(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "no details"
	}
	if len(s) > 240 {
		s = s[:240] + "..."
	}
	return s
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

type namespaceView struct {
	App        string
	Env        string
	Recipients int
	UpdatedAt  string
}

type webServer struct {
	store     *store
	token     string
	templates *template.Template
}

type pageData struct {
	Token      string
	Error      string
	Notice     string
	Namespaces []namespaceView
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

func (s *webServer) selected(app, env string) ([]secretEntry, envManifest, error) {
	keys, err := s.store.ListSecrets(app, env)
	if err != nil {
		return nil, envManifest{}, err
	}
	meta, err := s.store.findEnv(app, env)
	if err != nil {
		return nil, envManifest{}, err
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
    :root { color-scheme: light; --ink:#17202a; --muted:#607080; --line:#d8dee5; --accent:#116466; --bg:#f7f8f6; --panel:#ffffff; --warn:#9a3412; }
    * { box-sizing: border-box; }
    body { margin:0; font:14px/1.45 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; color:var(--ink); background:var(--bg); }
    header { display:flex; justify-content:space-between; align-items:center; padding:18px 24px; border-bottom:1px solid var(--line); background:var(--panel); }
    h1 { margin:0; font-size:20px; letter-spacing:0; }
    h2 { margin:0 0 12px; font-size:15px; }
    main { display:grid; grid-template-columns:minmax(240px,320px) 1fr; gap:20px; padding:20px 24px; max-width:1180px; margin:0 auto; }
    section, aside { background:var(--panel); border:1px solid var(--line); border-radius:8px; padding:16px; }
    a { color:var(--accent); text-decoration:none; }
    a:hover { text-decoration:underline; }
    .muted { color:var(--muted); }
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
    @media (max-width: 820px) { main { grid-template-columns:1fr; padding:14px; } header { padding:14px; } }
  </style>
</head>
<body>
<header>
  <h1>Thimble</h1>
  <span class="muted">local age-backed secrets</span>
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
    <section class="stack">
      <h2>Create Namespace</h2>
      <form class="stack" action="/namespace" method="post">
        <input type="hidden" name="token" value="{{.Token}}">
        <label>Application <input name="app" autocomplete="off" required></label>
        <label>Environment <input name="env" autocomplete="off" required></label>
        <label>Recipients <textarea name="recipients" spellcheck="false" required></textarea></label>
        <button>Create</button>
      </form>
    </section>
  </aside>
  <section class="stack">
    {{if .Selected}}
      <div class="row">
        <h2>{{.Selected.App}} / {{.Selected.Env}}</h2>
        <span class="pill">values redacted</span>
      </div>
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
    {{end}}
  </section>
</main>
</body>
</html>`
