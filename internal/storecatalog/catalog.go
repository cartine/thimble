package storecatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/cartine/thimble/internal/store"
)

const manifestName = "thimble.json"

type Status string

const (
	StatusMissing Status = "missing"
	StatusEmpty   Status = "empty"
	StatusValid   Status = "valid"
	StatusInvalid Status = "invalid"
)

type Entry struct {
	Name   string
	Path   string
	Status Status
}

type Catalog struct {
	root string
}

func New(root string) *Catalog { return &Catalog{root: root} }

func NewDefault() (*Catalog, error) {
	root, err := Root()
	if err != nil {
		return nil, err
	}
	return New(root), nil
}

func (c *Catalog) Root() string { return c.root }

// Create initializes a managed store without requiring age on PATH.
func (c *Catalog) Create(name string) (Selection, error) {
	selection, err := c.resolveName(name)
	if err != nil {
		return Selection{}, err
	}
	if err := c.makeManagedPath(selection.Path); err != nil {
		return Selection{}, err
	}
	if err := initializeManifest(selection.Path); err != nil {
		return Selection{}, err
	}
	return selection, nil
}

func (c *Catalog) Inspect(selection Selection) Entry {
	entry := Entry{Name: selection.Name, Path: selection.Path}
	info, err := os.Lstat(selection.Path)
	if errors.Is(err, os.ErrNotExist) {
		entry.Status = StatusMissing
		return entry
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		entry.Status = StatusInvalid
		return entry
	}
	entry.Status = inspectManifest(selection.Path)
	return entry
}

// List discovers manifest-bearing managed stores without following symlinks.
func (c *Catalog) List() ([]Entry, error) {
	if _, err := os.Stat(c.root); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var entries []Entry
	err := filepath.WalkDir(c.root, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if item.Type()&os.ModeSymlink != 0 {
			if item.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if item.IsDir() || item.Name() != manifestName {
			return nil
		}
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(c.root, dir)
		if err != nil || rel == "." {
			return err
		}
		name, err := cleanManagedName(filepath.ToSlash(rel))
		if err != nil {
			return nil
		}
		entries = append(entries, Entry{
			Name: name, Path: dir, Status: inspectManifest(dir),
		})
		return filepath.SkipDir
	})
	if err != nil {
		return nil, fmt.Errorf("list managed stores: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func (c *Catalog) resolveName(name string) (Selection, error) {
	cleaned, err := cleanManagedName(name)
	if err != nil {
		return Selection{}, err
	}
	path := filepath.Join(c.root, filepath.FromSlash(cleaned))
	if err := ensureWithinRoot(c.root, path); err != nil {
		return Selection{}, err
	}
	return Selection{
		Name: cleaned, Path: path, Source: SourceWeb, Managed: true,
	}, nil
}

func (c *Catalog) makeManagedPath(path string) error {
	if err := os.MkdirAll(c.root, 0o700); err != nil {
		return err
	}
	rel, err := filepath.Rel(c.root, path)
	if err != nil {
		return err
	}
	current := c.root
	for _, segment := range splitPath(rel) {
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			if err := os.Mkdir(current, 0o700); err != nil {
				return err
			}
		case statErr != nil:
			return statErr
		case !info.IsDir() || info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("managed store path %q is not a directory", current)
		}
	}
	return nil
}

func splitPath(path string) []string {
	var parts []string
	for path != "." && path != "" {
		dir, file := filepath.Split(path)
		parts = append([]string{file}, parts...)
		path = filepath.Clean(dir)
	}
	return parts
}

func initializeManifest(path string) error {
	manifestPath := filepath.Join(path, manifestName)
	// #nosec G304 -- path is resolved beneath the catalog root from a validated name.
	file, err := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString("{\n  \"version\": 1,\n  \"apps\": {}\n}\n")
	return err
}

func inspectManifest(path string) Status {
	// #nosec G304 -- callers pass a catalog-resolved or explicitly selected store root.
	b, err := os.ReadFile(filepath.Join(path, manifestName))
	if errors.Is(err, os.ErrNotExist) {
		return StatusEmpty
	}
	if err != nil {
		return StatusInvalid
	}
	var manifest store.Manifest
	if json.Unmarshal(b, &manifest) != nil || manifest.Version < 1 {
		return StatusInvalid
	}
	return StatusValid
}
