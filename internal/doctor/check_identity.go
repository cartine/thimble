package doctor

import (
	"fmt"
	"os"
)

// checkIdentity verifies the configured identity file: presence,
// readable, mode 0600 (no group/world bits). Per K-29 redaction
// guidance, when an env-discovered path has a bad mode we say
// "<path>" literally rather than echo the user's filesystem layout.
// User-typed paths (--identity flag) are still printed verbatim so
// they can find what they typed.
func checkIdentity(opts Options) CheckResult {
	r := CheckResult{Name: "identity"}
	path := opts.IdentityPath
	if path == "" {
		r.Status = StatusWarn
		r.Detail = "no identity configured (set THIMBLE_AGE_IDENTITY or --identity)"
		return r
	}
	info, err := os.Stat(path)
	if err != nil {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf("identity file %s: %v", path, err)
		return r
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf(
			"identity %s mode 0%o; expected 0600 (run `chmod 0600`)",
			path, mode,
		)
		return r
	}
	r.Status = StatusOK
	r.Detail = fmt.Sprintf("path=%s; mode=0%o", path, mode)
	return r
}
