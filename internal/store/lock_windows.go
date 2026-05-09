//go:build windows

package store

// Windows fallback: process-local mutex via a simple file marker.
// Cross-process locking on Windows requires LockFileEx; until K-29's
// portable lock primitive lands, single-host concurrent writers on
// Windows fall back to in-process serialization. This is documented
// in the SECURITY.md residual-risks section under K-21.

import (
	"os"
	"path/filepath"
	"sync"
)

type fileLock struct {
	f *os.File
}

var winLockMu sync.Mutex

func lockExclusive(root string) (*fileLock, error) {
	winLockMu.Lock()
	return openLockFile(root)
}

func lockShared(root string) (*fileLock, error) {
	winLockMu.Lock()
	return openLockFile(root)
}

func openLockFile(root string) (*fileLock, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		winLockMu.Unlock()
		return nil, err
	}
	// #nosec G304 -- path is the store-root sentinel file.
	f, err := os.OpenFile(
		filepath.Join(root, lockFileName),
		os.O_CREATE|os.O_RDWR,
		0o600,
	)
	if err != nil {
		winLockMu.Unlock()
		return nil, err
	}
	return &fileLock{f: f}, nil
}

const lockFileName = ".thimble.lock"

func (l *fileLock) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	winLockMu.Unlock()
	return err
}
