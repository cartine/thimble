package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/storecatalog"
)

func runStore(cfg cliConfig, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: thimble store <create|list|status>")
	}
	catalog, err := storecatalog.NewDefault()
	if err != nil {
		return err
	}
	switch args[0] {
	case "create":
		return runStoreCreate(catalog, args[1:], stdout)
	case "list":
		if len(args) != 1 {
			return errors.New("usage: thimble store list")
		}
		return runStoreList(catalog, stdout)
	case "status":
		if len(args) != 1 {
			return errors.New("usage: thimble store status")
		}
		return runStoreStatus(catalog, cfg, stdout)
	default:
		return fmt.Errorf("unknown store command %q", args[0])
	}
}

func runStoreCreate(
	catalog *storecatalog.Catalog, args []string, stdout io.Writer,
) error {
	if len(args) != 1 {
		return errors.New("usage: thimble store create <name>")
	}
	selection, err := catalog.Create(args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created managed store %s at %s\n", selection.Name, selection.Path)
	return nil
}

func runStoreList(catalog *storecatalog.Catalog, stdout io.Writer) error {
	entries, err := catalog.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "No managed stores installed. Run: thimble store create default")
		return nil
	}
	fmt.Fprintln(stdout, "NAME\tSTATUS\tPATH")
	for _, entry := range entries {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", entry.Name, entry.Status, entry.Path)
	}
	return nil
}

func runStoreStatus(
	catalog *storecatalog.Catalog, cfg cliConfig, stdout io.Writer,
) error {
	selection := cfg.selection
	entry := catalog.Inspect(selection)
	name := selection.Name
	if !selection.Managed {
		name = "-"
	}
	fmt.Fprintf(stdout, "Store name: %s\n", name)
	fmt.Fprintf(stdout, "Store path: %s\n", selection.Path)
	fmt.Fprintf(stdout, "Selected by: %s\n", selection.Source)
	fmt.Fprintf(stdout, "Store status: %s\n", entry.Status)
	printIdentityStatus(stdout, cfg.identity)
	if entry.Status != storecatalog.StatusValid {
		fmt.Fprintln(stdout, "Namespaces: 0")
		return nil
	}
	namespaces, err := store.New(selection.Path, "").ListNamespaces()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Namespaces: %d\n", len(namespaces))
	for _, namespace := range namespaces {
		fmt.Fprintf(
			stdout, "  %s/%s (%d recipients)\n",
			namespace.App, namespace.Env, namespace.Recipients,
		)
	}
	return nil
}

func printIdentityStatus(stdout io.Writer, identity string) {
	if identity == "" {
		fmt.Fprintln(stdout, "Identity: not configured")
		return
	}
	if _, err := os.Stat(identity); err != nil {
		fmt.Fprintf(stdout, "Identity: %s (unavailable)\n", identity)
		return
	}
	fmt.Fprintf(stdout, "Identity: %s\n", identity)
}

func requireManagedStore(cfg cliConfig, command string) error {
	if !cfg.selection.Managed || !isMutatingCommand(command) {
		return nil
	}
	info, err := os.Lstat(cfg.storeDir)
	if err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return fmt.Errorf(
		"managed store %q is not installed; run `thimble store create %s`, "+
			"`thimble store list`, or select an absolute path with --store",
		cfg.selection.Name, cfg.selection.Name,
	)
}
