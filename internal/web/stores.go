package web

import (
	"errors"

	"github.com/cartine/thimble/internal/store"
)

type StoreInfo struct {
	Name   string
	Path   string
	Source string
	Status string
	Active bool
}

// StoreCatalog supplies the active store and the managed stores available to
// the single-operator web session. Browser input is always a managed name.
type StoreCatalog interface {
	Current() (*store.Store, StoreInfo, error)
	List() ([]StoreInfo, error)
	Create(name string) (StoreInfo, error)
	Select(name string) (StoreInfo, error)
}

type staticStoreCatalog struct {
	store *store.Store
	info  StoreInfo
}

func newStaticStoreCatalog(st *store.Store) *staticStoreCatalog {
	return &staticStoreCatalog{
		store: st,
		info: StoreInfo{
			Name: "-", Path: st.Root(), Source: "explicit", Status: "valid", Active: true,
		},
	}
}

func (c *staticStoreCatalog) Current() (*store.Store, StoreInfo, error) {
	return c.store, c.info, nil
}

func (c *staticStoreCatalog) List() ([]StoreInfo, error) {
	return []StoreInfo{c.info}, nil
}

func (c *staticStoreCatalog) Create(string) (StoreInfo, error) {
	return StoreInfo{}, errors.New("managed store creation is unavailable")
}

func (c *staticStoreCatalog) Select(string) (StoreInfo, error) {
	return StoreInfo{}, errors.New("managed store selection is unavailable")
}
