#!/bin/sh
# Thimble installer.
#
# Usage:
#   curl -fsSL https://github.com/cartine/thimble/releases/latest/download/install.sh | sh
#
# Environment variables:
#   THIMBLE_REPO=cartine/thimble    # source repo (default).
#   THIMBLE_VERSION=vX.Y.Z|latest   # release tag to install (default: latest).
#   THIMBLE_INSTALL_DIR=$HOME/.local/bin
#   THIMBLE_BIN_NAME=thimble
#   THIMBLE_INSTALL_NO_VERIFY=1     # skip checksum verification.
#                                   # DANGEROUS — only for emergency reinstalls
#                                   # when checksums.txt is unreachable.
#
# Verification: SHA-256 against checksums.txt. Mandatory by default.
# If you want SLSA build provenance on top of the checksum, run
# `gh attestation verify <asset> --repo cartine/thimble` against
# the downloaded tarball — that requires `gh auth login` and is left
# to security-conscious operators rather than bundled into the
# default install path.

set -eu

REPO="${THIMBLE_REPO:-cartine/thimble}"
VERSION="${THIMBLE_VERSION:-latest}"
INSTALL_DIR="${THIMBLE_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="${THIMBLE_BIN_NAME:-thimble}"
NO_VERIFY="${THIMBLE_INSTALL_NO_VERIFY:-}"

# Print this script's own SHA-256 first so an operator piping it through
# `sh` can spot-check that the script they ran matches what is published
# in the release. macOS ships `shasum -a 256` rather than `sha256sum`.
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
# When invoked via `curl … | sh`, $0 is typically the shell name itself
# (`sh` or `/bin/sh`), and the script body lives only on the inherited
# stdin pipe — there is no file we can hash. Skip the self-hash print
# in that case.
_self_base="$(basename -- "$0" 2>/dev/null || echo)"
case "$_self_base" in
  sh|-sh|bash|-bash|zsh|dash|""|*"$0"*) : ;;
  *)
    if [ -r "$0" ]; then
      _self="$(sha256_of "$0" || true)"
      [ -n "$_self" ] && echo "thimble installer SHA-256: $_self" >&2
      unset _self
    fi
    ;;
esac
unset _self_base

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  # Resolve latest tag via the redirect on /releases/latest (no API quota).
  redirect="$(curl -fsSI "https://github.com/$REPO/releases/latest" \
    | tr -d '\r' \
    | awk 'tolower($1)=="location:" {print $2}' | head -n 1)"
  VERSION="${redirect##*/}"
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

if [ -n "$NO_VERIFY" ]; then
  echo "" >&2
  echo "============================================================" >&2
  echo "WARNING: THIMBLE_INSTALL_NO_VERIFY=1 is set." >&2
  echo "Checksum verification is DISABLED. A network attacker or a" >&2
  echo "compromised CDN could substitute a malicious binary and this" >&2
  echo "installer would not detect it. Re-run without the flag as" >&2
  echo "soon as possible." >&2
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
  expected="$(awk -v name="$asset" '$2==name {print $1}' "$tmp/checksums.txt")"
  if [ -z "$expected" ]; then
    echo "no checksum entry for $asset in checksums.txt" >&2
    exit 1
  fi
  actual="$(sha256_of "$tmp/$asset")"
  if [ "$expected" != "$actual" ]; then
    echo "checksum mismatch for $asset" >&2
    echo "  expected: $expected" >&2
    echo "  actual:   $actual" >&2
    exit 1
  fi
fi

tar -xzf "$tmp/$asset" -C "$tmp"
install -m 0755 "$tmp/thimble" "$INSTALL_DIR/$BIN_NAME"
echo "installed $BIN_NAME $VERSION to $INSTALL_DIR/$BIN_NAME"
