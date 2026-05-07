package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/store"
)

func TestNamespacedCRUDAndRender(t *testing.T) {
	st := newTestStore(t)

	if err := st.Init("web-api", "production", []string{"age1operator"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.CreateSecret(
		"web-api", "production", "POSTGRES_PASSWORD", "alpha secret",
	); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.UpdateSecret(
		"web-api", "production", "POSTGRES_PASSWORD", "bravo secret",
	); err != nil {
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

	ciphertext, err := os.ReadFile(filepath.Join(st.Root(), "web-api", "production.env.age"))
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
	st := newTestStore(t)

	if err := st.Init("api", "prod", []string{"age1alice"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("api", "prod", "TOKEN", "topsecret"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := st.AddRecipient("api", "prod", "age1bob"); err != nil {
		t.Fatalf("add recipient: %v", err)
	}
	meta, err := st.Find("api", "prod")
	if err != nil {
		t.Fatalf("find env: %v", err)
	}
	if got, want := strings.Join(meta.Recipients, ","), "age1alice,age1bob"; got != want {
		t.Fatalf("recipients = %q, want %q", got, want)
	}
	if err := st.RemoveRecipient("api", "prod", "age1alice"); err != nil {
		t.Fatalf("remove recipient: %v", err)
	}
	meta, err = st.Find("api", "prod")
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

// newTestStore returns a Store backed by a fake `age` binary that
// reverses ROT13 instead of doing real encryption. Deterministic
// timestamps are pinned via SetClock so test output stays stable.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	st := store.New(filepath.Join(root, "secrets"), "")
	st.SetAge(age.New(fakeAge, ""))
	st.SetClock(func() time.Time { return time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC) })
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
