package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/storecatalog"
)

func TestDefaultSelectionPrefersLegacyStore(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("THIMBLE_STORE", "")
	if err := os.Mkdir("secrets", 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestManifest(t, filepath.Join(root, "secrets"))
	cfg, _, err := parseTopFlags([]string{"store", "status"}, new(strings.Builder))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "secrets")
	if cfg.storeDir != want || cfg.selection.Source != storecatalog.SourceLegacy {
		t.Fatalf("legacy selection = %#v, want path %q", cfg.selection, want)
	}
}

func TestDefaultSelectionIgnoresUninitializedLegacyDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("THIMBLE_STORE", "")
	if err := os.Mkdir("secrets", 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := parseTopFlags([]string{"store", "status"}, new(strings.Builder))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.selection.Managed || cfg.selection.Name != storecatalog.DefaultName {
		t.Fatalf("uninitialized legacy selection = %#v", cfg.selection)
	}
}

func TestDefaultSelectionUsesExistingHomeStore(t *testing.T) {
	working := t.TempDir()
	home := t.TempDir()
	t.Chdir(working)
	t.Setenv("HOME", home)
	t.Setenv("THIMBLE_STORE", "")
	path := filepath.Join(home, ".config", "thimble", "store")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestManifest(t, path)
	cfg, _, err := parseTopFlags([]string{"store", "status"}, new(strings.Builder))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.storeDir != path || cfg.selection.Source != storecatalog.SourceHome {
		t.Fatalf("home selection = %#v", cfg.selection)
	}
}

func writeTestManifest(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "thimble.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultSelectionFallsBackWithoutHome(t *testing.T) {
	working := t.TempDir()
	t.Chdir(working)
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("THIMBLE_STORE", "")
	cfg, _, err := parseTopFlags([]string{"store", "status", "--path"}, new(strings.Builder))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(working, "secrets")
	if cfg.storeDir != want || cfg.selection.Source != storecatalog.SourceLegacy {
		t.Fatalf("fallback selection = %#v, want path %q", cfg.selection, want)
	}
}

func TestDefaultIdentitySupportsCurrentAndLegacyNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("THIMBLE_AGE_IDENTITY", "")
	root := filepath.Join(home, ".config", "thimble")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "identity.txt")
	if err := os.WriteFile(legacy, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := defaultIdentityPath(); got != legacy {
		t.Fatalf("legacy identity = %q, want %q", got, legacy)
	}
	current := filepath.Join(root, "identity")
	if err := os.WriteFile(current, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := defaultIdentityPath(); got != current {
		t.Fatalf("current identity = %q, want %q", got, current)
	}
	t.Setenv("THIMBLE_AGE_IDENTITY", "/explicit/identity")
	if got := defaultIdentityPath(); got != "/explicit/identity" {
		t.Fatalf("configured identity = %q", got)
	}
}
