package quorum

import "context"

// AgeTool is the narrow surface of internal/age.Tool that the
// quorum protocol needs. We accept an interface here (a) to keep
// the package easy to fake-test with a ROT13 stub and (b) so the
// quorum package does not import internal/age directly — internal/
// store is the integration point and passes its own *age.Tool in.
type AgeTool interface {
	// Encrypt returns ASCII-armored age ciphertext addressed to
	// recipients, wrapping plain.
	Encrypt(ctx context.Context, recipients []string, plain string) ([]byte, error)
	// Decrypt reads the age ciphertext at path and returns the
	// plaintext. The configured identity (if any) is supplied by the
	// implementation.
	Decrypt(ctx context.Context, path string) (string, error)
}
