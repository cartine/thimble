package storecatalog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveManagedAndExplicitSelections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	managed, err := Resolve("personal/main", SourceFlag)
	if err != nil {
		t.Fatalf("resolve managed: %v", err)
	}
	if !managed.Managed || managed.Name != "personal/main" {
		t.Fatalf("managed selection = %#v", managed)
	}
	if managed.Source != SourceFlag || !filepath.IsAbs(managed.Path) {
		t.Fatalf("managed source/path = %#v", managed)
	}

	explicitPath := filepath.Join(home, "elsewhere")
	explicit, err := Resolve(explicitPath, SourceEnv)
	if err != nil {
		t.Fatalf("resolve explicit: %v", err)
	}
	if explicit.Managed || explicit.Path != explicitPath || explicit.Name != "" {
		t.Fatalf("explicit selection = %#v", explicit)
	}
}

func TestResolveRejectsTraversalAndDotNames(t *testing.T) {
	for _, name := range []string{".", "..", "../escape", "team/../escape", "/"} {
		if name == "/" {
			continue
		}
		if _, err := Resolve(name, SourceFlag); err == nil {
			t.Fatalf("Resolve(%q) succeeded", name)
		}
	}
}

func TestCatalogCreateListAndInspect(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stores")
	catalog := New(root)
	for _, name := range []string{"personal", "clients/acme"} {
		selection, err := catalog.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if got := catalog.Inspect(selection).Status; got != StatusValid {
			t.Fatalf("status %s = %s", name, got)
		}
		info, err := os.Stat(selection.Path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("store mode = %o", info.Mode().Perm())
		}
	}

	entries, err := catalog.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 || entries[0].Name != "clients/acme" ||
		entries[1].Name != "personal" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestCatalogDoesNotDiscoverSymlinkedStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	base := t.TempDir()
	catalog := New(filepath.Join(base, "stores"))
	if _, err := catalog.Create("real"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := initializeManifest(outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(catalog.Root(), "linked")); err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "real" {
		t.Fatalf("symlink was discovered: %#v", entries)
	}
}
