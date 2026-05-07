package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/cartine/thimble/internal/peer"
)

// runPeerPing implements `thimble peer ping [<name>] [--quiet]`. The
// command runs heartbeats against either one named peer or every
// configured peer and updates .peer-state.json. With --quiet, the
// command writes nothing on success but still propagates a non-zero
// exit if any peer fails.
func runPeerPing(cfg cliConfig, args []string, stdout io.Writer) error {
	name, quiet, err := parsePingArgs(args)
	if err != nil {
		return err
	}
	mgr, err := peer.Load(cfg.storeDir)
	if err != nil {
		return err
	}
	peers, err := selectPingPeers(mgr, name)
	if err != nil {
		return err
	}
	if len(peers) == 0 {
		fmt.Fprintln(stdout, "no peers configured; run: thimble peer add <name> <ssh-target>")
		return nil
	}
	results := pingSelected(cfg.storeDir, peers)
	failures := emitPingResults(stdout, peers, results, quiet)
	if failures > 0 {
		return fmt.Errorf("%d peer ping(s) failed", failures)
	}
	return nil
}

// parsePingArgs separates the optional --quiet flag and a single
// positional name. Returns (name, quiet, err); name is empty when
// the operator wants all peers pinged.
func parsePingArgs(args []string) (string, bool, error) {
	var (
		positional []string
		quiet      bool
	)
	for _, a := range args {
		switch {
		case a == "--quiet" || a == "-q":
			quiet = true
		case strings.HasPrefix(a, "-"):
			return "", false, fmt.Errorf("unknown peer ping flag %q", a)
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) > 1 {
		return "", false, errors.New("usage: thimble peer ping [<name>] [--quiet]")
	}
	if len(positional) == 1 {
		return positional[0], quiet, nil
	}
	return "", quiet, nil
}

// selectPingPeers returns either the full peer list or just the
// named one. A name miss is reported as an error.
func selectPingPeers(mgr *peer.Manager, name string) ([]peer.Peer, error) {
	if name == "" {
		return mgr.List(), nil
	}
	p, ok := mgr.Find(name)
	if !ok {
		return nil, fmt.Errorf("peer %q not found", name)
	}
	return []peer.Peer{p}, nil
}

// pingSelected runs Ping for each peer in the list. We use Ping (not
// PingAll) so the order matches the input list and the results map
// to the correct peer. Single-peer ping is the common case (cron),
// so we keep this lean rather than always fanning out.
func pingSelected(storeRoot string, peers []peer.Peer) map[string]peer.PingResult {
	results := map[string]peer.PingResult{}
	for _, p := range peers {
		r := peer.Ping(context.Background(), p)
		results[p.Name] = r
		if r.Err != nil {
			_ = peer.RecordPushFailure(storeRoot, p.Name, r.Err)
			continue
		}
		_ = peer.RecordPingResult(storeRoot, p.Name, r.Health)
	}
	return results
}

// emitPingResults prints a one-line summary per peer (or stays
// silent under --quiet on success). Returns the failure count so
// the caller can adjust the exit code.
func emitPingResults(
	stdout io.Writer, peers []peer.Peer,
	results map[string]peer.PingResult, quiet bool,
) int {
	failures := 0
	for _, p := range peers {
		r := results[p.Name]
		if r.Err != nil {
			failures++
			fmt.Fprintf(stdout, "%s  FAIL  %s\n", p.Name, peer.PushError{Peer: p.Name, Err: r.Err}.Error())
			continue
		}
		if quiet {
			continue
		}
		fmt.Fprintf(stdout, "%s  OK    %s\n", p.Name, formatVersions(r.Health.ManifestVersions))
	}
	return failures
}

// formatVersions renders a tiny "abc/production:43,abc/staging:12"
// summary; deterministic by sorting the keys.
func formatVersions(versions map[string]int) string {
	if len(versions) == 0 {
		return "(no namespaces)"
	}
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, versions[k]))
	}
	return "versions=" + strings.Join(parts, ",")
}

// runPeerStatus implements `thimble peer status`. The command reads
// the on-disk .peer-state.json (no fresh ping) and prints a tabular
// summary joined with peers.toml.
func runPeerStatus(cfg cliConfig, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: thimble peer status")
	}
	mgr, err := peer.Load(cfg.storeDir)
	if err != nil {
		return err
	}
	state, err := peer.LoadState(cfg.storeDir)
	if err != nil {
		return err
	}
	peers := mgr.List()
	if len(peers) == 0 {
		fmt.Fprintln(stdout, "no peers configured; run: thimble peer add <name> <ssh-target>")
		return nil
	}
	if len(state.Peers) == 0 {
		fmt.Fprintln(stdout, "no peers contacted yet; run: thimble peer ping")
		return nil
	}
	printStatusTable(stdout, peers, state)
	return nil
}

// printStatusTable writes the joined peers + state table in a
// stable column layout. Stale entries (>1h) are tagged inline so an
// operator scanning the output spots them.
func printStatusTable(stdout io.Writer, peers []peer.Peer, state peer.State) {
	maxName := len("name")
	maxTarget := len("target")
	for _, p := range peers {
		if len(p.Name) > maxName {
			maxName = len(p.Name)
		}
		if len(p.Target) > maxTarget {
			maxTarget = len(p.Target)
		}
	}
	fmt.Fprintf(stdout, "%-*s  %-*s  %-25s  %s\n",
		maxName, "name", maxTarget, "target", "last_seen", "last_error",
	)
	for _, p := range peers {
		h := state.Peers[p.Name]
		fmt.Fprintf(stdout, "%-*s  %-*s  %-25s  %s\n",
			maxName, p.Name, maxTarget, p.Target,
			renderLastSeen(h.LastSeen), renderLastError(h.LastError),
		)
	}
}

// renderLastSeen renders a UTC RFC3339 timestamp with a "(stale)"
// suffix when the last contact is older than one hour. Zero values
// render as "(never)" so the operator can tell "never reached"
// apart from "stale".
func renderLastSeen(t time.Time) string {
	if t.IsZero() {
		return "(never)"
	}
	stamp := t.UTC().Format(time.RFC3339)
	if time.Since(t) > time.Hour {
		return stamp + " (stale)"
	}
	return stamp
}

// renderLastError renders the empty-string case as "—" (em-dash) so
// the operator's eye doesn't have to resolve a blank cell.
func renderLastError(e string) string {
	if e == "" {
		return "-"
	}
	return e
}
