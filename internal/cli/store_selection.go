package cli

import (
	"os"
	"path/filepath"

	"github.com/cartine/thimble/internal/storecatalog"
)

const legacyStoreDir = "secrets"

func resolveStoreSelection(value, source string) (storecatalog.Selection, error) {
	if source == storecatalog.SourceDefault {
		if legacy, ok, err := existingLegacyStore(); err != nil {
			return storecatalog.Selection{}, err
		} else if ok {
			return legacy, nil
		}
		if homeStore, ok := existingHomeStore(); ok {
			return homeStore, nil
		}
	}
	selection, err := storecatalog.Resolve(value, source)
	if err == nil || source != storecatalog.SourceDefault {
		return selection, err
	}
	return legacyStoreFallback()
}

func existingLegacyStore() (storecatalog.Selection, bool, error) {
	path, err := filepath.Abs(legacyStoreDir)
	if err != nil {
		return storecatalog.Selection{}, false, err
	}
	selection, ok := existingStore(path, storecatalog.SourceLegacy)
	return selection, ok, nil
}

func existingHomeStore() (storecatalog.Selection, bool) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return storecatalog.Selection{}, false
	}
	path := filepath.Join(home, ".config", "thimble", "store")
	return existingStore(path, storecatalog.SourceHome)
}

func existingStore(path, source string) (storecatalog.Selection, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return storecatalog.Selection{}, false
	}
	manifest, err := os.Lstat(filepath.Join(path, "thimble.json"))
	if err != nil || !manifest.Mode().IsRegular() {
		return storecatalog.Selection{}, false
	}
	return storecatalog.Selection{Path: filepath.Clean(path), Source: source}, true
}

func legacyStoreFallback() (storecatalog.Selection, error) {
	path, err := filepath.Abs(legacyStoreDir)
	if err != nil {
		return storecatalog.Selection{}, err
	}
	return storecatalog.Selection{Path: path, Source: storecatalog.SourceLegacy}, nil
}

func defaultIdentityPath() string {
	if configured := os.Getenv("THIMBLE_AGE_IDENTITY"); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	root := filepath.Join(home, ".config", "thimble")
	for _, name := range []string{"identity", "identity.txt"} {
		path := filepath.Join(root, name)
		info, statErr := os.Lstat(path)
		if statErr == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}
