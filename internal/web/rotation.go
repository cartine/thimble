package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"time"
)

// rotatedTokenPrefix is the operator-visible line emitted to stdout
// whenever the web token rotates (manual or idle). The new token is
// printed on the next line so a tail on stdout can pick it up.
const rotatedTokenPrefix = "web token rotated; current token printed below:"

// RandomToken returns a 32-byte URL-safe random token, the same shape
// the CLI uses to generate the startup token. Exported so cli/run_web
// can mint the initial token via the same helper.
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Rotate generates a fresh token, swaps it in under write-lock, and
// prints `web token rotated; current token printed below:\n<token>`
// to stdout. Existing session cookies (which encode the old token)
// fail constant-time compare on the next request and the operator
// must paste the new token at /login. Safe to call concurrently with
// in-flight requests; safe to call back-to-back (each call logs once).
func (s *Server) Rotate(stdout io.Writer) error {
	token, err := RandomToken()
	if err != nil {
		return err
	}
	s.setToken(token)
	if stdout != nil {
		fmt.Fprintln(stdout, rotatedTokenPrefix)
		fmt.Fprintln(stdout, token)
	}
	s.bumpActivity()
	return nil
}

// markAuthorized signals the idle watcher (if running) that an
// authorized request just landed. Non-blocking — the channel is
// 1-buffered so a flurry of requests collapses into a single reset.
func (s *Server) markAuthorized() {
	s.bumpActivity()
}

// bumpActivity is the internal non-blocking send used by both
// markAuthorized (request path) and Rotate (so the post-rotate idle
// window starts fresh). Returns immediately if the buffer is full;
// when no watcher is running the slot stays full harmlessly until
// the next watcher drains it.
func (s *Server) bumpActivity() {
	select {
	case s.activity <- struct{}{}:
	default:
	}
}

// RunIdleRotation watches for authorized requests and rotates the
// token whenever `idle` elapses without one. It blocks until ctx is
// canceled and returns then; the underlying timer is stopped on exit
// so there is no goroutine or timer leak when the controlling
// process shuts down. `stdout` receives the same banner Rotate
// prints. `idle` <= 0 disables the loop and returns immediately
// (operators who pass --idle-rotate=0 opt out).
func (s *Server) RunIdleRotation(ctx context.Context, idle time.Duration,
	stdout io.Writer) error {
	if idle <= 0 {
		return nil
	}
	// Drain any signal queued before the watcher started so the
	// freshly-armed timer measures from "now" rather than firing
	// immediately on an inherited bump.
	drainOnce(s.activity)
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.activity:
			resetTimer(timer, idle)
		case <-timer.C:
			if err := s.Rotate(stdout); err != nil {
				return err
			}
			// Rotate bumped activity; drain so the timer starts
			// fresh from "now" instead of "right after rotate".
			drainOnce(s.activity)
			resetTimer(timer, idle)
		}
	}
}

// resetTimer is the canonical pattern for time.Timer.Reset on a
// possibly-fired timer. Without this, a concurrent timer fire racing
// with Reset can leave the channel with a stale value.
func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

// drainOnce non-blockingly removes one pending value from a buffered
// channel, if any. Used post-rotate so the activity slot Rotate
// inserted does not immediately reset the freshly-armed timer, and
// at watcher startup so a stale signal does not skip the first idle
// window.
func drainOnce(c chan struct{}) {
	select {
	case <-c:
	default:
	}
}
