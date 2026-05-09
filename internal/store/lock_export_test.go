package store

// ExportLockCloser is the test-only public wrapper for fileLock so
// store_test (external package) can verify panic-safe release.
type ExportLockCloser interface {
	Close() error
}

// ExportLockExclusive is the test-only entry point that returns the
// internal exclusive flock as an ExportLockCloser.
func ExportLockExclusive(root string) (ExportLockCloser, error) {
	return lockExclusive(root)
}
