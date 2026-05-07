package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cartine/thimble/internal/store"
)

// readOrigins returns the on-disk bytes of the origins file for
// (app, env). Used by K-37 tests to compare pre/post snapshots when
// asserting atomicity. Empty bytes indicate the file does not exist.
func readOrigins(t *testing.T, st *store.Store, app, env string) []byte {
	t.Helper()
	path := filepath.Join(
		st.Root(), app, env+store.OriginsFileSuffix,
	)
	// #nosec G304 -- path is rooted at t.TempDir() controlled by the test.
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("read origins: %v", err)
	}
	return b
}

// removeIfExists deletes path, swallowing the not-exist error so
// callers can use it on optional files like .origins.json without a
// pre-stat round-trip.
func removeIfExists(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
