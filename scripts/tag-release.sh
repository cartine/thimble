#!/usr/bin/env bash
# scripts/tag-release.sh — cut a Thimble release.
#
# Wraps the manual release flow (bump version → CHANGELOG entry → tag →
# push → watch workflow → verify checksums + attestation) into one
# command. The same logic is exposed by `make tag-release VERSION=…`
# and by the `/release` agent skill.
#
# Usage:
#   scripts/tag-release.sh patch
#   scripts/tag-release.sh minor
#   scripts/tag-release.sh major
#   scripts/tag-release.sh v0.2.5
#   scripts/tag-release.sh patch --dry-run
#
# Refuses if:
# - working tree is dirty
# - branch is not `main`
# - CHANGELOG.md has no `[Unreleased]` content (refuse empty release)
#
# Dry-run prints every shell side effect prefixed with `[dry-run]` and
# exits 0 without touching git, GitHub, or the working tree.

set -euo pipefail

REPO="${THIMBLE_REPO:-cartine/thimble}"

# ---------------------------------------------------------------------------
# Bump algorithm — exposed so test_tag_release_bump.sh can source us.

# Compute the next version given the latest tag and a kind (patch|minor|major
# or an explicit vX.Y.Z). Echoes the next version (with leading v).
# Special case: "no prior tag" (latest is empty) suggests v0.1.0 as the
# first patch — matches the plan note about cutting v0.1.0 as the first
# release. minor and major from no-prior-tag also map to v0.1.0 / v1.0.0.
compute_next_version() {
  latest="$1"
  kind="$2"

  case "$kind" in
    v[0-9]*.[0-9]*.[0-9]*)
      if printf '%s\n' "$kind" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
        printf '%s\n' "$kind"
        return 0
      fi
      echo "tag-release: invalid version: $kind" >&2
      return 1
      ;;
  esac

  if [ -z "$latest" ]; then
    case "$kind" in
      patch|minor) printf 'v0.1.0\n' ;;
      major) printf 'v1.0.0\n' ;;
      *)
        echo "tag-release: unknown bump kind: $kind" >&2
        return 1
        ;;
    esac
    return 0
  fi

  cur="${latest#v}"
  major="${cur%%.*}"
  rest="${cur#*.}"
  minor="${rest%%.*}"
  patch="${rest#*.}"
  case "$kind" in
    patch) patch=$((patch + 1)) ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    major) major=$((major + 1)); minor=0; patch=0 ;;
    *)
      echo "tag-release: unknown bump kind: $kind" >&2
      return 1
      ;;
  esac
  printf 'v%d.%d.%d\n' "$major" "$minor" "$patch"
}

# Library mode: when sourced from tests, return after function defs.
if [ "${TAG_RELEASE_LIB_ONLY:-}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi

usage() {
  cat >&2 <<'EOF'
usage: scripts/tag-release.sh <patch|minor|major|vX.Y.Z> [--dry-run]

env:
  THIMBLE_REPO=cartine/thimble  # source repo (default)

Refuses unless:
  - working tree is clean
  - branch == main
  - CHANGELOG.md has [Unreleased] content
EOF
  exit 2
}

VERSION_INPUT=""
DRY_RUN=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    -h|--help) usage ;;
    -*) echo "tag-release: unknown flag: $arg" >&2; usage ;;
    *)
      if [ -n "$VERSION_INPUT" ]; then
        echo "tag-release: extra arg: $arg" >&2
        usage
      fi
      VERSION_INPUT="$arg"
      ;;
  esac
done

if [ -z "$VERSION_INPUT" ]; then
  usage
fi

run() {
  if [ -n "$DRY_RUN" ]; then
    printf '[dry-run] %s\n' "$*"
  else
    "$@"
  fi
}

# ---------------------------------------------------------------------------
# Preconditions.

if [ -z "$DRY_RUN" ]; then
  if [ -n "$(git status --porcelain)" ]; then
    echo "tag-release: working tree dirty; commit or stash first." >&2
    exit 1
  fi
  branch="$(git rev-parse --abbrev-ref HEAD)"
  if [ "$branch" != "main" ]; then
    echo "tag-release: must run on main; current branch=$branch" >&2
    exit 1
  fi
fi

# Determine the previous tag (may be empty for the first release).
prev_tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"
NEXT_VERSION="$(compute_next_version "$prev_tag" "$VERSION_INPUT")"
NEXT_NO_V="${NEXT_VERSION#v}"
echo "tag-release: prev=${prev_tag:-<none>}  next=$NEXT_VERSION"

# CHANGELOG must have [Unreleased] content (refuse to cut an empty
# release). The block is "the lines from `## [Unreleased]` up to the
# next `## [` heading"; if every non-blank line in that block is just
# the heading itself, refuse.
if [ -f CHANGELOG.md ]; then
  unreleased_body="$(awk '
    /^## \[Unreleased\]/ {capture=1; next}
    capture && /^## \[/ {capture=0}
    capture {print}
  ' CHANGELOG.md | grep -v '^[[:space:]]*$' || true)"
  if [ -z "$unreleased_body" ]; then
    echo "tag-release: CHANGELOG.md [Unreleased] block is empty; refusing to cut." >&2
    exit 1
  fi
fi

today="$(date -u +%Y-%m-%d)"

# ---------------------------------------------------------------------------
# Update CHANGELOG: rename [Unreleased] -> [X.Y.Z] — date, add fresh
# [Unreleased] block above it, refresh link references.
update_changelog() {
  if [ ! -f CHANGELOG.md ]; then
    echo "tag-release: CHANGELOG.md missing — skipping rewrite" >&2
    return 0
  fi
  python3 - "$NEXT_NO_V" "$today" "$prev_tag" <<'PY'
import os
import re
import sys

next_no_v, today, prev_tag = sys.argv[1], sys.argv[2], sys.argv[3]
path = "CHANGELOG.md"
with open(path, "r", encoding="utf-8") as f:
    text = f.read()

# Rename the existing [Unreleased] heading to [next] — today.
new_heading = f"## [{next_no_v}] — {today}"
text = re.sub(r"^## \[Unreleased\]\s*$", new_heading, text, count=1, flags=re.M)

# Insert a fresh empty [Unreleased] block above it.
fresh = "## [Unreleased]\n\n### Added\n\n"
text = text.replace(new_heading, fresh + new_heading, 1)

# Rewrite link references at the bottom. Keep any others.
lines = text.splitlines()
keep = []
saw_unreleased_ref = False
for line in lines:
    if re.match(r"^\[Unreleased\]:", line):
        saw_unreleased_ref = True
        keep.append(
            f"[Unreleased]: https://github.com/cartine/thimble/compare/v{next_no_v}...HEAD"
        )
        continue
    keep.append(line)

# Add a [next] link reference if not present.
ref_line = f"[{next_no_v}]: https://github.com/cartine/thimble/releases/tag/v{next_no_v}"
if ref_line not in keep:
    keep.append(ref_line)

if not saw_unreleased_ref:
    keep.append(
        f"[Unreleased]: https://github.com/cartine/thimble/compare/v{next_no_v}...HEAD"
    )

with open(path, "w", encoding="utf-8") as f:
    f.write("\n".join(keep).rstrip() + "\n")
PY
}

if [ -n "$DRY_RUN" ]; then
  echo "[dry-run] update CHANGELOG.md: rename [Unreleased] -> [$NEXT_NO_V] — $today"
  echo "[dry-run] git add CHANGELOG.md"
  echo "[dry-run] git commit -m 'release: $NEXT_VERSION'"
  echo "[dry-run] git tag $NEXT_VERSION"
  echo "[dry-run] git push origin main $NEXT_VERSION"
  echo "[dry-run] gh run watch --workflow=release.yml"
  echo "[dry-run] verify checksum + attestation for each release artifact"
  echo "[dry-run] would print: ready: https://github.com/$REPO/releases/tag/$NEXT_VERSION"
  exit 0
fi

# ---------------------------------------------------------------------------
# Live path.
update_changelog
run git add CHANGELOG.md
run git commit -m "release: $NEXT_VERSION"
run git tag "$NEXT_VERSION"
run git push origin main "$NEXT_VERSION"

# Capture the release-workflow run id and watch it to completion.
sleep 2
run_id="$(gh run list --workflow=release.yml --limit=1 \
  --json databaseId --jq '.[0].databaseId')"
if [ -z "$run_id" ]; then
  echo "tag-release: failed to capture release run id." >&2
  exit 1
fi
run gh run watch --exit-status "$run_id"

# Download and verify each artifact.
verify_dir="$(mktemp -d -t thimble-release-verify-XXXXXX)"
trap 'rm -rf "$verify_dir"' EXIT
(
  cd "$verify_dir"
  run gh release download "$NEXT_VERSION" --repo "$REPO"
  run sha256sum -c checksums.txt
  for f in thimble_*.tar.gz; do
    run gh attestation verify "$f" --repo "$REPO"
  done
)

printf 'ready: https://github.com/%s/releases/tag/%s\n' "$REPO" "$NEXT_VERSION"
