package store

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// fileSHA256 returns the lowercase hex-encoded SHA-256 of the file at
// path. K-22 uses this to bind a manifest entry to the on-disk
// ciphertext: encryptAndWrite computes it after a successful rename
// and decrypt verifies it before shelling out to age.
func fileSHA256(path string) (string, error) {
	// #nosec G304 -- path is a manifest-controlled bundle file inside
	// the store root; it is not user-supplied at this layer.
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

// bytesSHA256 returns the lowercase hex-encoded SHA-256 of b.
func bytesSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
