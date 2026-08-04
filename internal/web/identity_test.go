//go:build !windows

package web

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityReadyRequiresExplicitRegularFile(t *testing.T) {
	if identityReady("", true) {
		t.Fatal("empty identity was accepted with unsafe mode enabled")
	}
	path := filepath.Join(t.TempDir(), "identity")
	if err := os.WriteFile(path, []byte("test identity"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !identityReady(path, false) {
		t.Fatal("private identity file was rejected")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if identityReady(path, false) {
		t.Fatal("world-readable identity was accepted")
	}
	if !identityReady(path, true) {
		t.Fatal("unsafe override did not accept readable identity")
	}
}
