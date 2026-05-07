package peer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DefaultPushTimeout is the per-peer rsync timeout when no override
// is set. K-56 hits this if a peer is offline or slow; the local
// mutation is unaffected.
const DefaultPushTimeout = 30 * time.Second

// PushError is one entry in the failure list returned by
// PushChanges. The Peer field is the human handle from peers.toml so
// the operator can act on the message.
type PushError struct {
	Peer string
	Err  error
}

// Error returns "peer push failed: <name>: <reason>" so a
// PushError satisfies the error interface for logging.
func (p PushError) Error() string {
	return fmt.Sprintf("peer push failed: %s: %v", p.Peer, p.Err)
}

// PushChanges replicates the storeRoot tree to every peer in mgr
// using rsync over ssh. The push runs sequentially across peers so
// no goroutine outlives the function. Each per-peer call is bounded
// by a context.WithTimeout. Per-peer success/failure is recorded in
// the .peer-state.json file alongside whatever K-57 has written.
//
// PushChanges never returns an error itself — peer failures are
// returned as a slice and the caller decides whether to surface
// them. The local store is the source of truth; a peer being slow
// must not roll back local state.
func PushChanges(
	ctx context.Context, mgr *Manager, storeRoot string, stderr io.Writer,
) []PushError {
	if mgr == nil {
		return nil
	}
	peers := mgr.List()
	if len(peers) == 0 {
		return nil
	}
	timeout := pushTimeoutFromEnv()
	rsync, err := resolveRsync()
	if err != nil {
		fmt.Fprintf(stderr, "peer push skipped: %v\n", err)
		return nil
	}
	var failures []PushError
	for _, p := range peers {
		if err := pushOnePeer(ctx, rsync, storeRoot, p.Target, timeout); err != nil {
			failures = append(failures, PushError{Peer: p.Name, Err: err})
			fmt.Fprintf(stderr, "%s\n", PushError{Peer: p.Name, Err: err}.Error())
			_ = RecordPushFailure(storeRoot, p.Name, err)
			continue
		}
		_ = RecordPushSuccess(storeRoot, p.Name, time.Now())
	}
	return failures
}

// resolveRsync returns the rsync binary path. THIMBLE_RSYNC_BINARY
// overrides PATH lookup for tests and operators who pin a specific
// binary.
func resolveRsync() (string, error) {
	bin := os.Getenv("THIMBLE_RSYNC_BINARY")
	if bin == "" {
		bin = "rsync"
	}
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return "", fmt.Errorf("rsync not found on PATH: %w", err)
	}
	return resolved, nil
}

// pushTimeoutFromEnv reads THIMBLE_PEER_PUSH_TIMEOUT (seconds) and
// falls back to DefaultPushTimeout. A non-numeric value falls back
// silently rather than aborting the push.
func pushTimeoutFromEnv() time.Duration {
	v := os.Getenv("THIMBLE_PEER_PUSH_TIMEOUT")
	if v == "" {
		return DefaultPushTimeout
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return DefaultPushTimeout
	}
	return time.Duration(n) * time.Second
}

// pushOnePeer runs a single rsync invocation against one peer's
// target. Excludes are explicit so peers.toml and the local
// .peer-state.json never travel between leaders — those files are
// per-leader.
func pushOnePeer(
	ctx context.Context, rsync, storeRoot, target string,
	timeout time.Duration,
) error {
	if !strings.Contains(target, ":") {
		return errors.New("malformed peer target")
	}
	src := storeRoot
	if !strings.HasSuffix(src, "/") {
		src += "/"
	}
	dst := target
	if !strings.HasSuffix(dst, "/") {
		dst += "/"
	}
	pushCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := []string{
		"-av", "--delete",
		"--exclude=" + PeersFileName,
		"--exclude=" + StateFileName,
		src, dst,
	}
	// #nosec G204 -- rsync is resolved via LookPath; src is a Thimble
	// store root; target is validated by ValidatePeer at add time and
	// re-checked above.
	cmd := exec.CommandContext(pushCtx, rsync, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, truncateError(string(output), 120))
	}
	return nil
}
