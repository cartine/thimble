//go:build !windows

package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// fileLock holds a flock on a sentinel file inside the store root.
// Use lockExclusive / lockShared to obtain it; release with Close.
type fileLock struct {
	f *os.File
}

const lockFileName = ".thimble.lock"

// lockExclusive acquires LOCK_EX over secrets/.thimble.lock. The
// store root is created with 0o700 if missing.
func lockExclusive(root string) (*fileLock, error) {
	return acquireLock(root, unix.LOCK_EX)
}

// lockShared acquires LOCK_SH over secrets/.thimble.lock.
func lockShared(root string) (*fileLock, error) {
	return acquireLock(root, unix.LOCK_SH)
}

func acquireLock(root string, how int) (*fileLock, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create store root for lock: %w", err)
	}
	path := filepath.Join(root, lockFileName)
	// #nosec G304 -- path is the store-root sentinel file, not user input.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open store lock: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock store lock: %w", err)
	}
	return &fileLock{f: f}, nil
}

// Close releases the flock and closes the descriptor. Safe to call
// once; subsequent calls return an error.
func (l *fileLock) Close() error {
	if l == nil || l.f == nil {
		return errors.New("lock already released")
	}
	defer func() {
		l.f.Close()
		l.f = nil
	}()
	return unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
}
