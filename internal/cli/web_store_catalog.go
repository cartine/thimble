package cli

import (
	"errors"
	"fmt"
	"sync"

	"github.com/cartine/thimble/internal/store"
	"github.com/cartine/thimble/internal/storecatalog"
	"github.com/cartine/thimble/internal/web"
)

type webStoreCatalog struct {
	mu      sync.RWMutex
	catalog *storecatalog.Catalog
	active  storecatalog.Selection
	open    func(string) *store.Store
}

func newWebStoreCatalog(
	catalog *storecatalog.Catalog,
	active storecatalog.Selection,
	open func(string) *store.Store,
) *webStoreCatalog {
	return &webStoreCatalog{catalog: catalog, active: active, open: open}
}

func (c *webStoreCatalog) Current() (*store.Store, web.StoreInfo, error) {
	c.mu.RLock()
	selection := c.active
	c.mu.RUnlock()
	entry := c.catalog.Inspect(selection)
	info := webStoreInfo(selection, entry.Status, true)
	if entry.Status == storecatalog.StatusInvalid {
		return nil, info, fmt.Errorf("active store %q is invalid", selection.Path)
	}
	if selection.Managed && entry.Status == storecatalog.StatusMissing {
		return nil, info, nil
	}
	return c.open(selection.Path), info, nil
}

func (c *webStoreCatalog) List() ([]web.StoreInfo, error) {
	entries, err := c.catalog.List()
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	active := c.active
	c.mu.RUnlock()
	infos := make([]web.StoreInfo, 0, len(entries)+1)
	foundActive := false
	for _, entry := range entries {
		isActive := active.Managed && entry.Path == active.Path
		foundActive = foundActive || isActive
		selection := storecatalog.Selection{
			Name: entry.Name, Path: entry.Path, Source: storecatalog.SourceWeb, Managed: true,
		}
		infos = append(infos, webStoreInfo(selection, entry.Status, isActive))
	}
	if !foundActive {
		entry := c.catalog.Inspect(active)
		infos = append([]web.StoreInfo{webStoreInfo(active, entry.Status, true)}, infos...)
	}
	return infos, nil
}

func (c *webStoreCatalog) Create(name string) (web.StoreInfo, error) {
	selection, err := c.catalog.Create(name)
	if err != nil {
		return web.StoreInfo{}, err
	}
	selection.Source = storecatalog.SourceWeb
	c.setActive(selection)
	return webStoreInfo(selection, storecatalog.StatusValid, true), nil
}

func (c *webStoreCatalog) Select(name string) (web.StoreInfo, error) {
	selection, err := storecatalog.Resolve(name, storecatalog.SourceWeb)
	if err != nil {
		return web.StoreInfo{}, err
	}
	if !selection.Managed {
		return web.StoreInfo{}, errors.New("the web UI selects managed stores by name only")
	}
	entry := c.catalog.Inspect(selection)
	if entry.Status != storecatalog.StatusValid {
		return web.StoreInfo{}, fmt.Errorf(
			"managed store %q is not available (%s)", name, entry.Status,
		)
	}
	c.setActive(selection)
	return webStoreInfo(selection, entry.Status, true), nil
}

func (c *webStoreCatalog) setActive(selection storecatalog.Selection) {
	c.mu.Lock()
	c.active = selection
	c.mu.Unlock()
}

func webStoreInfo(
	selection storecatalog.Selection, status storecatalog.Status, active bool,
) web.StoreInfo {
	return web.StoreInfo{
		Name: selection.Name, Path: selection.Path, Source: selection.Source,
		Status: string(status), Active: active,
	}
}
