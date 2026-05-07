package quorum

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// CanonicalPrefix is the protocol tag prepended to every signed
// message. It namespaces our signatures so a ciphertext produced for
// some other purpose cannot be replayed as a recipient-add signature.
const CanonicalPrefix = "thimble-recipient-add"

// NonceBytes is the random nonce length in bytes; the nonce is
// included in canonical messages so two adds for the same target
// produce structurally distinct signatures even at the same bundle
// SHA. The nonce is generated at prepare time and recorded in
// meta.json so verifiers re-derive the same canonical message.
const NonceBytes = 16

// CanonicalMessage returns the bytes that operators sign over. The
// format is intentionally a single-line ASCII string with no
// whitespace beyond a colon separator, so byte-equality is the only
// comparison verifiers ever do. Replay resistance comes from
// bundleSHA (changes after every successful add) and nonce (random
// per prepare).
func CanonicalMessage(app, env, newRecipient, bundleSHA, nonceHex string) string {
	return strings.Join([]string{
		CanonicalPrefix,
		app,
		env,
		newRecipient,
		bundleSHA,
		nonceHex,
	}, ":")
}

// NewNonce returns NonceBytes of cryptographically random data,
// encoded as lowercase hex. Used by PrepareAdd; callers persist the
// hex string in meta.json so SignAdd / Verify can reconstruct the
// canonical message.
func NewNonce() (string, error) {
	buf := make([]byte, NonceBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("nonce read: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// CheckCanonical verifies that got equals the canonical message
// derived from the provided fields. Returns a precise error when it
// does not so verifier output points operators at the right diff.
func CheckCanonical(got, app, env, newRecipient, bundleSHA, nonceHex string) error {
	want := CanonicalMessage(app, env, newRecipient, bundleSHA, nonceHex)
	if strings.TrimRight(got, "\r\n") != want {
		return errors.New("canonical message mismatch (forged or stale signature)")
	}
	return nil
}
