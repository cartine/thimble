package doctor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/cartine/thimble/internal/age"
)

// checkAge produces the diagnostic for the configured age binary:
// resolved path, version, SHA-256, and an explicit verdict on the
// $THIMBLE_AGE_SHA256 pin if one is set.
func checkAge(tool *age.Tool, opts Options) CheckResult {
	r := CheckResult{Name: "age"}
	if tool == nil {
		r.Status = StatusFail
		r.Detail = "no age tool configured"
		return r
	}
	binPath := tool.Binary()
	version, vErr := ageVersion(binPath)
	sha, sErr := binarySHA256(binPath)
	r.Detail = formatAgeDetail(binPath, version, sha, vErr, sErr)
	if vErr != nil || sErr != nil {
		r.Status = StatusFail
		return r
	}
	if opts.AgeSHA256Pin != "" {
		if !strings.EqualFold(opts.AgeSHA256Pin, sha) {
			r.Status = StatusFail
			r.Detail += fmt.Sprintf(
				"; THIMBLE_AGE_SHA256=%s does NOT match", opts.AgeSHA256Pin,
			)
			return r
		}
		r.Detail += "; pin matches"
	}
	r.Status = StatusOK
	return r
}

func ageVersion(binPath string) (string, error) {
	if binPath == "" {
		return "", fmt.Errorf("age binary path empty")
	}
	// #nosec G204 -- binPath is the trusted age binary configured at
	// startup (resolved by age.Resolve).
	cmd := exec.Command(binPath, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func binarySHA256(path string) (string, error) {
	// #nosec G304 -- path is the resolved age binary, not user input.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func formatAgeDetail(binPath, version, sha string, vErr, sErr error) string {
	parts := []string{fmt.Sprintf("path=%s", binPath)}
	if vErr != nil {
		parts = append(parts, fmt.Sprintf("version=ERROR: %v", vErr))
	} else if version != "" {
		parts = append(parts, fmt.Sprintf("version=%s", version))
	}
	if sErr != nil {
		parts = append(parts, fmt.Sprintf("sha256=ERROR: %v", sErr))
	} else if sha != "" {
		parts = append(parts, fmt.Sprintf("sha256=%s", sha))
	}
	return strings.Join(parts, "; ")
}
