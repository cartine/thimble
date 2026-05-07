#!/bin/sh
# Verify that the binaries published for a Thimble release tag rebuild
# byte-for-byte from the same tag's source tree.
#
# Reproducibility relies on:
# - the same Go toolchain version (matched by go-version-file in CI),
# - the `-trimpath -ldflags="-s -w"` build flags,
# - stable third-party deps (no replace blocks at non-pinned commits),
# - matched -X ldflag values for version/commit/buildDate (computed below
#   exactly the way release.yml does).
#
# Usage:
#   scripts/verify-release.sh vX.Y.Z
#
# Optional env vars:
#   THIMBLE_REPO=cartine/thimble        # source repo (default).
#   VERIFY_KEEP=1                       # keep the temp worktree.
#   VERIFY_GOOS=linux                   # restrict to one platform.
#   VERIFY_GOARCH=amd64                 # restrict to one arch.

set -eu

VERSION="${1:-}"
if [ -z "$VERSION" ]; then
  echo "usage: $0 vX.Y.Z" >&2
  exit 2
fi

REPO="${THIMBLE_REPO:-cartine/thimble}"
KEEP="${VERIFY_KEEP:-}"

# Refuse to run if the tag doesn't exist locally — reproducibility is per
# release; we have nothing to compare against without a tag.
if ! git rev-parse --verify "refs/tags/$VERSION" >/dev/null 2>&1; then
  echo "tag $VERSION not present locally." >&2
  echo "fetch tags first: git fetch --tags" >&2
  exit 1
fi

work="$(mktemp -d -t thimble-verify-XXXXXX)"
trap '[ -n "$KEEP" ] || rm -rf "$work"' EXIT

git worktree add --quiet --detach "$work/src" "refs/tags/$VERSION"
trap 'git worktree remove --force "$work/src" >/dev/null 2>&1 || true; [ -n "$KEEP" ] || rm -rf "$work"' EXIT

# Resolve the version and commit values the workflow embeds, exactly the
# way the workflow computes them.
version_no_v="${VERSION#v}"
commit_short="$(git -C "$work/src" rev-parse --short=7 HEAD)"

# Build matrix: same as release.yml.
matrix='linux/amd64 linux/arm64 darwin/amd64 darwin/arm64'
if [ -n "${VERIFY_GOOS:-}" ] && [ -n "${VERIFY_GOARCH:-}" ]; then
  matrix="$VERIFY_GOOS/$VERIFY_GOARCH"
fi

# The workflow embeds the build date. We cannot reproduce it exactly
# without knowing the original timestamp, so we accept it as published in
# the release notes via env (THIMBLE_BUILD_DATE=2026-04-01T12:00:00Z) or
# fall back to "unknown" — in which case the binary differs from the
# upstream artifact by only the buildDate string. That's still useful for
# a smoke check; the SHA-256 will not match, which the user will see.
BUILD_DATE="${THIMBLE_BUILD_DATE:-unknown}"

dist="$work/dist"
mkdir -p "$dist"

for slash_pair in $matrix; do
  goos="${slash_pair%/*}"
  goarch="${slash_pair#*/}"
  name="thimble_${version_no_v}_${goos}_${goarch}"

  echo "[verify] building $name (commit=$commit_short build_date=$BUILD_DATE)"

  ldflags="-s -w"
  ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.version=$version_no_v"
  ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.commit=$commit_short"
  ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.buildDate=$BUILD_DATE"

  out="$dist/$name"
  mkdir -p "$out"
  (
    cd "$work/src"
    GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
      go build -trimpath -ldflags="$ldflags" -o "$out/thimble" ./cmd/thimble
    cp README.md SECURITY.md "$out/"
  )
  tar -C "$out" -czf "$dist/$name.tar.gz" .
done

# Fetch the published checksums.txt for this tag from the release.
checksums="$dist/checksums.txt.upstream"
url="https://github.com/$REPO/releases/download/$VERSION/checksums.txt"
if ! curl -fsSL -o "$checksums" "$url"; then
  echo "[verify] could not fetch $url" >&2
  exit 1
fi

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

mismatches=0
for f in "$dist"/*.tar.gz; do
  base="$(basename "$f")"
  actual="$(sha256_of "$f")"
  expected="$(grep "  $base\$" "$checksums" | awk '{print $1}' || true)"
  if [ -z "$expected" ]; then
    printf '[verify] %-50s NO UPSTREAM CHECKSUM\n' "$base"
    mismatches=$((mismatches + 1))
    continue
  fi
  if [ "$actual" = "$expected" ]; then
    printf '[verify] %-50s OK\n' "$base"
  else
    printf '[verify] %-50s MISMATCH\n' "$base"
    printf '          local:  %s\n' "$actual"
    printf '          remote: %s\n' "$expected"
    mismatches=$((mismatches + 1))
  fi
done

if [ "$mismatches" -gt 0 ]; then
  echo "[verify] $mismatches artifact(s) differ from the published release." >&2
  echo "         note: a buildDate mismatch alone produces a SHA mismatch."
  echo "         set THIMBLE_BUILD_DATE to the value from the release notes" >&2
  echo "         to fix the buildDate-only divergence." >&2
  exit 1
fi

echo "[verify] all artifacts reproduce the published release."
