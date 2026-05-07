package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VerifyReport is the K-22 attestation summary for a single
// (app, env) namespace: the on-disk bundle's stat info, the manifest's
// stored SHA, the recomputed SHA, and the recipient list.
type VerifyReport struct {
	App           string
	Env           string
	BundlePath    string
	BundleSize    int64
	BundleMode    os.FileMode
	ManifestSHA   string
	ActualSHA     string
	Match         bool
	Recipients    []RecipientView
	StoredVersion uint64
}

// RecipientView is one recipient entry in a verify report: its prefix
// (one of "age1", "ssh-ed25519", "ssh-rsa") and the raw value as stored
// in the manifest. The full value is shown so an operator can confirm
// the on-disk recipient against an out-of-band copy.
type RecipientView struct {
	Prefix string
	Value  string
}

// Verify recomputes the bundle's SHA-256, compares to the manifest's
// stored value, and returns a typed report. On failure to load the
// manifest entry or stat the bundle, an error is returned. A SHA
// mismatch is NOT an error — it is reported via Match=false in the
// returned report so the CLI can format it.
func (s *Store) Verify(app, env string) (VerifyReport, error) {
	if err := ValidateName("app", app); err != nil {
		return VerifyReport{}, err
	}
	if err := ValidateName("environment", env); err != nil {
		return VerifyReport{}, err
	}
	meta, err := s.Find(app, env)
	if err != nil {
		return VerifyReport{}, err
	}
	bundlePath := filepath.Join(s.root, filepath.FromSlash(meta.File))
	info, err := os.Stat(bundlePath)
	if err != nil {
		return VerifyReport{}, fmt.Errorf("stat bundle %s: %w", bundlePath, err)
	}
	actual, err := fileSHA256(bundlePath)
	if err != nil {
		return VerifyReport{}, fmt.Errorf("hash bundle %s: %w", bundlePath, err)
	}
	report := VerifyReport{
		App:           app,
		Env:           env,
		BundlePath:    bundlePath,
		BundleSize:    info.Size(),
		BundleMode:    info.Mode().Perm(),
		ManifestSHA:   meta.BundleSHA256,
		ActualSHA:     actual,
		Match:         meta.BundleSHA256 != "" && meta.BundleSHA256 == actual,
		Recipients:    recipientViews(meta.Recipients),
		StoredVersion: meta.Version,
	}
	if meta.BundleSHA256 == "" {
		// An empty stored SHA can't match. Report mismatch so the
		// CLI surfaces it as "manifest=<empty>".
		report.Match = false
	}
	return report, nil
}

// recipientViews maps raw recipient strings to typed RecipientView
// entries with the prefix labelled. Unknown prefixes are labelled
// "unknown"; CleanRecipient already gates this list, so that should
// never fire in practice.
func recipientViews(recipients []string) []RecipientView {
	out := make([]RecipientView, 0, len(recipients))
	for _, r := range recipients {
		out = append(out, RecipientView{Prefix: recipientPrefix(r), Value: r})
	}
	return out
}

func recipientPrefix(r string) string {
	switch {
	case strings.HasPrefix(r, "age1"):
		return "age1"
	case strings.HasPrefix(r, "ssh-ed25519 "):
		return "ssh-ed25519"
	case strings.HasPrefix(r, "ssh-rsa "):
		return "ssh-rsa"
	default:
		return "unknown"
	}
}

// ErrSHAEmpty is returned by callers that need to distinguish an
// empty manifest SHA (older format) from a real mismatch.
var ErrSHAEmpty = errors.New("manifest BundleSHA256 is empty")
