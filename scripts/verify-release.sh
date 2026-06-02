#!/bin/sh
# Verify that the binaries published for a Thimble release tag rebuild
# byte-for-byte from the same tag's source tree.
#
# The published tarballs are checksummed release assets, but their gzip
# headers and tar entry metadata are not normalized. This verifier first
# checks each downloaded tarball against the published checksums.txt, then
# extracts the payload and compares the rebuilt binary and bundled docs.
#
# Usage:
#   scripts/verify-release.sh vX.Y.Z
#
# Optional env vars:
#   THIMBLE_REPO=cartine/thimble        # source repo (default).
#   VERIFY_KEEP=1                       # keep the temp clone and downloads.
#   VERIFY_GOOS=linux                   # restrict to one platform.
#   VERIFY_GOARCH=amd64                 # restrict to one arch.
#   THIMBLE_BUILD_DATE=...              # override extracted build date.

set -eu

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 2
fi

REPO="${THIMBLE_REPO:-cartine/thimble}"
KEEP="${VERIFY_KEEP:-}"

if ! git rev-parse --verify "refs/tags/$VERSION" >/dev/null 2>&1; then
  echo "tag $VERSION not present locally." >&2
  echo "fetch tags first: git fetch --tags" >&2
  exit 1
fi

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

checksum_for() {
  grep "  $1\$" "$checksums" | awk '{print $1}' || true
}

validate_archive() {
  tarball="$1"
  list="$2"
  tar -tzf "$tarball" >"$list"
  while IFS= read -r entry; do
    case "$entry" in
      ./ | ./README.md | ./SECURITY.md | ./thimble) ;;
      README.md | SECURITY.md | thimble) ;;
      *)
        echo "[verify] unexpected archive entry: $entry" >&2
        return 1
        ;;
    esac
  done <"$list"
}

extract_build_date() {
  binary="$1"
  dates="$(strings "$binary" | grep -E \
    '^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$' \
    | sort -u)"
  count="$(printf '%s\n' "$dates" | sed '/^$/d' | wc -l | tr -d ' ')"
  if [ "$count" -ne 1 ]; then
    echo "[verify] could not determine unique build date in $binary" >&2
    printf '%s\n' "$dates" >&2
    return 1
  fi
  printf '%s\n' "$dates"
}

build_payload() {
  build_src="$1"
  goos="$2"
  goarch="$3"
  build_date="$4"
  out="$5"
  ldflags="-s -w"
  ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.version=$version_no_v"
  ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.commit=$commit_short"
  ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.buildDate=$build_date"
  mkdir -p "$out"
  (
    cd "$build_src"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
      go build -trimpath -ldflags="$ldflags" -o "$out/thimble" ./cmd/thimble
    cp README.md SECURITY.md "$out/"
  )
}

module_version() {
  go version -m "$1" | awk '$1 == "mod" { print $3; exit }'
}

source_for_module() {
  binary="$1"
  module="$(module_version "$binary")"
  case "$module" in
    "$VERSION")
      printf '%s\n' "$src_tagged"
      ;;
    "(devel)")
      printf '%s\n' "$src_untagged"
      ;;
    *)
      echo "[verify] unexpected module version in $binary: ${module:-<empty>}" >&2
      return 1
      ;;
  esac
}

verify_file() {
  name="$1"
  actual_path="$2"
  expected_path="$3"
  if cmp -s "$actual_path" "$expected_path"; then
    return 0
  fi
  echo "[verify] $name differs" >&2
  printf '          rebuilt:  %s\n' "$(sha256_of "$actual_path")" >&2
  printf '          upstream: %s\n' "$(sha256_of "$expected_path")" >&2
  return 1
}

work="$(mktemp -d -t thimble-verify-XXXXXX)"
trap '[ -n "$KEEP" ] || rm -rf "$work"' EXIT

repo_root="$(git rev-parse --show-toplevel)"
src_tagged="$work/src-tagged"
src_untagged="$work/src-untagged"
git clone --quiet "$repo_root" "$src_tagged"
git -C "$src_tagged" -c advice.detachedHead=false checkout --quiet "$VERSION"
git clone --quiet "$repo_root" "$src_untagged"
git -C "$src_untagged" -c advice.detachedHead=false \
  checkout --quiet "$(git -C "$src_tagged" rev-parse HEAD)"
git -C "$src_untagged" tag -d "$VERSION" >/dev/null 2>&1 || true

version_no_v="${VERSION#v}"
commit_short="$(git -C "$src_tagged" rev-parse --short=7 HEAD)"

matrix='linux/amd64 linux/arm64 darwin/amd64 darwin/arm64'
if [ -n "${VERIFY_GOOS:-}" ] && [ -n "${VERIFY_GOARCH:-}" ]; then
  matrix="$VERIFY_GOOS/$VERIFY_GOARCH"
fi

dist="$work/dist"
mkdir -p "$dist"

checksums="$dist/checksums.txt.upstream"
url="https://github.com/$REPO/releases/download/$VERSION/checksums.txt"
if ! curl -fsSL -o "$checksums" "$url"; then
  echo "[verify] could not fetch $url" >&2
  exit 1
fi

mismatches=0
for slash_pair in $matrix; do
  goos="${slash_pair%/*}"
  goarch="${slash_pair#*/}"
  name="thimble_${version_no_v}_${goos}_${goarch}"
  asset="$name.tar.gz"
  asset_url="https://github.com/$REPO/releases/download/$VERSION/$asset"
  tarball="$dist/$asset"
  upstream="$dist/$name.upstream"
  rebuilt="$dist/$name.rebuilt"

  if ! curl -fsSL -o "$tarball" "$asset_url"; then
    echo "[verify] could not fetch $asset_url" >&2
    mismatches=$((mismatches + 1))
    continue
  fi

  expected="$(checksum_for "$asset")"
  actual="$(sha256_of "$tarball")"
  if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
    printf '[verify] %-50s CHECKSUM MISMATCH\n' "$asset"
    printf '          local:  %s\n' "$actual"
    printf '          remote: %s\n' "${expected:-<missing>}"
    mismatches=$((mismatches + 1))
    continue
  fi

  mkdir -p "$upstream"
  validate_archive "$tarball" "$dist/$name.list"
  tar -C "$upstream" -xzf "$tarball"

  build_date="${THIMBLE_BUILD_DATE:-}"
  if [ -z "$build_date" ]; then
    build_date="$(extract_build_date "$upstream/thimble")"
  fi
  build_src="$(source_for_module "$upstream/thimble")"

  echo "[verify] building $name (commit=$commit_short build_date=$build_date)"
  build_payload "$build_src" "$goos" "$goarch" "$build_date" "$rebuilt"

  ok=1
  verify_file "$asset binary" "$rebuilt/thimble" "$upstream/thimble" || ok=0
  verify_file "$asset README.md" "$rebuilt/README.md" "$upstream/README.md" || ok=0
  verify_file "$asset SECURITY.md" "$rebuilt/SECURITY.md" "$upstream/SECURITY.md" || ok=0

  if [ "$ok" -eq 1 ]; then
    printf '[verify] %-50s OK\n' "$asset"
  else
    printf '[verify] %-50s PAYLOAD MISMATCH\n' "$asset"
    mismatches=$((mismatches + 1))
  fi
done

if [ "$mismatches" -gt 0 ]; then
  echo "[verify] $mismatches artifact(s) differ from the published release." >&2
  exit 1
fi

echo "[verify] all release payloads reproduce the published artifacts."
