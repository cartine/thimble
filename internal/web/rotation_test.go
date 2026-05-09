package web_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/web"
)

// syncBuffer is a *bytes.Buffer guarded by a mutex so concurrent
// goroutines (the rotation loop running on its own goroutine, the
// test polling for "did it rotate yet?") can interact without
// stepping on each other. bytes.Buffer is not safe for concurrent
// use; this wrapper closes that gap for the rotation tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newSyncBuffer() *syncBuffer { return &syncBuffer{} }

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestRotateInvalidatesExistingCookie covers AC#6: a cookie minted
// against token T1 must stop authorizing requests after Rotate runs;
// a fresh login with the new token must succeed.
func TestRotateInvalidatesExistingCookie(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("webapp", "dev", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	server := web.New(st, "T1", true)
	mux := http.NewServeMux()
	server.Routes(mux)
	handler := web.NoStoreMiddleware(mux)

	cookie := loginAndExtractCookie(t, handler, "T1")
	if _, status := getBodyWithCookie(handler, "/", cookie); status != http.StatusOK {
		t.Fatalf("pre-rotation auth status = %d, want 200", status)
	}

	var stdout strings.Builder
	if err := server.Rotate(&stdout); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "web token rotated; current token printed below:\n") {
		t.Fatalf("rotate stdout missing prefix line: %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 || lines[1] == "" {
		t.Fatalf("rotate stdout did not print a token after the prefix: %q", out)
	}
	newToken := lines[1]
	if newToken == "T1" {
		t.Fatalf("rotate produced same token: %q", newToken)
	}

	if status := getStatus(handler, "/"); status != http.StatusSeeOther {
		t.Fatalf("post-rotate / status = %d, want %d (redirect to /login)",
			status, http.StatusSeeOther)
	}
	if _, status := getBodyWithCookie(handler, "/", cookie); status != http.StatusSeeOther {
		t.Fatalf("post-rotate stale-cookie / status = %d, want redirect", status)
	}
	fresh := loginAndExtractCookie(t, handler, newToken)
	if _, status := getBodyWithCookie(handler, "/", fresh); status != http.StatusOK {
		t.Fatalf("post-rotate fresh-cookie / status = %d, want 200", status)
	}
}

// TestIdleRotationFiresWithinSmallWindow drives RunIdleRotation with
// a 50ms window and no traffic; the token must rotate within a short
// real-time budget. We capture the post-rotate stdout banner to
// observe the rotation deterministically rather than racing the
// cookie-validity probe (which would itself reset the idle timer).
func TestIdleRotationFiresWithinSmallWindow(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("webapp", "dev", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	server := web.New(st, "T1", true)
	mux := http.NewServeMux()
	server.Routes(mux)
	handler := web.NoStoreMiddleware(mux)
	cookie := loginAndExtractCookie(t, handler, "T1")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stdout := newSyncBuffer()
	done := make(chan struct{})
	go func() {
		_ = server.RunIdleRotation(ctx, 50*time.Millisecond, stdout)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	rotated := false
	for time.Now().Before(deadline) {
		if strings.Contains(stdout.String(),
			"web token rotated; current token printed below:") {
			rotated = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if !rotated {
		t.Fatalf("idle rotation never logged within deadline; stdout=%q",
			stdout.String())
	}
	if _, status := getBodyWithCookie(handler, "/", cookie); status != http.StatusSeeOther {
		t.Fatalf("post-rotate stale-cookie status = %d, want redirect", status)
	}
}

// TestRunIdleRotationStopsOnContextCancel guards the no-leak
// invariant: the goroutine must return promptly when ctx is
// canceled. Tested by waiting on a done channel after cancel.
func TestRunIdleRotationStopsOnContextCancel(t *testing.T) {
	st := newTestStore(t)
	server := web.New(st, "T1", true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = server.RunIdleRotation(ctx, 1*time.Hour, io.Discard)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("RunIdleRotation did not return within 2s of ctx cancel")
	}
}

// TestRunIdleRotationDisabledWhenZero verifies that --idle-rotate=0
// (the operator-disabled path) returns immediately without spinning
// up a timer.
func TestRunIdleRotationDisabledWhenZero(t *testing.T) {
	st := newTestStore(t)
	server := web.New(st, "T1", true)
	start := time.Now()
	if err := server.RunIdleRotation(context.Background(), 0,
		io.Discard); err != nil {
		t.Fatalf("RunIdleRotation(0): %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("RunIdleRotation(0) blocked unexpectedly")
	}
}

// TestActivityResetsIdleTimer asserts that authorized requests
// arriving regularly keep the timer from firing. We hold the loop
// active for ~250ms while authorizing every 30ms and check that the
// original cookie still works at the end.
func TestActivityResetsIdleTimer(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("webapp", "dev", []string{testRecipientOperator}); err != nil {
		t.Fatalf("init: %v", err)
	}
	server := web.New(st, "T1", true)
	mux := http.NewServeMux()
	server.Routes(mux)
	handler := web.NoStoreMiddleware(mux)
	cookie := loginAndExtractCookie(t, handler, "T1")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		_ = server.RunIdleRotation(ctx, 100*time.Millisecond, io.Discard)
		close(done)
	}()

	keepAliveDeadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(keepAliveDeadline) {
		_, status := getBodyWithCookie(handler, "/", cookie)
		if status != http.StatusOK {
			t.Fatalf("during keep-alive got status = %d, want 200", status)
		}
		time.Sleep(30 * time.Millisecond)
	}
	cancel()
	<-done
}

// TestRotateWithNilWriterDoesNotPanic guards the inadvertent-nil
// case: tests and shutdown paths might pass io.Discard, but a future
// caller passing nil should not crash the server.
func TestRotateWithNilWriterDoesNotPanic(t *testing.T) {
	st := newTestStore(t)
	server := web.New(st, "T1", true)
	if err := server.Rotate(nil); err != nil {
		t.Fatalf("rotate(nil): %v", err)
	}
}

// TestRotateLogsOncePerCall guards against the "double-print" bug:
// each Rotate call must produce exactly one prefix+token pair,
// even when called back-to-back.
func TestRotateLogsOncePerCall(t *testing.T) {
	st := newTestStore(t)
	server := web.New(st, "T1", true)
	var stdout strings.Builder
	if err := server.Rotate(&stdout); err != nil {
		t.Fatalf("rotate1: %v", err)
	}
	if err := server.Rotate(&stdout); err != nil {
		t.Fatalf("rotate2: %v", err)
	}
	count := strings.Count(stdout.String(),
		"web token rotated; current token printed below:")
	if count != 2 {
		t.Fatalf("rotate logged prefix %d times across 2 calls, want 2; out=%q",
			count, stdout.String())
	}
}

// loginPostStatus posts a token to /login and returns the response
// status. Used by tests that want to assert login success without
// needing the cookie itself.
func loginPostStatus(t *testing.T, mux http.Handler, token string) int {
	t.Helper()
	form := "token=" + token
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// TestRotateUpdatesLoginToken confirms /login starts accepting the
// new token after Rotate and rejects the previous one. The companion
// to TestRotateInvalidatesExistingCookie focused on issued cookies.
func TestRotateUpdatesLoginToken(t *testing.T) {
	st := newTestStore(t)
	server := web.New(st, "T1", true)
	mux := http.NewServeMux()
	server.Routes(mux)

	if got := loginPostStatus(t, mux, "T1"); got != http.StatusSeeOther {
		t.Fatalf("login with T1 pre-rotate status = %d, want %d",
			got, http.StatusSeeOther)
	}
	var stdout strings.Builder
	if err := server.Rotate(&stdout); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if got := loginPostStatus(t, mux, "T1"); got != http.StatusUnauthorized {
		t.Fatalf("login with stale T1 post-rotate status = %d, want 401", got)
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("rotate did not print a token: %q", stdout.String())
	}
	newToken := lines[1]
	if got := loginPostStatus(t, mux, newToken); got != http.StatusSeeOther {
		t.Fatalf("login with new token status = %d, want %d",
			got, http.StatusSeeOther)
	}
}
