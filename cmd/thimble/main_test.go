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

	"github.com/cartine/thimble/internal/age"
)

func TestNamespacedCRUDAndRender(t *testing.T) {
	st := testStore(t)

	if err := st.Init("web-api", "production", []string{"age1operator"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.CreateSecret("web-api", "production", "POSTGRES_PASSWORD", "alpha secret"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.UpdateSecret("web-api", "production", "POSTGRES_PASSWORD", "bravo secret"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := st.SetSecret("web-api", "staging", "IGNORED", "value"); err == nil {
		t.Fatalf("set against uninitialized namespace succeeded")
	}

	keys, err := st.ListSecrets("web-api", "production")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got, want := strings.Join(keys, ","), "POSTGRES_PASSWORD"; got != want {
		t.Fatalf("keys = %q, want %q", got, want)
	}

	rendered, err := st.Render("web-api", "production")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, "POSTGRES_PASSWORD=\"bravo secret\"") {
		t.Fatalf("rendered dotenv missing updated value: %q", rendered)
	}

	ciphertext, err := os.ReadFile(filepath.Join(st.root, "web-api", "production.env.age"))
	if err != nil {
		t.Fatalf("read ciphertext: %v", err)
	}
	if strings.Contains(string(ciphertext), "bravo secret") {
		t.Fatalf("ciphertext contains plaintext secret")
	}

	if err := st.DeleteSecret("web-api", "production", "POSTGRES_PASSWORD"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rendered, err = st.Render("web-api", "production")
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
	if !strings.Contains(body, `aria-label="Thimble"`) || !strings.Contains(body, "Safe entry") {
		t.Fatalf("web UI polish elements missing: %s", body)
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

func TestCLIRejectsSecretArgumentAndAcceptsPipe(t *testing.T) {
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	t.Setenv("PATH", filepath.Dir(fakeAge)+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout, stderr strings.Builder
	if err := run([]string{"--store", filepath.Join(root, "secrets"), "init", "web-api", "dev", "--recipient", "age1operator"}, &stdout, &stderr); err != nil {
		t.Fatalf("init: %v stderr=%s", err, stderr.String())
	}
	err := run([]string{"--store", filepath.Join(root, "secrets"), "set", "web-api", "dev", "API_KEY", "unsafe-argv-value"}, &stdout, &stderr)
	if err == nil {
		t.Fatalf("set with argv value succeeded")
	}
	if !strings.Contains(err.Error(), "do not pass secret values") {
		t.Fatalf("unexpected argv value error: %v", err)
	}

	withStdin(t, "safe piped value\n", func() {
		if err := run([]string{"--store", filepath.Join(root, "secrets"), "set", "web-api", "dev", "API_KEY"}, &stdout, &stderr); err != nil {
			t.Fatalf("piped set: %v stderr=%s", err, stderr.String())
		}
	})
	rendered, err := newStore(filepath.Join(root, "secrets"), "").Render("web-api", "dev")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, "API_KEY=\"safe piped value\"") {
		t.Fatalf("piped value was not stored: %q", rendered)
	}
}

func TestProvisionAndSetAndGetFlow(t *testing.T) {
	st := testStore(t)
	if err := st.Init("worker", "prod", []string{"age1operator"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	root := t.TempDir()
	producer := filepath.Join(root, "producer.sh")
	if err := os.WriteFile(producer, []byte("#!/bin/sh\nprintf '%s\\n' generated-secret\n"), 0o700); err != nil {
		t.Fatalf("write producer: %v", err)
	}
	var stdout, stderr strings.Builder
	if err := runAndSet(st, []string{"worker", "prod", "SERVICE_TOKEN", "--", producer}, &stdout, &stderr); err != nil {
		t.Fatalf("and-set: %v stderr=%s", err, stderr.String())
	}
	rendered, err := st.Render("worker", "prod")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, "SERVICE_TOKEN=generated-secret") {
		t.Fatalf("and-set value missing: %q", rendered)
	}

	capture := filepath.Join(root, "capture.sh")
	outFile := filepath.Join(root, "captured.txt")
	if err := os.WriteFile(capture, []byte("#!/bin/sh\ncat > \"$1\"\n"), 0o700); err != nil {
		t.Fatalf("write capture: %v", err)
	}
	if err := runAndGet(st, []string{"worker", "prod", "SERVICE_TOKEN", "--", capture, outFile}, &stdout, &stderr); err != nil {
		t.Fatalf("and-get: %v stderr=%s", err, stderr.String())
	}
	captured, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read captured: %v", err)
	}
	if string(captured) != "generated-secret" {
		t.Fatalf("and-get passed %q", captured)
	}
}

func TestProvisionRequiresStrongTokenAndWritesToPipe(t *testing.T) {
	var stderr strings.Builder
	if err := runProvision([]string{"--bytes", "8"}, ioDiscardFile{}, &stderr); err == nil {
		t.Fatalf("weak provision succeeded")
	}

	var stdout strings.Builder
	if err := runProvision([]string{"--bytes", "16"}, &stdout, &stderr); err != nil {
		t.Fatalf("provision to non-terminal writer: %v", err)
	}
	if strings.TrimSpace(stdout.String()) == "" {
		t.Fatalf("provision wrote no token")
	}
}

func testStore(t *testing.T) *store {
	t.Helper()
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	st := newStore(filepath.Join(root, "secrets"), "")
	st.age = age.New(fakeAge, "")
	st.now = func() time.Time { return time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC) }
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

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	old := os.Stdin
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := write.WriteString(input); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := write.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	os.Stdin = read
	defer func() {
		os.Stdin = old
		read.Close()
	}()
	fn()
}

type ioDiscardFile struct{}

func (ioDiscardFile) Write(p []byte) (int, error) { return len(p), nil }
