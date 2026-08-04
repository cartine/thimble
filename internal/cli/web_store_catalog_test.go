package cli

import (
	"path/filepath"
	"testing"

	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/storecatalog"
)

func TestWebStoreCatalogListsCreatesAndSelectsManagedStores(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	catalog, err := storecatalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	alpha, err := catalog.Create("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.Create("beta"); err != nil {
		t.Fatal(err)
	}
	provider := newWebStoreCatalog(
		catalog, alpha, func(path string) *store.Store { return store.New(path, "") },
	)

	stores, err := provider.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 2 || !stores[0].Active || stores[1].Active {
		t.Fatalf("initial stores = %#v", stores)
	}
	if _, err := provider.Select("beta"); err != nil {
		t.Fatal(err)
	}
	_, current, err := provider.Current()
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "beta" || !current.Active {
		t.Fatalf("current after select = %#v", current)
	}
	created, err := provider.Create("personal/main")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "personal/main" || !created.Active {
		t.Fatalf("created = %#v", created)
	}
	if _, err := provider.Select("../escape"); err == nil {
		t.Fatalf("traversal selection succeeded")
	}
	if _, err := provider.Select(filepath.Join(t.TempDir(), "absolute")); err == nil {
		t.Fatalf("absolute selection succeeded")
	}
}
