package web_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/audit"
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

	server := web.NewForTest(st, "test-token", true)
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
	t.Run("cross-origin write with valid cookie is rejected", func(t *testing.T) {
		assertCrossOriginWriteRejected(t, handler, st, cookie)
	})
	t.Run("masked set stores without reflecting plaintext", func(t *testing.T) {
		assertMaskedSetNoReflection(t, handler, st, cookie)
	})
	t.Run("generated passphrase stores without reaching browser", func(t *testing.T) {
		assertGeneratedPassphraseNoReflection(t, handler, st, cookie)
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

func assertCrossOriginWriteRejected(
	t *testing.T, handler http.Handler, st *store.Store, cookie *http.Cookie,
) {
	t.Helper()
	guard := web.NewHostGuard(web.LoopbackAuthorities("8787"))
	protected := guard.Middleware(handler)
	form := url.Values{
		"app": {"webapp"}, "env": {"dev"}, "key": {"API_KEY"},
		"value": {"attacker-value"}, "action": {"set"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost, "http://localhost:8787/secret", strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://localhost:3000")
	req.AddCookie(cookie)
	protected.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want 403; body=%q", rec.Code, rec.Body.String())
	}
	values, _, err := st.ReadEnv("webapp", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if values["API_KEY"] != "browser secret" {
		t.Fatalf("cross-origin write changed API_KEY to %q", values["API_KEY"])
	}
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
		!strings.Contains(body, `data-secret-editor`) {
		t.Fatalf("web UI polish elements missing: %s", body)
	}
	if strings.Contains(body, "browser secret") {
		t.Fatalf("web UI leaked secret value")
	}
	if strings.Contains(body, "token=") {
		t.Fatalf("web UI still passes token in URL: %s", body)
	}
	if !strings.Contains(body, `type="password" name="value"`) ||
		!strings.Contains(body, `autocomplete="off"`) {
		t.Fatalf("web UI masked value input missing: %s", body)
	}
	if !strings.Contains(body, "Create or update without revealing") ||
		!strings.Contains(body, `value="generate-passphrases"`) ||
		!strings.Contains(body, `value="hyphenated-4w-gt30c"`) {
		t.Fatalf("web UI secret creation choices missing: %s", body)
	}
	if !strings.Contains(body, `id="staged-key-list"`) ||
		!strings.Contains(body, `class="secondary copy-command"`) ||
		!strings.Contains(body, `id="delete-secret-dialog"`) ||
		!strings.Contains(body, "Copied the CLI reveal command for") {
		t.Fatalf("web UI secret controls missing: %s", body)
	}
	if !strings.Contains(body, `.toast-region { position:fixed; right:24px; bottom:24px`) {
		t.Fatalf("web UI bottom-right toast styling missing: %s", body)
	}
	if !strings.Contains(body, "thimble --store") ||
		!strings.Contains(body, "get webapp dev API_KEY") {
		t.Fatalf("web UI did not surface retrieval command: %s", body)
	}
}

func assertMaskedSetNoReflection(
	t *testing.T, mux http.Handler, st *store.Store, cookie *http.Cookie,
) {
	t.Helper()
	const marker = "unique-browser-secret-marker-8472"
	st.SetAuditLogger(audit.New(st.Root(), io.Discard))
	form := url.Values{
		"app": {"webapp"}, "env": {"dev"}, "key": {"BROWSER_SET"},
		"value": {marker}, "action": {"set"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/secret", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("set status = %d, want 303; body=%q", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if strings.Contains(rec.Body.String(), marker) || strings.Contains(location, marker) {
		t.Fatalf("set response reflected plaintext: body=%q location=%q",
			rec.Body.String(), location)
	}
	body, status := getBodyWithCookie(mux, location, cookie)
	if status != http.StatusOK || strings.Contains(body, marker) {
		t.Fatalf("post-set page status=%d leaked marker=%v", status,
			strings.Contains(body, marker))
	}
	if !strings.Contains(body, "get webapp dev BROWSER_SET") {
		t.Fatalf("post-set retrieval command missing: %s", body)
	}
	values, _, err := st.ReadEnv("webapp", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if values["BROWSER_SET"] != marker {
		t.Fatalf("stored value = %q", values["BROWSER_SET"])
	}
	auditBody, err := os.ReadFile(filepath.Join(st.Root(), ".thimble-audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(auditBody), marker) {
		t.Fatalf("audit log reflected plaintext: %s", auditBody)
	}
}

func assertGeneratedPassphraseNoReflection(
	t *testing.T, mux http.Handler, st *store.Store, cookie *http.Cookie,
) {
	t.Helper()
	form := url.Values{
		"app": {"webapp"}, "env": {"dev"},
		"key":       {"GENERATED_PHRASE", "GENERATED_PHRASE", "GENERATED_SECOND"},
		"action":    {"generate-passphrases"},
		"generator": {"hyphenated-4w-gt30c"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/secret", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("generate status = %d, want 303; body=%q", rec.Code, rec.Body.String())
	}

	values, _, err := st.ReadEnv("webapp", "dev")
	if err != nil {
		t.Fatal(err)
	}
	phrases := []string{values["GENERATED_PHRASE"], values["GENERATED_SECOND"]}
	for _, phrase := range phrases {
		if len(phrase) < 31 || len(strings.Split(phrase, "-")) != 4 {
			t.Fatalf("generated value has wrong shape: %q", phrase)
		}
		for _, char := range phrase {
			if (char < 'a' || char > 'z') && char != '-' {
				t.Fatalf("generated value contains unexpected character %q", char)
			}
		}
	}
	location := rec.Header().Get("Location")
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	notice := redirectURL.Query().Get("notice")
	if !strings.HasPrefix(notice, "2 four-word secrets generated") {
		t.Fatalf("notice = %q", notice)
	}
	if saved := redirectURL.Query()["saved"]; len(saved) != 2 {
		t.Fatalf("saved keys = %#v, want two", saved)
	}
	for _, phrase := range phrases {
		if strings.Contains(rec.Body.String(), phrase) || strings.Contains(location, phrase) {
			t.Fatalf("generate response reflected plaintext: body=%q location=%q",
				rec.Body.String(), location)
		}
	}
	body, status := getBodyWithCookie(mux, location, cookie)
	if status != http.StatusOK {
		t.Fatalf("post-generate page status=%d", status)
	}
	for _, phrase := range phrases {
		if strings.Contains(body, phrase) {
			t.Fatalf("post-generate page leaked generated value")
		}
	}
	if count := strings.Count(body, `class="saved"`); count != 2 {
		t.Fatalf("highlighted rows = %d, want 2", count)
	}
	auditBody, err := os.ReadFile(filepath.Join(st.Root(), ".thimble-audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, phrase := range phrases {
		if strings.Contains(string(auditBody), phrase) {
			t.Fatalf("audit log reflected generated value: %s", auditBody)
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
	server := web.NewForTest(st, "test-token", false)
	mux := http.NewServeMux()
	server.Routes(mux)

	cookie := loginAndExtractCookie(t, mux, "test-token")
	assertCookieAttrs(t, cookie, true)
	form := url.Values{
		"app": {"webapp"}, "env": {"dev"}, "key": {"KEY"},
		"value": {"must-not-store"}, "action": {"set"},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/secret", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther ||
		!strings.Contains(rec.Header().Get("Location"), "loopback") {
		t.Fatalf("non-loopback set response = %d %q",
			rec.Code, rec.Header().Get("Location"))
	}
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
