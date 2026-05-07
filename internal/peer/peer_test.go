package peer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/peer"
)

const aliceTarget = "alice@laptop.local:/srv/abc-secrets"
const bobTarget = "bob@bob-laptop.local:/srv/abc-secrets"

// TestRoundTripEmpty ensures Load/Save survive a missing file by
// returning an empty Manager that writes an empty list (header
// comment only) on Save.
func TestRoundTripEmpty(t *testing.T) {
	root := t.TempDir()
	mgr, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := mgr.List(); len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	body, err := os.ReadFile(peer.PeersPath(root))
	if err != nil {
		t.Fatalf("read peers.toml: %v", err)
	}
	if !strings.Contains(string(body), "# Thimble peers") {
		t.Fatalf("expected header comment, got %q", body)
	}
	mgr2, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load (after Save): %v", err)
	}
	if got := mgr2.List(); len(got) != 0 {
		t.Fatalf("expected empty list after round-trip, got %v", got)
	}
}

// TestAddListRemoveRoundTrip exercises the happy path: two peers
// added, listed, removed, and reloaded.
func TestAddListRemoveRoundTrip(t *testing.T) {
	root := t.TempDir()
	mgr, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := mgr.Add(peer.Peer{Name: "alice-laptop", Target: aliceTarget}); err != nil {
		t.Fatalf("Add alice: %v", err)
	}
	if err := mgr.Add(peer.Peer{Name: "bob-laptop", Target: bobTarget}); err != nil {
		t.Fatalf("Add bob: %v", err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	mgr2, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}
	got := mgr2.List()
	if len(got) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(got))
	}
	if got[0].Name != "alice-laptop" || got[0].Target != aliceTarget {
		t.Fatalf("peer 0 mismatch: %+v", got[0])
	}
	if got[1].Name != "bob-laptop" || got[1].Target != bobTarget {
		t.Fatalf("peer 1 mismatch: %+v", got[1])
	}
	if err := mgr2.Remove("alice-laptop"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := mgr2.Save(); err != nil {
		t.Fatalf("Save after remove: %v", err)
	}
	mgr3, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load after remove: %v", err)
	}
	if len(mgr3.List()) != 1 {
		t.Fatalf("expected 1 peer after remove, got %d", len(mgr3.List()))
	}
}

// TestAddDuplicateName ensures Add rejects an attempt to add the
// same name twice.
func TestAddDuplicateName(t *testing.T) {
	mgr, err := peer.Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := mgr.Add(peer.Peer{Name: "alice", Target: aliceTarget}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	err = mgr.Add(peer.Peer{Name: "alice", Target: bobTarget})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

// TestRemoveMissing ensures Remove fails with a clear message when
// the peer isn't present.
func TestRemoveMissing(t *testing.T) {
	mgr, err := peer.Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	err = mgr.Remove("ghost")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// TestValidatePeerRejects covers the validation cases peers.toml is
// expected to reject so a malformed entry can't hide silently.
func TestValidatePeerRejects(t *testing.T) {
	cases := []struct {
		name string
		p    peer.Peer
		want string
	}{
		{"empty name", peer.Peer{Target: aliceTarget}, "name is empty"},
		{"empty target", peer.Peer{Name: "alice"}, "target is empty"},
		{"target no colon", peer.Peer{Name: "n", Target: "host"}, "must be"},
		{"target whitespace", peer.Peer{Name: "n", Target: "host: /p"}, "forbidden"},
		{"target shell meta", peer.Peer{Name: "n", Target: "host:/p;rm"}, "forbidden"},
		{"target leading dash", peer.Peer{Name: "n", Target: "-flag:host:/p"}, "must not start with '-'"},
		{"name with quote", peer.Peer{Name: `a"b`, Target: aliceTarget}, "illegal"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := peer.ValidatePeer(c.p)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ValidatePeer(%+v) = %v; want contains %q", c.p, err, c.want)
			}
		})
	}
}

// TestParseFromDisk covers the parser's tolerance for comments,
// blank lines, and surrounding whitespace.
func TestParseFromDisk(t *testing.T) {
	root := t.TempDir()
	body := `# leading comment
# another comment

[[peers]]
name = "alice-laptop"
target = "` + aliceTarget + `" # trailing comment

  # indented comment

[[peers]]
name = "bob-laptop"
target = "` + bobTarget + `"
`
	path := filepath.Join(root, peer.PeersFileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write peers.toml: %v", err)
	}
	mgr, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := mgr.List()
	if len(got) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(got))
	}
	if got[0].Name != "alice-laptop" {
		t.Fatalf("peer 0 name: %q", got[0].Name)
	}
}

// TestParseRejectsBad covers the parser's failure paths: unknown
// keys, unknown sections, unbalanced quotes, missing fields.
func TestParseRejectsBad(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"unknown section",
			"[[unknown]]\nname = \"x\"\n",
			"unknown section",
		},
		{
			"unknown key",
			"[[peers]]\nfoo = \"x\"\n",
			"unknown",
		},
		{
			"key outside section",
			"name = \"x\"\n",
			"outside",
		},
		{
			"bare value",
			"[[peers]]\nname = bare\n",
			"double-quoted",
		},
		{
			"missing target",
			"[[peers]]\nname = \"alice\"\n",
			"target is empty",
		},
		{
			"missing name",
			"[[peers]]\ntarget = \"host:/p\"\n",
			"name is empty",
		},
		{
			"duplicate name",
			"[[peers]]\nname = \"alice\"\ntarget = \"" + aliceTarget +
				"\"\n[[peers]]\nname = \"alice\"\ntarget = \"" + bobTarget + "\"\n",
			"duplicate",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, peer.PeersFileName)
			if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			_, err := peer.Load(root)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected %q, got %v", c.want, err)
			}
		})
	}
}

// TestSaveFileMode confirms the file is written with 0o640 so it is
// not world-readable.
func TestSaveFileMode(t *testing.T) {
	root := t.TempDir()
	mgr, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := mgr.Add(peer.Peer{Name: "n", Target: aliceTarget}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(peer.PeersPath(root))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("file mode = %o; want 0640", got)
	}
}

// TestFindPresentMissing covers the read-only Find accessor used by
// the CLI's "peer X" name-as-positional commands.
func TestFindPresentMissing(t *testing.T) {
	mgr, err := peer.Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := mgr.Add(peer.Peer{Name: "alice", Target: aliceTarget}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, ok := mgr.Find("alice"); !ok {
		t.Fatalf("Find(alice) reported missing")
	}
	if _, ok := mgr.Find("bob"); ok {
		t.Fatalf("Find(bob) reported present")
	}
}
