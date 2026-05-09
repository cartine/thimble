// Package cli K-55 peer subcommand: routes
// `thimble peer <add|remove|list|join>` to the membership-management
// helpers in internal/peer. Membership edits are local-only — they
// edit the on-disk peers file. Replication on mutation lives in K-56;
// heartbeats live in K-57.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/peer"
)

// runPeer dispatches the peer subcommands. Kept in its own file so
// the dispatcher in cli.go stays a flat switch.
func runPeer(cfg cliConfig, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return errors.New(
			"usage: thimble peer <add|remove|list|join|ping|status> ...",
		)
	}
	switch args[0] {
	case "add":
		return runPeerAdd(cfg, args[1:], stdout, stderr)
	case "remove", "rm":
		return runPeerRemove(cfg, args[1:], stdout, stderr)
	case "list", "ls":
		return runPeerList(cfg, args[1:], stdout)
	case "join":
		return runPeerJoin(cfg, args[1:], stdout, stderr)
	case "ping":
		return runPeerPing(cfg, args[1:], stdout)
	case "status":
		return runPeerStatus(cfg, args[1:], stdout)
	default:
		return fmt.Errorf(
			"unknown peer subcommand %q; expected add|remove|list|join|ping|status",
			args[0],
		)
	}
}

// runPeerAdd implements `thimble peer add <name> <ssh-target>`. The
// argument order is fixed; we don't accept --name=... flags so that
// human-typed adds compose with shell completion of names. After the
// add succeeds the operator should run `thimble peer list` to verify.
func runPeerAdd(cfg cliConfig, args []string, stdout, stderr io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: thimble peer add <name> <ssh-target>")
	}
	name, target := args[0], args[1]
	mgr, err := peer.Load(cfg.storeDir)
	if err != nil {
		return err
	}
	if err := mgr.Add(peer.Peer{Name: name, Target: target}); err != nil {
		return err
	}
	if err := mgr.Save(); err != nil {
		return err
	}
	recordPeerAudit(cfg, audit.OpPeerAdd, name, stderr)
	fmt.Fprintf(stdout, "added peer %s\n", name)
	return nil
}

// runPeerRemove implements `thimble peer remove <name>`. Removal is
// idempotent in spirit but we error on a missing name so a typo
// surfaces; the operator can `peer list` first if uncertain.
func runPeerRemove(cfg cliConfig, args []string, stdout, stderr io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: thimble peer remove <name>")
	}
	name := args[0]
	mgr, err := peer.Load(cfg.storeDir)
	if err != nil {
		return err
	}
	if err := mgr.Remove(name); err != nil {
		return err
	}
	if err := mgr.Save(); err != nil {
		return err
	}
	recordPeerAudit(cfg, audit.OpPeerRemove, name, stderr)
	fmt.Fprintf(stdout, "removed peer %s\n", name)
	return nil
}

// runPeerList implements `thimble peer list`. Output is tabular,
// space-padded for readability rather than parseable. When no peers
// are configured we emit a hint pointing at `peer add` so the
// operator isn't left wondering what they're missing.
func runPeerList(cfg cliConfig, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: thimble peer list")
	}
	mgr, err := peer.Load(cfg.storeDir)
	if err != nil {
		return err
	}
	peers := mgr.List()
	if len(peers) == 0 {
		fmt.Fprintln(stdout, "no peers configured; run: thimble peer add <name> <ssh-target>")
		return nil
	}
	maxName := len("name")
	for _, p := range peers {
		if len(p.Name) > maxName {
			maxName = len(p.Name)
		}
	}
	fmt.Fprintf(stdout, "%-*s  %s\n", maxName, "name", "target")
	for _, p := range peers {
		fmt.Fprintf(stdout, "%-*s  %s\n", maxName, p.Name, p.Target)
	}
	return nil
}

// runPeerJoin implements `thimble peer join <ssh-target>`. The
// command rsyncs the entire secrets/ tree from the target into the
// local store. Bootstrap refuses to overwrite an existing
// non-trivial store unless --replace is passed; the operator must
// opt in to losing local state.
func runPeerJoin(cfg cliConfig, args []string, stdout, stderr io.Writer) error {
	target, replace, err := parseJoinArgs(args)
	if err != nil {
		return err
	}
	if !replace {
		if err := refuseIfStorePopulated(cfg.storeDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(cfg.storeDir, 0o700); err != nil {
		return err
	}
	if err := runRsyncJoin(target, cfg.storeDir, stderr); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "joined as peer of %s\n", target)
	return nil
}

// parseJoinArgs separates --replace from the single target
// positional. The flag may appear before or after the target.
func parseJoinArgs(args []string) (string, bool, error) {
	var (
		positional []string
		replace    bool
	)
	for _, a := range args {
		switch {
		case a == "--replace":
			replace = true
		case strings.HasPrefix(a, "-"):
			return "", false, fmt.Errorf("unknown peer join flag %q", a)
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return "", false, errors.New(
			"usage: thimble peer join [--replace] <ssh-target>",
		)
	}
	target := positional[0]
	if err := validateTargetForJoin(target); err != nil {
		return "", false, err
	}
	return target, replace, nil
}

// validateTargetForJoin reuses peer.ValidatePeer's target shape
// check by constructing a synthetic Peer.
func validateTargetForJoin(target string) error {
	return peer.ValidatePeer(peer.Peer{Name: "join-target", Target: target})
}

// refuseIfStorePopulated returns an error if the store at storeDir
// has any visible thimble.json content or any *.env.age bundles. The
// goal is to refuse to clobber a leader's existing state while still
// allowing a fresh `thimble peer join` into a never-initialized
// directory.
func refuseIfStorePopulated(storeDir string) error {
	manifestPath := filepath.Join(storeDir, "thimble.json")
	// #nosec G304 -- storeDir is the configured store root.
	if info, err := os.Stat(manifestPath); err == nil && info.Size() > 2 {
		return fmt.Errorf(
			"%s is non-empty; refusing to overwrite. " +
				"Pass --replace to bootstrap on top of existing state",
			manifestPath,
		)
	}
	bundles, err := filepath.Glob(filepath.Join(storeDir, "*", "*.env.age"))
	if err != nil {
		return err
	}
	if len(bundles) > 0 {
		return fmt.Errorf(
			"local store has %d encrypted bundle(s); refusing to overwrite. " +
				"Pass --replace to bootstrap on top of existing state",
			len(bundles),
		)
	}
	return nil
}

// runRsyncJoin shells out to rsync to mirror target's secrets/ into
// storeDir. The trailing slash on src is intentional — without it
// rsync nests an extra directory level. We bound the call with a
// 5-minute context so a stuck transport eventually fails the
// command.
func runRsyncJoin(target, storeDir string, stderr io.Writer) error {
	rsync := os.Getenv("THIMBLE_RSYNC_BINARY")
	if rsync == "" {
		rsync = "rsync"
	}
	resolved, err := exec.LookPath(rsync)
	if err != nil {
		return fmt.Errorf("rsync not found on PATH: %w", err)
	}
	src := target
	if !strings.HasSuffix(src, "/") {
		src += "/"
	}
	dst := storeDir
	if !strings.HasSuffix(dst, "/") {
		dst += "/"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// #nosec G204 -- rsync resolved via LookPath; src is validated by
	// peer.ValidatePeer before reaching this call site.
	cmd := exec.CommandContext(ctx, resolved, "-av", "--delete", src, dst)
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	return cmd.Run()
}

// recordPeerAudit appends a peer_add / peer_remove event to the
// audit log if a logger is configured. We don't have a Store on this
// path so we wire the logger directly via audit.New + the operator
// thumbprint resolved from the configured identity file.
func recordPeerAudit(cfg cliConfig, op, name string, warn io.Writer) {
	logger := audit.New(cfg.storeDir, warn)
	thumb := audit.UnknownOperator
	if cfg.identity != "" {
		recipient, err := audit.PublicRecipientFromIdentityFile(cfg.identity)
		if err == nil {
			thumb = audit.Thumbprint(recipient)
		}
	}
	_ = logger.Append(audit.Event{
		Operator: thumb,
		Op:       op,
		Subject:  name,
	})
}
