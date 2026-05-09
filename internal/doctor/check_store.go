package doctor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cartine/thimble/internal/store"
)

// checkStoreDir verifies the secrets store root: it exists, is mode
// 0700 (no group/world bits), and is writable.
func checkStoreDir(st *store.Store) CheckResult {
	r := CheckResult{Name: "store"}
	root := st.Root()
	info, err := os.Stat(root)
	if err != nil {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf("store dir %s: %v", root, err)
		return r
	}
	if !info.IsDir() {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf("store path %s is not a directory", root)
		return r
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf(
			"store dir %s mode 0%o; expected 0700 (run `chmod 0700`)",
			root, mode,
		)
		return r
	}
	if err := writeProbe(root); err != nil {
		r.Status = StatusFail
		r.Detail = fmt.Sprintf("store dir %s not writable: %v", root, err)
		return r
	}
	r.Status = StatusOK
	r.Detail = fmt.Sprintf("path=%s; mode=0%o", root, mode)
	return r
}

// writeProbe creates and removes a temp file inside dir to confirm
// write permission without leaving a footprint.
func writeProbe(dir string) error {
	f, err := os.CreateTemp(dir, ".doctor-probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(filepath.Clean(name))
}
