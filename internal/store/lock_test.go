package store_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/cartine/thimble/internal/store"
)

// TestRewriteEnvConcurrentDifferentKeysOneFails arranges two
// goroutines that load the same env at the same Version, mutate
// different keys, then race to commit. K-21 requires the loser to
// fail with "another writer changed app/env; rerun".
func TestRewriteEnvConcurrentDifferentKeysOneFails(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "EXISTING", "ok"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	runConcurrentSet(t, st, []string{"K_ALPHA", "K_BRAVO"})
}

// TestRewriteEnvConcurrentSameKeyOneFails covers the same-key race:
// the loser still fails cleanly with the K-21 error message.
func TestRewriteEnvConcurrentSameKeyOneFails(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	runConcurrentSet(t, st, []string{"TOKEN", "TOKEN"})
}

// TestLockReleasedOnPanic ensures a panic during a write still
// releases the flock so subsequent operations are not deadlocked.
// The deferred Close in lockExclusive provides this guarantee; we
// exercise it by panicking inside a goroutine that has acquired the
// lock, then verifying the next write succeeds.
func TestLockReleasedOnPanic(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		runWithExclusiveLockThenPanic(t, st.Root())
	}()
	<-done
	if err := st.SetSecret("svc", "prod", "AFTER", "v"); err != nil {
		t.Fatalf("set after panicking writer: %v", err)
	}
}

// runConcurrentSet starts len(keys) goroutines that call SetSecret
// concurrently and asserts that exactly one succeeds and the rest
// fail with the K-21 conflict message.
func runConcurrentSet(t *testing.T, st *store.Store, keys []string) {
	t.Helper()
	results := make(chan error, len(keys))
	var wg sync.WaitGroup
	wg.Add(len(keys))
	for _, k := range keys {
		go func(key string) {
			defer wg.Done()
			results <- st.SetSecret("svc", "prod", key, "v")
		}(k)
	}
	wg.Wait()
	close(results)
	ok, conflict := classifyResults(t, results)
	if ok != 1 || conflict != len(keys)-1 {
		t.Fatalf("ok=%d conflict=%d, want 1 of each (keys=%v)", ok, conflict, keys)
	}
}

// classifyResults counts successes and K-21 conflict failures,
// failing the test on any unexpected error shape.
func classifyResults(t *testing.T, ch <-chan error) (int, int) {
	t.Helper()
	var ok, conflict int
	for err := range ch {
		if err == nil {
			ok++
			continue
		}
		if !strings.Contains(err.Error(), "another writer changed svc/prod; rerun") {
			t.Fatalf("unexpected error: %v", err)
		}
		conflict++
	}
	return ok, conflict
}

// runWithExclusiveLockThenPanic acquires the exclusive store lock
// via the package-internal helper exposed by ExportLockExclusive
// (added for tests in lock_export_test.go), then panics. The
// deferred Close still releases the flock.
func runWithExclusiveLockThenPanic(t *testing.T, root string) {
	t.Helper()
	lock, err := store.ExportLockExclusive(root)
	if err != nil {
		t.Fatalf("lockExclusive: %v", err)
	}
	defer lock.Close()
	panic("intentional test panic")
}
