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

func TestWebUICookieFlowAndRedaction(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("webapp", "dev", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("webapp", "dev", "API_KEY", "browser secret"); err != nil {
		t.Fatalf("set: %v", err)
	}

	server := web.New(st, "test-token", true)
	mux := http.NewServeMux()
	server.Routes(mux)
	// Wrap with NoStoreMiddleware so cache-control headers are
	// asserted as part of the live cookie flow (K-32).
	handler := web.NoStoreMiddleware(mux)

	t.Run("missing cookie redirects or 401s", func(t *testing.T) {
		assertMissingCookieResponses(t, handler)
	})
	t.Run("wrong token rejected", func(t *testing.T) {
		assertWrongTokenRejected(t, handler)
	})
	cookie := loginAndExtractCookie(t, handler, "test-token")
	t.Run("correct token sets cookie attributes", func(t *testing.T) {
		assertCookieAttrs(t, cookie, false)
	})
	t.Run("authorized session shows redacted UI", func(t *testing.T) {
		assertAuthorizedView(t, handler, cookie)
	})
	t.Run("strict mode rejects plaintext POST", func(t *testing.T) {
		assertStrictModeRejection(t, handler, cookie)
	})
	t.Run("authorized delete still works", func(t *testing.T) {
		assertAuthorizedDelete(t, handler, st, cookie)
	})
	t.Run("logout clears cookie", func(t *testing.T) {
		assertLogoutClears(t, handler, cookie)
	})
	t.Run("authorized GET sets no-store headers", func(t *testing.T) {
		assertNoStoreHeaders(t, handler, http.MethodGet, "/?app=webapp&env=dev",
			cookie)
	})
}

func assertMissingCookieResponses(t *testing.T, mux http.Handler) {
	t.Helper()
	if got := getStatus(mux, "/"); got != http.StatusSeeOther {
		t.Fatalf("missing-cookie / status = %d, want %d", got, http.StatusSeeOther)
	}
	form := url.Values{
		"app": {"webapp"}, "env": {"dev"}, "key": {"k"},
		"value": {"v"}, "action": {"create"},
	}
	if got := postFormStatus(mux, "/secret", form, nil); got != http.StatusUnauthorized {
		t.Fatalf("missing-cookie /secret status = %d, want %d", got, http.StatusUnauthorized)
	}
}

func assertWrongTokenRejected(t *testing.T, mux http.Handler) {
	t.Helper()
	rec := httptest.NewRecorder()
	wrong := url.Values{"token": {"nope"}}
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(wrong.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "invalid token") {
		t.Fatalf("wrong token body missing form/error: %s", rec.Body.String())
	}
}

func assertCookieAttrs(t *testing.T, cookie *http.Cookie, wantSecure bool) {
	t.Helper()
	if cookie.Name != "thimble_session" {
		t.Fatalf("cookie name = %q, want thimble_session", cookie.Name)
	}
	if !cookie.HttpOnly {
		t.Fatalf("cookie HttpOnly false")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Fatalf("cookie Path = %q, want /", cookie.Path)
	}
	if cookie.MaxAge != 3600 {
		t.Fatalf("cookie MaxAge = %d, want 3600", cookie.MaxAge)
	}
	if cookie.Secure != wantSecure {
		t.Fatalf("cookie Secure = %v, want %v", cookie.Secure, wantSecure)
	}
}

func assertAuthorizedView(t *testing.T, mux http.Handler, cookie *http.Cookie) {
	t.Helper()
	body, status := getBodyWithCookie(mux, "/?app=webapp&env=dev", cookie)
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
	if strings.Contains(body, "token=") {
		t.Fatalf("web UI still passes token in URL: %s", body)
	}
	// K-34 strict mode: no <input name="value"> field should ever
	// render. Recipient and namespace fields are unaffected.
	if strings.Contains(body, `name="value"`) {
		t.Fatalf("web UI still has a value input: %s", body)
	}
	if !strings.Contains(body, "thimble set webapp dev API_KEY") {
		t.Fatalf("web UI did not surface CLI suggestion: %s", body)
	}
	// K-35 in-page banner above the namespace list.
	if !strings.Contains(body,
		"single-operator local tool · use CLI for shared/production") {
		t.Fatalf("web UI missing K-35 scope banner: %s", body)
	}
}

// assertStrictModeRejection covers K-34: any non-empty value posted to
// /secret with action=create or action=update must return 400 with a
// CLI suggestion and must never echo the submitted value back.
func assertStrictModeRejection(t *testing.T, mux http.Handler, cookie *http.Cookie) {
	t.Helper()
	for _, action := range []string{"create", "update"} {
		form := url.Values{
			"app": {"webapp"}, "env": {"dev"}, "key": {"API_KEY"},
			"value": {"super-secret-attempt"}, "action": {action},
		}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/secret",
			strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(cookie)
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s plaintext status = %d, want 400; body=%q",
				action, rec.Code, rec.Body.String())
		}
		body := rec.Body.String()
		if !strings.Contains(body, "web UI does not accept secret values") ||
			!strings.Contains(body, "thimble set webapp dev API_KEY") {
			t.Fatalf("%s rejection body missing CLI hint: %q", action, body)
		}
		if strings.Contains(body, "super-secret-attempt") {
			t.Fatalf("%s rejection echoed plaintext value: %q", action, body)
		}
	}
}

// assertAuthorizedDelete confirms the strict-mode UI still permits the
// non-value-bearing operations: deletes and recipients.
func assertAuthorizedDelete(t *testing.T, mux http.Handler, st *store.Store,
	cookie *http.Cookie) {
	t.Helper()
	if err := st.SetSecret("webapp", "dev", "DOOMED", "to-delete"); err != nil {
		t.Fatalf("seed delete target: %v", err)
	}
	form := url.Values{
		"app": {"webapp"}, "env": {"dev"}, "key": {"DOOMED"},
		"action": {"delete"},
	}
	if status := postFormStatus(mux, "/secret", form, cookie); status != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want %d", status, http.StatusSeeOther)
	}
	keys, err := st.ListSecrets("webapp", "dev")
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	for _, k := range keys {
		if k == "DOOMED" {
			t.Fatalf("delete via web did not remove key: %v", keys)
		}
	}
}

func assertLogoutClears(t *testing.T, mux http.Handler, cookie *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	logoutReq := httptest.NewRequest(http.MethodGet, "/logout", nil)
	logoutReq.AddCookie(cookie)
	mux.ServeHTTP(rec, logoutReq)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("logout status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	cleared := findSessionCookie(rec.Result().Cookies())
	if cleared == nil {
		t.Fatalf("logout did not Set-Cookie thimble_session")
	}
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("logout cookie not cleared: value=%q maxage=%d",
			cleared.Value, cleared.MaxAge)
	}
}

func TestWebUINonLoopbackSetsSecureCookie(t *testing.T) {
	st := newTestStore(t)
	server := web.New(st, "test-token", false)
	mux := http.NewServeMux()
	server.Routes(mux)

	cookie := loginAndExtractCookie(t, mux, "test-token")
	assertCookieAttrs(t, cookie, true)
}

func loginAndExtractCookie(t *testing.T, mux http.Handler, token string) *http.Cookie {
	t.Helper()
	form := url.Values{"token": {token}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	c := findSessionCookie(rec.Result().Cookies())
	if c == nil {
		t.Fatalf("login did not Set-Cookie thimble_session")
	}
	return c
}

func findSessionCookie(cookies []*http.Cookie) *http.Cookie {
	for _, c := range cookies {
		if c.Name == "thimble_session" {
			return c
		}
	}
	return nil
}

func getStatus(mux http.Handler, target string) int {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec.Code
}

func getBodyWithCookie(mux http.Handler, target string, c *http.Cookie) (string, int) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if c != nil {
		req.AddCookie(c)
	}
	mux.ServeHTTP(rec, req)
	return rec.Body.String(), rec.Code
}

func postFormStatus(mux http.Handler, path string, form url.Values,
	c *http.Cookie) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path,
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c != nil {
		req.AddCookie(c)
	}
	mux.ServeHTTP(rec, req)
	return rec.Code
}


// testRecipientOperator is a real-shape 62-char age recipient (Bech32
// charset only). Used to satisfy ValidateRecipient under K-20.
const testRecipientOperator = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"

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
