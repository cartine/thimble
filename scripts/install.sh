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
# Checksum verification is MANDATORY by default. If checksums.txt cannot be
# downloaded, the asset's checksum line is missing, or the SHA-256 does not
# match, the installer aborts. Set THIMBLE_INSTALL_NO_VERIFY=1 to bypass.

set -eu

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
fi

tar -xzf "$tmp/$asset" -C "$tmp"
install -m 0755 "$tmp/thimble" "$INSTALL_DIR/$BIN_NAME"
echo "installed $BIN_NAME $VERSION to $INSTALL_DIR/$BIN_NAME"
