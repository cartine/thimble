package cli

import (
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/store"
)

func seedGetNamespace(t *testing.T, st *store.Store) {
	t.Helper()
	if err := st.Init("web-api", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("web-api", "prod", "DATABASE_URL", "postgres://x"); err != nil {
		t.Fatalf("set DB: %v", err)
	}
	if err := st.SetSecret("web-api", "prod", "API_KEY", "k-77-secret"); err != nil {
		t.Fatalf("set API: %v", err)
	}
}

func TestGetPrintsValueForExistingKey(t *testing.T) {
	st := newTestStore(t)
	seedGetNamespace(t, st)
	var stdout, stderr strings.Builder
	err := runGet(st, []string{"web-api", "prod", "API_KEY"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("runGet: %v stderr=%s", err, stderr.String())
	}
	if got := stdout.String(); got != "k-77-secret\n" {
		t.Fatalf("stdout = %q, want %q", got, "k-77-secret\n")
	}
}

func TestGetMissingKeyErrors(t *testing.T) {
	st := newTestStore(t)
	seedGetNamespace(t, st)
	var stdout, stderr strings.Builder
	err := runGet(st, []string{"web-api", "prod", "NOPE"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("runGet: want error for missing key, got nil")
	}
	want := "NOPE is not set in web-api/prod"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestGetWithoutKeyListsNamesOnly(t *testing.T) {
	st := newTestStore(t)
	seedGetNamespace(t, st)
	var stdout, stderr strings.Builder
	if err := runGet(st, []string{"web-api", "prod"}, &stdout, &stderr); err != nil {
		t.Fatalf("runGet: %v stderr=%s", err, stderr.String())
	}
	got := stdout.String()
	want := "API_KEY\nDATABASE_URL\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	for _, value := range []string{"k-77-secret", "postgres://x"} {
		if strings.Contains(got, value) {
			t.Fatalf("stdout leaked value %q: %q", value, got)
		}
	}
}

func TestGetUsageErrorOnWrongArgCount(t *testing.T) {
	st := newTestStore(t)
	var stdout, stderr strings.Builder
	for _, args := range [][]string{
		{},
		{"web-api"},
		{"web-api", "prod", "KEY", "extra"},
	} {
		err := runGet(st, args, &stdout, &stderr)
		if err == nil || err.Error() != "usage: thimble get <app> <env> [KEY]" {
			t.Fatalf("args %v: error = %v, want usage error", args, err)
		}
	}
}
