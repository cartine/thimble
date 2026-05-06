package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNamespacedCRUDAndRender(t *testing.T) {
	st := testStore(t)

	if err := st.Init("koja", "production", []string{"age1operator"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.CreateSecret("koja", "production", "POSTGRES_PASSWORD", "alpha secret"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.UpdateSecret("koja", "production", "POSTGRES_PASSWORD", "bravo secret"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := st.SetSecret("koja", "staging", "IGNORED", "value"); err == nil {
		t.Fatalf("set against uninitialized namespace succeeded")
	}

	keys, err := st.ListSecrets("koja", "production")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got, want := strings.Join(keys, ","), "POSTGRES_PASSWORD"; got != want {
		t.Fatalf("keys = %q, want %q", got, want)
	}

	rendered, err := st.Render("koja", "production")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, "POSTGRES_PASSWORD=\"bravo secret\"") {
		t.Fatalf("rendered dotenv missing updated value: %q", rendered)
	}

	ciphertext, err := os.ReadFile(filepath.Join(st.root, "koja", "production.env.age"))
	if err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	if strings.Contains(string(ciphertext), "bravo secret") {
		t.Fatalf("ciphertext contains plaintext secret")
	}

	if err := st.DeleteSecret("koja", "production", "POSTGRES_PASSWORD"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rendered, err = st.Render("koja", "production")
	if err != nil {
		t.Fatalf("render after delete: %v", err)
	}
	if rendered != "" {
		t.Fatalf("render after delete = %q, want empty", rendered)
	}
}

func TestRecipientsRewriteBundle(t *testing.T) {
	st := testStore(t)

	if err := st.Init("api", "prod", []string{"age1alice"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("api", "prod", "TOKEN", "topsecret"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := st.AddRecipient("api", "prod", "age1bob"); err != nil {
		t.Fatalf("add recipient: %v", err)
	}
	meta, err := st.findEnv("api", "prod")
	if err != nil {
		t.Fatalf("find env: %v", err)
	}
	if got, want := strings.Join(meta.Recipients, ","), "age1alice,age1bob"; got != want {
		t.Fatalf("recipients = %q, want %q", got, want)
	}
	if err := st.RemoveRecipient("api", "prod", "age1alice"); err != nil {
		t.Fatalf("remove recipient: %v", err)
	}
	meta, err = st.findEnv("api", "prod")
	if err != nil {
		t.Fatalf("find env after remove: %v", err)
	}
	if got, want := strings.Join(meta.Recipients, ","), "age1bob"; got != want {
		t.Fatalf("recipients = %q, want %q", got, want)
	}
	rendered, err := st.Render("api", "prod")
	if err != nil {
		t.Fatalf("render after recipient changes: %v", err)
	}
	if !strings.Contains(rendered, "TOKEN=topsecret") {
		t.Fatalf("rendered dotenv lost secret: %q", rendered)
	}
}

func TestWebUIRequiresTokenAndRedactsValues(t *testing.T) {
	st := testStore(t)
	if err := st.Init("webapp", "dev", []string{"age1operator"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("webapp", "dev", "API_KEY", "browser secret"); err != nil {
		t.Fatalf("set: %v", err)
	}

	server := &webServer{
		store:     st,
		token:     "test-token",
		templates: template.Must(template.New("ui").Parse(uiTemplate)),
	}
	mux := http.NewServeMux()
	server.routes(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	authorized := httptest.NewRecorder()
	mux.ServeHTTP(authorized, httptest.NewRequest(http.MethodGet, "/?token=test-token&app=webapp&env=dev", nil))
	body := authorized.Body.String()
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", authorized.Code, body)
	}
	if !strings.Contains(body, "API_KEY") {
		t.Fatalf("web UI did not show key: %s", body)
	}
	if strings.Contains(body, "browser secret") {
		t.Fatalf("web UI leaked secret value")
	}

	form := url.Values{
		"token":  {"test-token"},
		"app":    {"webapp"},
		"env":    {"dev"},
		"key":    {"API_KEY"},
		"value":  {"new browser secret"},
		"action": {"update"},
	}
	update := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/secret", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(update, req)
	if update.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d, want %d", update.Code, http.StatusSeeOther)
	}
	rendered, err := st.Render("webapp", "dev")
	if err != nil {
		t.Fatalf("render after web update: %v", err)
	}
	if !strings.Contains(rendered, "API_KEY=\"new browser secret\"") {
		t.Fatalf("web update did not persist: %q", rendered)
	}
}

func testStore(t *testing.T) *store {
	t.Helper()
	root := t.TempDir()
	fakeAge := filepath.Join(root, "age")
	script := `#!/bin/sh
set -eu
if [ "${1:-}" = "-d" ]; then
  for last do :; done
  sed '1d' "$last" | tr 'A-Za-z' 'N-ZA-Mn-za-m'
else
  printf 'FAKE AGE CIPHERTEXT\n'
  tr 'A-Za-z' 'N-ZA-Mn-za-m'
fi
`
	if err := os.WriteFile(fakeAge, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake age: %v", err)
	}
	st := newStore(filepath.Join(root, "secrets"), "")
	st.agePath = fakeAge
	st.now = func() time.Time { return time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC) }
	return st
}
