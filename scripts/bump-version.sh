#!/usr/bin/env bash
# scripts/bump-version.sh — propose a new Thimble release as a PR.
#
# The whole release flow is split in two:
#   1. (here) Bump the top-level VERSION file, rewrite the CHANGELOG
#      [Unreleased] block to [X.Y.Z] — today, commit on a release
#      branch, push, open a PR.
#   2. (workflow) When the PR merges, .github/workflows/release.yml
#      sees the VERSION change on main, builds the tarballs, tags
#      vX.Y.Z, and publishes the GitHub Release.
#
# The local step does no `git tag`, no `git push origin <tag>`, no
# `gh pr merge`, no `gh run watch`. That eliminates the three races
# the previous tag-release.sh hit on protected branches.
#
# Usage:
#   scripts/bump-version.sh patch
#   scripts/bump-version.sh minor
#   scripts/bump-version.sh major
#   scripts/bump-version.sh v0.2.5
#   scripts/bump-version.sh patch --dry-run

set -euo pipefail

REPO="${THIMBLE_REPO:-cartine/thimble}"

# Compute next version from the current VERSION file + a bump kind.
# Echoes the next version with leading "v".
compute_next_version() {
  current="$1"
  kind="$2"
  case "$kind" in
    v[0-9]*.[0-9]*.[0-9]*)
      if printf '%s\n' "$kind" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
        printf '%s\n' "$kind"
        return 0
      fi
      echo "bump-version: invalid version: $kind" >&2
      return 1
      ;;
  esac
  if [ -z "$current" ]; then
    case "$kind" in
      patch|minor) printf 'v0.1.0\n' ;;
      major)       printf 'v1.0.0\n' ;;
      *) echo "bump-version: unknown bump kind: $kind" >&2; return 1 ;;
    esac
    return 0
  fi
  major="${current%%.*}"
  rest="${current#*.}"
  minor="${rest%%.*}"
  patch="${rest#*.}"
  case "$kind" in
    patch) patch=$((patch + 1)) ;;
    minor) minor=$((minor + 1)); patch=0 ;;
    major) major=$((major + 1)); minor=0; patch=0 ;;
    *) echo "bump-version: unknown bump kind: $kind" >&2; return 1 ;;
  esac
  printf 'v%d.%d.%d\n' "$major" "$minor" "$patch"
}

# Library mode: when sourced from tests, return after function defs.
if [ "${BUMP_VERSION_LIB_ONLY:-}" = "1" ]; then
  return 0 2>/dev/null || exit 0
fi

usage() {
  cat >&2 <<'EOF'
usage: scripts/bump-version.sh <patch|minor|major|vX.Y.Z> [--dry-run]
EOF
  exit 2
}

VERSION_INPUT=""
DRY_RUN=""
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    -h|--help) usage ;;
    -*) echo "bump-version: unknown flag: $arg" >&2; usage ;;
    *)
      if [ -n "$VERSION_INPUT" ]; then
        echo "bump-version: extra arg: $arg" >&2
        usage
      fi
      VERSION_INPUT="$arg"
      ;;
  esac
done
[ -n "$VERSION_INPUT" ] || usage

run() {
  if [ -n "$DRY_RUN" ]; then
    printf '[dry-run] %s\n' "$*"
  else
    "$@"
  fi
}

# Preconditions (skipped in dry-run so the script remains rehearsable).
if [ -z "$DRY_RUN" ]; then
  if [ -n "$(git status --porcelain)" ]; then
    echo "bump-version: working tree dirty; commit or stash first." >&2
    exit 1
  fi
  branch="$(git rev-parse --abbrev-ref HEAD)"
  if [ "$branch" != "main" ]; then
    echo "bump-version: must run on main; current=$branch" >&2
    exit 1
  fi
fi

current_version=""
if [ -f VERSION ]; then
  current_version="$(tr -d '[:space:]' < VERSION)"
fi
NEXT_VERSION="$(compute_next_version "$current_version" "$VERSION_INPUT")"
NEXT_NO_V="${NEXT_VERSION#v}"
echo "bump-version: current=${current_version:-<none>}  next=$NEXT_VERSION"

# CHANGELOG must have an [Unreleased] block with body.
if [ -f CHANGELOG.md ]; then
  unreleased_body="$(awk '
    /^## \[Unreleased\]/ {capture=1; next}
    capture && /^## \[/ {capture=0}
    capture {print}
  ' CHANGELOG.md | grep -v '^[[:space:]]*$' || true)"
  if [ -z "$unreleased_body" ]; then
    echo "bump-version: CHANGELOG.md [Unreleased] block is empty; refusing." >&2
    exit 1
  fi
fi

today="$(date -u +%Y-%m-%d)"

# Rewrite CHANGELOG: rename [Unreleased] -> [X.Y.Z] — date, insert
# fresh empty [Unreleased] above it, refresh link references.
update_changelog() {
  [ -z "$DRY_RUN" ] || return 0
  [ -f CHANGELOG.md ] || return 0
  python3 - "$NEXT_NO_V" "$today" <<'PY'
import re
import sys

next_no_v, today = sys.argv[1], sys.argv[2]
path = "CHANGELOG.md"
with open(path, "r", encoding="utf-8") as f:
    text = f.read()

new_heading = f"## [{next_no_v}] — {today}"
text = re.sub(r"^## \[Unreleased\]\s*$", new_heading, text, count=1, flags=re.M)

fresh = "## [Unreleased]\n\n### Added\n\n"
text = text.replace(new_heading, fresh + new_heading, 1)

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

release_branch="release/$NEXT_VERSION"
run git checkout -b "$release_branch"
if [ -n "$DRY_RUN" ]; then
  echo "[dry-run] write VERSION: $NEXT_NO_V"
  echo "[dry-run] update CHANGELOG.md: rename [Unreleased] -> [$NEXT_NO_V] — $today"
else
  printf '%s\n' "$NEXT_NO_V" > VERSION
  update_changelog
fi
run git add VERSION CHANGELOG.md
run git commit -m "release: $NEXT_VERSION"
run git push -u origin "$release_branch"
run gh pr create --base main --head "$release_branch" \
  --title "release: $NEXT_VERSION" \
  --body "Bumps VERSION to ${NEXT_NO_V} and finalizes the CHANGELOG block.

When this PR merges, .github/workflows/release.yml detects the
VERSION change on main, builds the tarballs, tags ${NEXT_VERSION},
and publishes the GitHub Release. No further action required after
the merge."

echo "next: review the PR, get CI green, merge it."
echo "      the release workflow takes over from there."
