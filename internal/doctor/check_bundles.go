package doctor

import (
	"fmt"
	"strings"

	"github.com/cartine/thimble/internal/store"
)

// checkBundles loads each namespace's manifest entry, recomputes
// sha256(ciphertext), and compares to the stored BundleSHA256
// (K-22). Any per-namespace mismatch produces an aggregate fail
// result naming each offender.
func checkBundles(st *store.Store) CheckResult {
	r := CheckResult{Name: "bundles"}
	m, err := st.LoadManifest()
	if err != nil {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf("load manifest: %v", err)
		return r
	}
	failures, ok := bundleVerdicts(st, m)
	if len(failures) > 0 {
		r.Status = StatusFail
		r.Detail = strings.Join(failures, "; ")
		return r
	}
	r.Status = StatusOK
	r.Detail = fmt.Sprintf("verified=%d", ok)
	return r
}

// bundleVerdicts walks every namespace and returns (failures, ok-count).
// A verify error or false Match is a failure; matched bundles count
// toward ok. Older manifests with no SHA produce a warn-style note
// but still count toward ok (consistent with Render's read path).
func bundleVerdicts(st *store.Store, m store.Manifest) ([]string, int) {
	var failures []string
	ok := 0
	for app, appMeta := range m.Apps {
		for env := range appMeta.Environments {
			report, err := st.Verify(app, env)
			if err != nil {
				failures = append(failures, fmt.Sprintf(
					"%s/%s: %v", app, env, err,
				))
				continue
			}
			if report.ManifestSHA == "" {
				ok++
				continue
			}
			if !report.Match {
				failures = append(failures, fmt.Sprintf(
					"%s/%s: SHA-256 MISMATCH", app, env,
				))
				continue
			}
			ok++
		}
	}
	return failures, ok
}
