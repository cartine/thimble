package web

import "testing"

func TestRetrievalCommandUsesRunningExecutable(t *testing.T) {
	active := StoreInfo{Name: "personal", Path: "/tmp/managed stores/personal"}
	got := retrievalCommand(
		"/opt/Thimble Preview/thimble", active, "/tmp/key file", "personal", "main", "TOKEN",
	)
	want := "'/opt/Thimble Preview/thimble' --store '/tmp/managed stores/personal' " +
		"--identity '/tmp/key file' get personal main TOKEN"
	if got != want {
		t.Fatalf("retrieval command = %q, want %q", got, want)
	}
}

func TestRetrievalCommandQuotesExplicitStore(t *testing.T) {
	active := StoreInfo{Name: "-", Path: "/tmp/team's secrets"}
	got := retrievalCommand("thimble", active, "", "api", "prod", "TOKEN")
	want := "thimble --store '/tmp/team'\"'\"'s secrets' get api prod TOKEN"
	if got != want {
		t.Fatalf("retrieval command = %q, want %q", got, want)
	}
}
