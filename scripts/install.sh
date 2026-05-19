#!/bin/sh
# Thimble installer.
#
# Usage:
#   curl -fsSL <url>/install.sh | sh
#
# Environment variables:
#   THIMBLE_REPO=cartine/thimble    # source repo (default).
#   THIMBLE_VERSION=vX.Y.Z|latest   # release tag to install (default: latest).
#   THIMBLE_INSTALL_DIR=$HOME/.local/bin
#   THIMBLE_BIN_NAME=thimble
#
#   THIMBLE_INSTALL_NO_VERIFY=1     # skip checksum verification.
#                                   # DANGEROUS — only for emergency reinstalls
#                                   # when checksums.txt is unreachable. The
#                                   # script prints a multi-line warning and
#                                   # waits before proceeding.
#
# Verification levels (best to worst):
#   1. `gh attestation verify --bundle attestations.intoto.jsonl --repo …` —
#      full SLSA build provenance using the bundle shipped with the release.
#      Works offline; does NOT require `gh auth login`. `--repo` here is a
#      cert-identity constraint, not an API target — no auth call is made.
#   2. `gh attestation verify --repo …` — same check, but fetches the
#      attestation from GitHub. Requires `gh auth login`; we fall back to
#      this path only when the bundle asset isn't available (older releases).
#   3. `cosign verify-blob` if a `*.bundle` is uploaded for the asset and
#      `cosign` is on PATH (currently a no-op since K-40 ships with attestation
#      only — left as a forward hook).
#   4. SHA-256 against checksums.txt — mandatory baseline. Confirms the asset
#      matches what the release publisher hashed.
#
# If the SLSA verify is unavailable (no `gh`/`cosign`, no auth, no bundle on
# the release) the installer prints a `note:` and continues — the checksum
# already passed, so it would be misleading to emit a warning that suggests
# the asset is suspect.
#
# Checksum verification is MANDATORY by default. If checksums.txt cannot be
# downloaded, the asset's checksum line is missing, or the SHA-256 does not
# match, the installer aborts. Set THIMBLE_INSTALL_NO_VERIFY=1 to bypass.

set -eu

# Print this script's own SHA-256 first so an operator piping it through `sh`
# can spot-check that the script they ran matches what is published in the
# release. We compute at runtime to keep the build pipeline simple; macOS
# ships `shasum -a 256` rather than `sha256sum`, so try both.
self_sha256() {
  if [ ! -r "$1" ]; then
    return 0
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$1" | awk '{print $2}'
  fi
}
# When invoked via `curl … | sh`, $0 is typically the shell name itself
# (`sh` or `/bin/sh`), and the script body lives only on the inherited
# stdin pipe — there is no file we can hash. Detect by basename.
_self_base="$(basename -- "$0" 2>/dev/null || echo)"
case "$_self_base" in
  sh|-sh|bash|-bash|zsh|dash|""|*"$0"*)
    : # piped or unknown invocation, skip the self-hash print.
    ;;
  *)
    if [ -r "$0" ]; then
      _self="$(self_sha256 "$0" || true)"
      if [ -n "$_self" ]; then
        echo "thimble installer SHA-256: $_self" >&2
      fi
      unset _self
    fi
    ;;
esac
unset _self_base

REPO="${THIMBLE_REPO:-cartine/thimble}"
VERSION="${THIMBLE_VERSION:-latest}"
INSTALL_DIR="${THIMBLE_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="${THIMBLE_BIN_NAME:-thimble}"
NO_VERIFY="${THIMBLE_INSTALL_NO_VERIFY:-}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  url="https://api.github.com/repos/$REPO/releases/latest"
  VERSION="$(curl -fsSL "$url" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
fi

if [ -z "$VERSION" ]; then
  echo "could not resolve release version" >&2
  exit 1
fi

base="https://github.com/$REPO/releases/download/$VERSION"
asset="thimble_${VERSION#v}_${os}_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$INSTALL_DIR"
curl -fsSL "$base/$asset" -o "$tmp/$asset"

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$1" | awk '{print $2}'
  else
    echo "no sha256 tool available (need sha256sum, shasum, or openssl)" >&2
    return 1
  fi
}

if [ -n "$NO_VERIFY" ]; then
  echo "" >&2
  echo "============================================================" >&2
  echo "WARNING: THIMBLE_INSTALL_NO_VERIFY=1 is set." >&2
  echo "" >&2
  echo "Checksum verification is DISABLED. The downloaded asset will" >&2
  echo "be installed without integrity checking. A network attacker" >&2
  echo "or a compromised CDN could substitute a malicious binary and" >&2
  echo "this installer would not detect it." >&2
  echo "" >&2
  echo "This flag exists only for emergency reinstalls when the" >&2
  echo "official checksums.txt is unreachable. Re-run without the" >&2
  echo "flag as soon as possible." >&2
  echo "============================================================" >&2
  echo "" >&2
  sleep 3
else
  if ! curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"; then
    echo "failed to download checksums.txt from $base/checksums.txt" >&2
    echo "checksum verification is mandatory; aborting." >&2
    echo "set THIMBLE_INSTALL_NO_VERIFY=1 to bypass (not recommended)." >&2
    exit 1
  fi
  expected="$(grep "  $asset\$" "$tmp/checksums.txt" | awk '{print $1}')"
  if [ -z "$expected" ]; then
    echo "no checksum entry for $asset in checksums.txt" >&2
    echo "the release may be incomplete; aborting." >&2
    exit 1
  fi
  actual="$(sha256_of "$tmp/$asset")"
  if [ "$expected" != "$actual" ]; then
    echo "checksum mismatch for $asset" >&2
    echo "  expected: $expected" >&2
    echo "  actual:   $actual" >&2
    exit 1
  fi

  # Layered provenance. Checksum was the floor; this is additive.
  # Try to fetch the sigstore bundle that the release workflow uploads
  # alongside the tarballs. Present from v0.1.1 onward; absent on
  # v0.1.0 (where verification falls back to the auth'd remote path).
  attest_bundle=""
  if curl -fsSL -o "$tmp/attestations.intoto.jsonl" \
       "$base/attestations.intoto.jsonl" 2>/dev/null; then
    attest_bundle="$tmp/attestations.intoto.jsonl"
  fi

  # `gh attestation verify` requires `--owner` or `--repo` even when
  # `--bundle` is supplied (the flag's role with `--bundle` is to
  # constrain the certificate identity; no API call is made for it,
  # so no `gh auth login` is needed). Omitting it makes the command
  # exit non-zero with the misleading error
  #   "at least one of the flags in the group [owner repo] is required"
  # which is what was happening in v0.1.1.
  provenance_ok="no"
  if command -v gh >/dev/null 2>&1; then
    if [ -n "$attest_bundle" ] && \
       gh attestation verify "$tmp/$asset" \
         --bundle "$attest_bundle" --repo "$REPO" >/dev/null 2>&1; then
      echo "verified build provenance for $asset (sigstore bundle)"
      provenance_ok="yes"
    elif gh auth status >/dev/null 2>&1 && \
         gh attestation verify "$tmp/$asset" --repo "$REPO" >/dev/null 2>&1; then
      echo "verified build provenance for $asset (gh attestation, remote)"
      provenance_ok="yes"
    fi
  fi

  if [ "$provenance_ok" = "no" ]; then
    # Why this is a note, not a warning: the SHA-256 check above already
    # tied the bytes on disk to checksums.txt — that's the floor and it
    # passed. The provenance step adds "and the build came from this
    # workflow", which is desirable but secondary. Treat a missing tool,
    # missing bundle, or unauthenticated gh as a soft skip; emit a real
    # warning only if a configured verifier ran and *rejected* the asset.
    if ! command -v gh >/dev/null 2>&1; then
      echo "note: install \`gh\` (https://cli.github.com) for full SLSA"
      echo "      build-provenance verification."
    elif [ -z "$attest_bundle" ] && ! gh auth status >/dev/null 2>&1; then
      echo "note: gh is not authenticated and this release ships no"
      echo "      sigstore bundle — skipping SLSA provenance check."
      echo "      run \`gh auth login\` once and re-install for full provenance."
    else
      echo "warning: gh attestation verify failed for $asset" >&2
      echo "checksum is OK but provenance could not be confirmed." >&2
      echo "if you trust the checksum source, this may be acceptable;" >&2
      echo "otherwise abort and re-run with a known-good network." >&2
    fi
  fi
fi

tar -xzf "$tmp/$asset" -C "$tmp"
install -m 0755 "$tmp/thimble" "$INSTALL_DIR/$BIN_NAME"
echo "installed $BIN_NAME $VERSION to $INSTALL_DIR/$BIN_NAME"
