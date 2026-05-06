#!/bin/sh
set -eu

REPO="${THIMBLE_REPO:-cartine/thimble}"
VERSION="${THIMBLE_VERSION:-latest}"
INSTALL_DIR="${THIMBLE_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="${THIMBLE_BIN_NAME:-thimble}"

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
if curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"; then
  expected="$(grep "  $asset\$" "$tmp/checksums.txt" | awk '{print $1}')"
  actual="$(openssl dgst -sha256 "$tmp/$asset" | awk '{print $2}')"
  if [ "$expected" != "$actual" ]; then
    echo "checksum mismatch for $asset" >&2
    exit 1
  fi
fi

tar -xzf "$tmp/$asset" -C "$tmp"
install -m 0755 "$tmp/thimble" "$INSTALL_DIR/$BIN_NAME"
echo "installed $BIN_NAME $VERSION to $INSTALL_DIR/$BIN_NAME"
