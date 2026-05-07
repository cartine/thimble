package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/web"
)

func TestWebUIRequiresTokenAndRedactsValues(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("webapp", "dev", []string{"age1operator"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("webapp", "dev", "API_KEY", "browser secret"); err != nil {
		t.Fatalf("set: %v", err)
	}

	server := web.New(st, "test-token")
	mux := http.NewServeMux()
	server.Routes(mux)

	if got := getStatus(mux, "/"); got != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", got, http.StatusUnauthorized)
	}

	body, status := getBody(mux, "/?token=test-token&app=webapp&env=dev")
	if status != http.StatusOK {
		t.Fatalf("authorized status = %d body=%s", status, body)
	}
	if !strings.Contains(body, "API_KEY") {
		t.Fatalf("web UI did not show key: %s", body)
	}
	if !strings.Contains(body, `aria-label="Thimble"`) ||
		!strings.Contains(body, "Safe entry") {
		t.Fatalf("web UI polish elements missing: %s", body)
	}
	if strings.Contains(body, "browser secret") {
		t.Fatalf("web UI leaked secret value")
	}

	if status := postUpdateForm(mux); status != http.StatusSeeOther {
		t.Fatalf("update status = %d, want %d", status, http.StatusSeeOther)
	}
	rendered, err := st.Render("webapp", "dev")
	if err != nil {
		t.Fatalf("render after web update: %v", err)
	}
	if !strings.Contains(rendered, "API_KEY=\"new browser secret\"") {
		t.Fatalf("web update did not persist: %q", rendered)
	}
}

func getStatus(mux http.Handler, target string) int {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Code
}

func getBody(mux http.Handler, target string) (string, int) {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Body.String(), rec.Code
}

func postUpdateForm(mux http.Handler) int {
	form := url.Values{
		"token":  {"test-token"},
		"app":    {"webapp"},
		"env":    {"dev"},
		"key":    {"API_KEY"},
		"value":  {"new browser secret"},
		"action": {"update"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost, "/secret", strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	st := store.New(filepath.Join(root, "secrets"), "")
	st.SetAge(age.New(fakeAge, ""))
	st.SetClock(func() time.Time {
		return time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	})
	return st
}

func writeFakeAge(t *testing.T, root string) string {
	t.Helper()
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
	return fakeAge
}
