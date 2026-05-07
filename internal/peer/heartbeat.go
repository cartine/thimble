package peer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultPingTimeout is the per-peer reachability timeout for an
// individual heartbeat ping. The ssh + rsync pair is bounded by this
// duration; an unreachable peer fails fast.
const DefaultPingTimeout = 10 * time.Second

// pingFanOut bounds the number of in-flight pings during PingAll.
// The ping is a query — it has no per-process state — so a small
// pool keeps fan-out cheap without flooding the operator's ssh
// agent.
const pingFanOut = 4

// PingResult is one peer's heartbeat outcome. Health is the parsed
// PeerHealth (last_seen + manifest_versions); Err is non-nil if the
// ping failed for any reason.
type PingResult struct {
	Health PeerHealth
	Err    error
}

// Ping runs a single peer reachability check: ssh "true" for
// liveness, plus rsync to fetch the remote thimble.json so we can
// extract per-namespace versions. The function is bounded by ctx
// AND by DefaultPingTimeout — whichever fires first cancels the
// subprocesses.
func Ping(ctx context.Context, p Peer) PingResult {
	pingCtx, cancel := context.WithTimeout(ctx, DefaultPingTimeout)
	defer cancel()
	host, _, err := splitTarget(p.Target)
	if err != nil {
		return PingResult{Err: err}
	}
	if err := runSSHTrue(pingCtx, host); err != nil {
		return PingResult{Err: fmt.Errorf("ssh: %w", err)}
	}
	versions, err := fetchRemoteVersions(pingCtx, p.Target)
	if err != nil {
		return PingResult{Err: fmt.Errorf("manifest read: %w", err)}
	}
	return PingResult{Health: PeerHealth{
		LastSeen:         time.Now().UTC(),
		ManifestVersions: versions,
	}}
}

// PingAll runs Ping concurrently across every peer in mgr, capped
// at pingFanOut workers. The returned map is keyed by peer name.
// State-file updates are written serially after the workers join so
// the JSON file is never partially overwritten.
func PingAll(ctx context.Context, mgr *Manager, storeRoot string) map[string]PingResult {
	if mgr == nil {
		return map[string]PingResult{}
	}
	peers := mgr.List()
	results := make(map[string]PingResult, len(peers))
	if len(peers) == 0 {
		return results
	}
	var (
		mu   sync.Mutex
		wg   sync.WaitGroup
		sema = make(chan struct{}, pingFanOut)
	)
	for _, p := range peers {
		wg.Add(1)
		sema <- struct{}{}
		go func(p Peer) {
			defer wg.Done()
			defer func() { <-sema }()
			r := Ping(ctx, p)
			mu.Lock()
			results[p.Name] = r
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	for name, r := range results {
		writePingResult(storeRoot, name, r)
	}
	return results
}

// writePingResult persists a single ping outcome to .peer-state.json.
// On success the manifest_versions field is overwritten and
// last_error is cleared; on failure last_error is set and the prior
// last_seen is preserved.
func writePingResult(storeRoot, name string, r PingResult) {
	if r.Err != nil {
		_ = RecordPushFailure(storeRoot, name, r.Err)
		return
	}
	_ = RecordPingResult(storeRoot, name, r.Health)
}

// splitTarget splits an rsync-style `[user@]host:path` target into
// the ssh host portion (with any user@ preserved) and the remote
// path.
func splitTarget(target string) (string, string, error) {
	colon := strings.IndexByte(target, ':')
	if colon <= 0 || colon == len(target)-1 {
		return "", "", errors.New("malformed peer target")
	}
	return target[:colon], target[colon+1:], nil
}

// runSSHTrue runs `ssh <host> true` to verify reachability. The
// binary is resolved via THIMBLE_SSH_BINARY (overrides PATH) so
// tests can stub ssh.
func runSSHTrue(ctx context.Context, host string) error {
	bin := os.Getenv("THIMBLE_SSH_BINARY")
	if bin == "" {
		bin = "ssh"
	}
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return fmt.Errorf("ssh not found on PATH: %w", err)
	}
	// #nosec G204 -- bin is resolved via LookPath; host comes from
	// peer.ValidatePeer-vetted target.
	cmd := exec.CommandContext(ctx, resolved,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		host, "true",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, truncateError(string(output), 80))
	}
	return nil
}

// fetchRemoteVersions copies the remote thimble.json into a temp
// file, parses it, and returns a flat map of "<app>/<env>" →
// version. The temp file is cleaned up before returning.
func fetchRemoteVersions(ctx context.Context, target string) (map[string]int, error) {
	bin := os.Getenv("THIMBLE_RSYNC_BINARY")
	if bin == "" {
		bin = "rsync"
	}
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("rsync not found on PATH: %w", err)
	}
	tmp, err := os.CreateTemp("", "thimble-peer-manifest-*.json")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)
	src := strings.TrimSuffix(target, "/") + "/thimble.json"
	// #nosec G204 -- bin is resolved via LookPath; src is composed
	// from a target validated by peer.ValidatePeer.
	cmd := exec.CommandContext(ctx, resolved, "-q", src, tmpName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, truncateError(string(out), 80))
	}
	return parseManifestVersions(tmpName)
}

// parseManifestVersions reads a Thimble manifest file and extracts
// the per-namespace Version field as a flat map.
func parseManifestVersions(path string) (map[string]int, error) {
	// #nosec G304 -- path is a temp file we just created.
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var m struct {
		Apps map[string]struct {
			Environments map[string]struct {
				Version int `json:"version"`
			} `json:"environments"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := map[string]int{}
	for app, appMeta := range m.Apps {
		for env, envMeta := range appMeta.Environments {
			out[app+"/"+env] = envMeta.Version
		}
	}
	return out, nil
}
