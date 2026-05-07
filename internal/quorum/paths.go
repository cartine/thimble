package quorum

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// PendingDirName is the directory inside the store root where
// challenge and signature files live for an in-flight recipient add.
// One directory per add (cleared on success) is sufficient because
// only one add per (app, env) can be staged at a time.
const PendingDirName = ".pending-recipient-adds"

// MetaFileName is the canonical metadata file inside the pending
// directory. It records the new-recipient string, app, env, bundle
// SHA at prepare time, the nonce, and per-operator challenge entries
// with their thumbprints. Verifiers re-read this to reconstruct the
// canonical message.
const MetaFileName = "meta.json"

// challengeSuffix and sigSuffix make the file roles obvious on disk
// so a human listing the directory can tell the two phases apart.
const (
	challengeSuffix = ".challenge"
	sigSuffix       = ".sig"
)

// PendingDir returns the absolute path to the pending directory under
// storeRoot.
func PendingDir(storeRoot string) string {
	return filepath.Join(storeRoot, PendingDirName)
}

// MetaPath returns the absolute path to meta.json inside the pending
// directory.
func MetaPath(storeRoot string) string {
	return filepath.Join(PendingDir(storeRoot), MetaFileName)
}

// ChallengePath returns the absolute path to the per-operator
// challenge file. The file is named by the operator's recipient
// thumbprint so verifiers can pair signatures with operators without
// having to re-parse the age headers.
func ChallengePath(storeRoot, operatorThumb string) string {
	return filepath.Join(
		PendingDir(storeRoot), operatorThumb+challengeSuffix,
	)
}

// SignaturePath returns the absolute path to the per-operator
// signature file. Mirrors ChallengePath naming so the pending
// directory has at most 2 files per operator.
func SignaturePath(storeRoot, operatorThumb string) string {
	return filepath.Join(
		PendingDir(storeRoot), operatorThumb+sigSuffix,
	)
}

// RecipientThumbprint returns the K-27-style 16-hex-char thumbprint
// of recipient. We re-implement it locally instead of importing
// internal/audit to keep the package layering clean (audit imports
// nothing from quorum, and quorum imports nothing from audit; both
// agree on the algorithm).
func RecipientThumbprint(recipient string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(recipient)))
	return hex.EncodeToString(sum[:])[:16]
}
