package doctor

import (
	"fmt"
	"sort"
	"time"

	"github.com/cartine/thimble/internal/peer"
	"github.com/cartine/thimble/internal/store"
)

// peerStaleAfter is the threshold beyond which a peer's last_seen
// triggers a warn from the doctor check. Operators who run
// `thimble peer ping --all --quiet` from cron every 5 minutes will
// have all peers comfortably under this.
const peerStaleAfter = time.Hour

// checkPeers reports K-57 peer health. The check returns one
// CheckResult per peer plus a summary line so an operator scanning
// the doctor output sees both the rollup and the offending peer in
// the same listing.
func checkPeers(st *store.Store) []CheckResult {
	mgr, err := peer.Load(st.Root())
	if err != nil {
		return []CheckResult{{
			Name:   "peers",
			Status: StatusFail,
			Detail: fmt.Sprintf("load peers.toml: %v", err),
		}}
	}
	peers := mgr.List()
	if len(peers) == 0 {
		return []CheckResult{{
			Name:   "peers",
			Status: StatusOK,
			Detail: "no peers configured (single-leader mode)",
		}}
	}
	state, err := peer.LoadState(st.Root())
	if err != nil {
		return []CheckResult{{
			Name:   "peers",
			Status: StatusFail,
			Detail: fmt.Sprintf("load .peer-state.json: %v", err),
		}}
	}
	return buildPeerCheckResults(peers, state, time.Now())
}

// buildPeerCheckResults runs the actual classification logic so the
// IO-bound checkPeers stays simple. now is parameterized so tests
// can pin time without monkey-patching.
func buildPeerCheckResults(
	peers []peer.Peer, state peer.State, now time.Time,
) []CheckResult {
	sort.Slice(peers, func(i, j int) bool { return peers[i].Name < peers[j].Name })
	var (
		results []CheckResult
		worst   = StatusOK
	)
	for _, p := range peers {
		result := classifyPeer(p, state.Peers[p.Name], now)
		results = append(results, result)
		worst = worsten(worst, result.Status)
	}
	summary := CheckResult{
		Name:   "peers",
		Status: worst,
		Detail: peerSummaryDetail(worst, len(peers)),
	}
	return append([]CheckResult{summary}, results...)
}

// classifyPeer returns the doctor check result for one peer based
// on its on-disk health.
func classifyPeer(p peer.Peer, h peer.PeerHealth, now time.Time) CheckResult {
	name := fmt.Sprintf("peer[%s]", p.Name)
	if h.LastSeen.IsZero() && h.LastError == "" {
		return CheckResult{
			Name:   name,
			Status: StatusFail,
			Detail: "never contacted; run: thimble peer ping " + p.Name,
		}
	}
	if h.LastError != "" {
		return CheckResult{
			Name:   name,
			Status: StatusFail,
			Detail: fmt.Sprintf("last contact failed: %s", h.LastError),
		}
	}
	age := now.Sub(h.LastSeen)
	if age > peerStaleAfter {
		return CheckResult{
			Name:   name,
			Status: StatusWarn,
			Detail: fmt.Sprintf("stale; last_seen %s ago", age.Truncate(time.Second)),
		}
	}
	return CheckResult{
		Name:   name,
		Status: StatusOK,
		Detail: fmt.Sprintf("last_seen %s ago", age.Truncate(time.Second)),
	}
}

// worsten returns the worst-of-two among ok < warn < fail.
func worsten(a, b Status) Status {
	if a == StatusFail || b == StatusFail {
		return StatusFail
	}
	if a == StatusWarn || b == StatusWarn {
		return StatusWarn
	}
	return StatusOK
}

// peerSummaryDetail renders the rollup detail line for the parent
// "peers" entry.
func peerSummaryDetail(status Status, total int) string {
	switch status {
	case StatusFail:
		return fmt.Sprintf("%d peer(s) configured; one or more unreachable", total)
	case StatusWarn:
		return fmt.Sprintf("%d peer(s) configured; one or more stale (>1h)", total)
	default:
		return fmt.Sprintf("%d peer(s) configured; all healthy", total)
	}
}
