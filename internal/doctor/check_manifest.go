package doctor

import (
	"fmt"
	"os"
	"strings"

	"github.com/cartine/thimble/internal/store"
)

// checkManifest verifies the on-disk manifest is parseable, every
// envManifest has a positive Version (state isn't half-written), and
// every referenced bundle exists on disk.
func checkManifest(st *store.Store) CheckResult {
	r := CheckResult{Name: "manifest"}
	m, err := st.LoadManifest()
	if err != nil {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf("load manifest: %v", err)
		return r
	}
	missing := manifestIssues(st, m)
	if len(missing) > 0 {
		r.Status = StatusFail
		r.Detail = strings.Join(missing, "; ")
		return r
	}
	count := 0
	for _, app := range m.Apps {
		count += len(app.Environments)
	}
	r.Status = StatusOK
	r.Detail = fmt.Sprintf("manifest_version=%d; namespaces=%d", m.Version, count)
	return r
}

// manifestIssues walks every (app, env) entry and returns a list of
// human-readable issues: missing bundle files, zero Version, or
// empty File. An empty return value means the manifest is healthy.
func manifestIssues(st *store.Store, m store.Manifest) []string {
	var issues []string
	for app, appMeta := range m.Apps {
		for env, envMeta := range appMeta.Environments {
			ns := app + "/" + env
			if envMeta.File == "" {
				issues = append(issues, ns+": missing File")
				continue
			}
			if envMeta.Version == 0 {
				issues = append(issues, ns+": Version=0")
			}
			path := st.BundlePath(envMeta)
			if _, err := os.Stat(path); err != nil {
				issues = append(issues, fmt.Sprintf(
					"%s: bundle %s missing (%v)", ns, envMeta.File, err,
				))
			}
		}
	}
	return issues
}
