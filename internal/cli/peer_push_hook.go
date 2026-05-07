package cli

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/cartine/thimble/internal/peer"
)

// peerPushDisableEnv is the env var that globally disables the
// on-mutate broadcast. Setting it to "off" or "0" or "false" turns
// Thimble into single-leader mode.
const peerPushDisableEnv = "THIMBLE_PEER_PUSH"

// maybePushPeers runs the K-56 on-mutate broadcast unless the
// suppress flag, the disable env, or an empty peers list says
// otherwise. The push is best-effort — failures are logged to
// stderr and recorded in .peer-state.json but do not roll back the
// caller's mutation.
func maybePushPeers(cfg cliConfig, suppress bool, stderr io.Writer) {
	if suppress {
		return
	}
	if peerPushGloballyDisabled() {
		return
	}
	mgr, err := peer.Load(cfg.storeDir)
	if err != nil {
		return
	}
	if len(mgr.List()) == 0 {
		return
	}
	_ = peer.PushChanges(context.Background(), mgr, cfg.storeDir, stderr)
}

// peerPushGloballyDisabled returns true if the env var instructs
// us to skip pushes. We accept "off", "0", "false" (case-insensitive)
// for friendliness; any other value (including empty) leaves the
// push enabled.
func peerPushGloballyDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(peerPushDisableEnv)))
	switch v {
	case "off", "0", "false", "no":
		return true
	default:
		return false
	}
}

// extractNoPeerPush walks args, removes any --no-peer-push (or
// --no-peer-push=true) entries, and returns (suppress, remaining).
// We process this at the dispatcher level so per-subcommand parsers
// don't each have to know about the flag.
func extractNoPeerPush(args []string) (bool, []string) {
	out := make([]string, 0, len(args))
	suppress := false
	for _, a := range args {
		if a == "--no-peer-push" {
			suppress = true
			continue
		}
		if strings.HasPrefix(a, "--no-peer-push=") {
			val := strings.TrimPrefix(a, "--no-peer-push=")
			lower := strings.ToLower(val)
			if lower == "true" || lower == "1" || lower == "yes" {
				suppress = true
			}
			continue
		}
		out = append(out, a)
	}
	return suppress, out
}
