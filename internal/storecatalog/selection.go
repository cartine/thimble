package storecatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cartine/thimble/internal/store"
)

const DefaultName = "default"

const (
	SourceDefault = "default"
	SourceEnv     = "THIMBLE_STORE"
	SourceFlag    = "--store"
	SourceWeb     = "web UI"
	SourceLegacy  = "legacy ./secrets"
	SourceHome    = "~/.config/thimble/store"
)

// Selection is one resolved store directory. Managed selections are named
// children of the user configuration store root; explicit selections are
// absolute paths and are never included in discovery.
type Selection struct {
	Name    string
	Path    string
	Source  string
	Managed bool
}

// Root returns the directory containing managed Thimble stores.
func Root() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(configDir, "thimble", "stores"), nil
}

// Resolve converts a managed name or explicit absolute path into a Selection.
func Resolve(value, source string) (Selection, error) {
	if value == "" {
		value = DefaultName
	}
	if filepath.IsAbs(value) {
		return Selection{
			Path: filepath.Clean(value), Source: source,
		}, nil
	}
	root, err := Root()
	if err != nil {
		return Selection{}, err
	}
	name, err := cleanManagedName(value)
	if err != nil {
		return Selection{}, err
	}
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := ensureWithinRoot(root, path); err != nil {
		return Selection{}, err
	}
	return Selection{
		Name: name, Path: path, Source: source, Managed: true,
	}, nil
}

func cleanManagedName(value string) (string, error) {
	for _, rawSegment := range strings.Split(filepath.ToSlash(value), "/") {
		if rawSegment == "" || rawSegment == "." || rawSegment == ".." {
			return "", fmt.Errorf("invalid managed store name %q", value)
		}
	}
	cleaned := filepath.Clean(value)
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid managed store name %q", value)
	}
	name := filepath.ToSlash(cleaned)
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid managed store name %q", value)
		}
		if err := store.ValidateName("store name", segment); err != nil {
			return "", err
		}
	}
	return name, nil
}

func ensureWithinRoot(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("resolve managed store path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("managed store path escapes store root")
	}
	return nil
}
