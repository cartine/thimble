// Package web serves the local Thimble UI. Trust boundary: this is
// the only package that listens on a network socket; it accepts only
// loopback by default, gates every handler on a constant-time token
// check carried in an HttpOnly session cookie (K-30), and never echoes
// existing secret values back to the operator's browser.
package web

import (
	"crypto/subtle"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/cartine/thimble/internal/store"
)

// SecretEntry names one key visible in a namespace; the Set bool is a
// hook for future "value present?" UI without ever exposing the value.
type SecretEntry struct {
	Key string
	Set bool
}

// Server bundles the Thimble store, the UI access token, and the
// parsed HTML template. Construct one with New, then call Routes(mux)
// to mount handlers. The token is mutex-guarded (K-33) so rotation
// can run concurrently with handler reads.
type Server struct {
	store     *store.Store
	mu        sync.RWMutex
	token     string
	loopback  bool
	templates *template.Template
	// activity is a 1-buffered channel signaling that an authorized
	// request just landed. RunIdleRotation drains it to reset the
	// idle timer. Allocated in New so request handlers can publish
	// without coordinating with the watcher goroutine; if no watcher
	// is running, the publish is dropped non-blockingly.
	activity chan struct{}
}

// New returns a Server backed by st and gated on token. The HTML
// template is embedded; callers do not pass one in. The loopback bool
// drives the Secure attribute on the session cookie: true when bound
// to 127.0.0.1/::1/localhost (where browsers reject Secure on HTTP).
func New(st *store.Store, token string, loopback bool) *Server {
	return &Server{
		store:     st,
		token:     token,
		loopback:  loopback,
		templates: Template(),
		activity:  make(chan struct{}, 1),
	}
}

// currentToken returns the active token under read-lock. Use this
// instead of `s.token` everywhere; rotation flips the value under
// the corresponding write-lock in setToken.
func (s *Server) currentToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

// setToken replaces the active token under write-lock. After this
// returns, every cookie issued against the previous token will fail
// the constant-time compare in hasValidSession and the operator must
// re-login with the new value.
func (s *Server) setToken(token string) {
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
}

// Routes registers the UI's handlers on mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/namespace", s.handleNamespace)
	mux.HandleFunc("/secret", s.handleSecret)
	mux.HandleFunc("/recipient", s.handleRecipient)
}

type pageData struct {
	Error      string
	Notice     string
	Namespaces []store.NamespaceView
	Selected   *selectedNamespace
}

type loginData struct {
	Error string
}

type selectedNamespace struct {
	App        string
	Env        string
	Keys       []SecretEntry
	Recipients []string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !s.hasValidSession(r) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writePage(w, r, pageData{
		Notice: r.URL.Query().Get("notice"),
		Error:  r.URL.Query().Get("error"),
	})
}

// handleLogin renders the one-field token form on GET and validates a
// posted token on POST. Constant-time compare gates the cookie set.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.hasValidSession(r) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		s.writeLogin(w, loginData{})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.writeLogin(w, loginData{Error: "invalid form"})
			return
		}
		provided := r.FormValue("token")
		current := s.currentToken()
		if subtle.ConstantTimeCompare([]byte(provided), []byte(current)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			s.writeLogin(w, loginData{Error: "invalid token"})
			return
		}
		s.setSessionCookie(w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogout clears the session cookie and redirects back to /login.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleNamespace(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r) {
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

func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r) {
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
	if msg, ok := strictPlaintextReject(r); ok {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}
	if err := s.runSecretAction(r); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	s.redirectNotice(w, r, "secret changed")
}

// strictPlaintextReject implements K-34: web UI never accepts secret
// values. If a create/update arrives with a non-empty `value` form
// field, return a 400 message that points the operator at the CLI
// command — without echoing the submitted value.
func strictPlaintextReject(r *http.Request) (string, bool) {
	action := r.FormValue("action")
	if action != "create" && action != "update" {
		return "", false
	}
	if r.FormValue("value") == "" {
		return "", false
	}
	app := safeArg(r.FormValue("app"))
	env := safeArg(r.FormValue("env"))
	key := safeArg(r.FormValue("key"))
	cmd := "thimble set " + app + " " + env + " " + key
	return "web UI does not accept secret values; use the CLI:\n  " + cmd, true
}

// safeArg returns a placeholder when an app/env/key form field is
// missing, so the suggested CLI command is still copy-pasteable.
func safeArg(v string) string {
	if v == "" {
		return "<missing>"
	}
	return v
}

func (s *Server) runSecretAction(r *http.Request) error {
	app, env, key := r.FormValue("app"), r.FormValue("env"), r.FormValue("key")
	switch r.FormValue("action") {
	case "create", "update":
		// K-34: web UI is strict-mode. The CLI is the only path that
		// accepts plaintext values. Any non-empty value would have
		// been rejected upstream by strictPlaintextReject; we treat
		// this branch as a no-op to keep the form valid for the
		// "delete" case.
		return errors.New("web UI cannot create or update values; use the CLI")
	case "delete":
		return s.store.DeleteSecret(app, env, key)
	default:
		return errors.New("unknown secret action")
	}
}

func (s *Server) handleRecipient(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r) {
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
	if err := s.runRecipientAction(r); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	s.redirectNotice(w, r, "recipient changed")
}

func (s *Server) runRecipientAction(r *http.Request) error {
	app, env, recipient := r.FormValue("app"), r.FormValue("env"), r.FormValue("recipient")
	switch r.FormValue("action") {
	case "add":
		return s.store.AddRecipient(app, env, recipient)
	case "remove":
		return s.store.RemoveRecipient(app, env, recipient)
	default:
		return errors.New("unknown recipient action")
	}
}

// requireSession denies state-changing routes that lack a valid
// session cookie with 401 (rather than redirect-to-login). Returns
// true when the request may proceed.
func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) bool {
	if s.hasValidSession(r) {
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

// writePage renders the HTML page. Renamed from `render` so that the
// CLI's render verb (TAXONOMY: store.Render, runRender) and the page
// writer no longer share a name.
func (s *Server) writePage(w http.ResponseWriter, r *http.Request, data pageData) {
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
			data.Selected = &selectedNamespace{
				App:        app,
				Env:        env,
				Keys:       keys,
				Recipients: meta.Recipients,
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "ui", data); err != nil {
		log.Printf("writePage: %v", err)
	}
}

// writeLogin renders the standalone login page.
func (s *Server) writeLogin(w http.ResponseWriter, data loginData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "login", data); err != nil {
		log.Printf("writeLogin: %v", err)
	}
}

func (s *Server) selected(app, env string) ([]SecretEntry, store.EnvManifest, error) {
	keys, err := s.store.ListSecrets(app, env)
	if err != nil {
		return nil, store.EnvManifest{}, err
	}
	meta, err := s.store.Find(app, env)
	if err != nil {
		return nil, store.EnvManifest{}, err
	}
	entries := make([]SecretEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, SecretEntry{Key: key, Set: true})
	}
	return entries, meta, nil
}

func (s *Server) redirectNotice(w http.ResponseWriter, r *http.Request, notice string) {
	redirectWith(w, r, "notice", notice)
}

func (s *Server) redirectErr(w http.ResponseWriter, r *http.Request, err error) {
	redirectWith(w, r, "error", err.Error())
}

// redirectWith preserves the namespace selection but never propagates
// the token: the session cookie carries auth.
func redirectWith(w http.ResponseWriter, r *http.Request, key, value string) {
	q := r.URL.Query()
	q.Set(key, value)
	if app, env := r.FormValue("app"), r.FormValue("env"); app != "" && env != "" {
		q.Set("app", app)
		q.Set("env", env)
	}
	http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
}
