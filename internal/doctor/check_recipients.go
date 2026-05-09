package doctor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/store"
)

// checkRecipientsList walks every namespace in the manifest and
// returns one informational CheckResult per namespace. Each result
// is always StatusOK — the recipient list is informational, not
// pass/fail material. Each result names the namespace, the count of
// recipients, and a brief summary of types and thumbprints.
func checkRecipientsList(st *store.Store) []CheckResult {
	m, err := st.LoadManifest()
	if err != nil {
		return []CheckResult{{
			Name:   "recipients",
			Status: StatusFail,
			Detail: fmt.Sprintf("load manifest: %v", err),
		}}
	}
	keys := manifestKeys(m)
	if len(keys) == 0 {
		return []CheckResult{{
			Name:   "recipients",
			Status: StatusOK,
			Detail: "no namespaces initialized",
		}}
	}
	out := make([]CheckResult, 0, len(keys))
	for _, k := range keys {
		envMeta := m.Apps[k.app].Environments[k.env]
		out = append(out, recipientResult(k.app, k.env, envMeta.Recipients))
	}
	return out
}

// nsKey is a (app, env) key sorted alphabetically.
type nsKey struct{ app, env string }

func manifestKeys(m store.Manifest) []nsKey {
	var keys []nsKey
	for app, appMeta := range m.Apps {
		for env := range appMeta.Environments {
			keys = append(keys, nsKey{app: app, env: env})
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].app == keys[j].app {
			return keys[i].env < keys[j].env
		}
		return keys[i].app < keys[j].app
	})
	return keys
}

func recipientResult(app, env string, recipients []string) CheckResult {
	var summary []string
	for _, r := range recipients {
		summary = append(summary, fmt.Sprintf(
			"%s(%s)", recipientPrefix(r), audit.Thumbprint(r),
		))
	}
	return CheckResult{
		Name:   fmt.Sprintf("recipients[%s/%s]", app, env),
		Status: StatusOK,
		Detail: fmt.Sprintf(
			"count=%d; %s", len(recipients), strings.Join(summary, ", "),
		),
	}
}

// recipientPrefix returns "age1", "ssh-ed25519", "ssh-rsa", or
// "unknown" depending on the recipient's wire prefix.
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
