// Package web serves the local Thimble UI. Trust boundary: this is
// the only package that listens on a network socket; it accepts only
// loopback by default, gates every handler on a constant-time token
// check, and never echoes existing secret values back to the
// operator's browser.
package web

import (
	"crypto/subtle"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"

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
// to mount handlers.
type Server struct {
	store     *store.Store
	token     string
	templates *template.Template
}

// New returns a Server backed by st and gated on token. The HTML
// template is embedded; callers do not pass one in.
func New(st *store.Store, token string) *Server {
	return &Server{store: st, token: token, templates: Template()}
}

// Routes registers the UI's handlers on mux.
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/namespace", s.handleNamespace)
	mux.HandleFunc("/secret", s.handleSecret)
	mux.HandleFunc("/recipient", s.handleRecipient)
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
	Keys       []SecretEntry
	Recipients []string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writePage(w, r, pageData{
		Token:  s.token,
		Notice: r.URL.Query().Get("notice"),
		Error:  r.URL.Query().Get("error"),
	})
}

func (s *Server) handleNamespace(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
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
	if err := s.runSecretAction(r); err != nil {
		s.redirectErr(w, r, err)
		return
	}
	s.redirectNotice(w, r, "secret changed")
}

func (s *Server) runSecretAction(r *http.Request) error {
	app, env, key := r.FormValue("app"), r.FormValue("env"), r.FormValue("key")
	switch r.FormValue("action") {
	case "create":
		return s.store.CreateSecret(app, env, key, r.FormValue("value"))
	case "update":
		return s.store.UpdateSecret(app, env, key, r.FormValue("value"))
	case "delete":
		return s.store.DeleteSecret(app, env, key)
	default:
		return errors.New("unknown secret action")
	}
}

func (s *Server) handleRecipient(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) authorized(r *http.Request) bool {
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
	if err := s.templates.Execute(w, data); err != nil {
		log.Printf("writePage: %v", err)
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
