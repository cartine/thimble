package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/storecatalog"
)

func TestStoreCreateListAndStatusWithoutAge(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	var stdout, stderr strings.Builder

	if err := Run([]string{"store", "create", "personal"}, &stdout, &stderr); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(stdout.String(), "created managed store personal") {
		t.Fatalf("create output = %q", stdout.String())
	}
	stdout.Reset()
	if err := Run([]string{"store", "list"}, &stdout, &stderr); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout.String(), "personal\tvalid") {
		t.Fatalf("list output = %q", stdout.String())
	}

	stdout.Reset()
	if err := Run(
		[]string{"--store", "personal", "store", "status"},
		&stdout, &stderr,
	); err != nil {
		t.Fatalf("status: %v", err)
	}
	status := stdout.String()
	for _, want := range []string{
		"Store name: personal", "Selected by: --store",
		"Store status: valid", "Identity: not configured", "Namespaces: 0",
	} {
		if !strings.Contains(status, want) {
			t.Fatalf("status missing %q: %s", want, status)
		}
	}
}

func TestStoreStatusShowsNamespacesWithoutValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	catalog, err := storecatalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := catalog.Create("personal")
	if err != nil {
		t.Fatal(err)
	}
	fakeAge := writeFakeAge(t, t.TempDir())
	st := store.NewWithAge(selection.Path, age.New(fakeAge, ""))
	if err := st.Init("personal", "main", []string{testRecipientOperator}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecret("personal", "main", "PRIVATE_KEY", "never-print-me"); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr strings.Builder
	if err := Run(
		[]string{"--store", "personal", "store", "status"},
		&stdout, &stderr,
	); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "personal/main (1 recipients)") {
		t.Fatalf("namespace missing: %s", got)
	}
	for _, forbidden := range []string{"PRIVATE_KEY", "never-print-me"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("status leaked %q: %s", forbidden, got)
		}
	}
}

func TestStoreSelectionPrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("THIMBLE_STORE", "from-env")
	var stderr strings.Builder

	cfg, _, err := parseTopFlags([]string{"store", "status"}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.selection.Name != "from-env" || cfg.selection.Source != storecatalog.SourceEnv {
		t.Fatalf("env selection = %#v", cfg.selection)
	}
	cfg, _, err = parseTopFlags(
		[]string{"--store", "from-flag", "store", "status"}, &stderr,
	)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.selection.Name != "from-flag" || cfg.selection.Source != storecatalog.SourceFlag {
		t.Fatalf("flag selection = %#v", cfg.selection)
	}
}

func TestMissingManagedStoreGuidesMutation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "")
	var stdout, stderr strings.Builder
	err := Run(
		[]string{
			"--store", "missing", "init", "personal", "main",
			"--recipient", testRecipientOperator,
		},
		&stdout, &stderr,
	)
	if err == nil || !strings.Contains(err.Error(), "thimble store create missing") {
		t.Fatalf("missing-store error = %v", err)
	}
	root, rootErr := storecatalog.Root()
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "missing")); !os.IsNotExist(statErr) {
		t.Fatalf("missing store was created: %v", statErr)
	}
}
