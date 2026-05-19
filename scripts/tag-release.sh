#!/usr/bin/env bash
# scripts/tag-release.sh — cut a Thimble release.
#
# Wraps the manual release flow (bump version → CHANGELOG entry → tag →
# push → watch workflow → verify checksums + attestation) into one
# command. The same logic is exposed by `make tag-release VERSION=…`
# and by the `/ship-release` agent skill.
#
# Usage:
#   scripts/tag-release.sh patch
#   scripts/tag-release.sh minor
#   scripts/tag-release.sh major
#   scripts/tag-release.sh v0.2.5
#   scripts/tag-release.sh patch --dry-run
#
# Two flows, selected automatically:
#   - Unprotected main: rewrite CHANGELOG, commit, tag, atomic-push
#     main + tag together.
#   - Protected main (any ruleset with type=pull_request): commit on a
#     release/vX.Y.Z branch, open a PR, wait for required checks to go
#     green, rebase-merge it, then tag the resulting commit on main
#     and push the tag. GitHub's rebase rewrites SHAs even on linear
#     PRs, so the tag must point at the post-merge sha — not the local
#     pre-merge sha — or it ends up orphan.
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
# `git tag --sort=-version:refname` finds the latest v* tag even if
# the tag is not an ancestor of HEAD — which can happen on a protected
# main where the rebase-merge rewrote SHAs and left the tag orphan.
# `git describe --tags --abbrev=0` would miss that tag and propose the
# same version again.
prev_tag="$(git tag --list 'v*' --sort=-version:refname | head -n 1)"
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
  if [ -n "$DRY_RUN" ]; then
    return 0
  fi
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

# ---------------------------------------------------------------------------
# Path detection and helpers.

# Does main require changes to go through a PR? Reads the repo's
# branch rulesets. Returns 0 (true) if any active rule on main has
# type=pull_request, 1 otherwise. Falls back to "no PR required" if
# the API call fails (e.g. gh unauthenticated in dry-run).
requires_pr() {
  out="$(gh api "repos/$REPO/rules/branches/main" \
    --jq '[.[] | select(.type=="pull_request")] | length' 2>/dev/null \
    || echo 0)"
  [ "${out:-0}" != "0" ]
}

# Poll the PR until GitHub reports at least one check run, so the
# subsequent `gh pr checks --watch` has something to watch. Gives up
# after ~60s of polling — at that point either the PR's workflows
# really don't trigger any checks (which the user must fix in CI
# config) or `--watch` will fail loudly with the same "no checks
# reported" message and we surface that.
wait_for_checks_to_register() {
  if [ -n "$DRY_RUN" ]; then
    return 0
  fi
  i=0
  while [ "$i" -lt 12 ]; do
    count="$(gh pr view "$release_branch" --json statusCheckRollup \
      --jq '.statusCheckRollup | length' 2>/dev/null || echo 0)"
    if [ "${count:-0}" -gt 0 ]; then
      return 0
    fi
    sleep 5
    i=$((i + 1))
  done
  return 0
}

# Pick a merge method gh-CLI flag that the repo actually allows.
# Prefer --rebase (linear history, no synthetic merge commits), then
# --merge, then --squash. Returns 1 if none are allowed so the caller
# can surface the failure — `exit 1` inside a $(...) subshell would
# only exit the subshell.
pick_merge_flag() {
  allowed="$(gh api "repos/$REPO" --jq \
    '"\(.allow_rebase_merge) \(.allow_merge_commit) \(.allow_squash_merge)"' \
    2>/dev/null || echo "true true true")"
  case "$allowed" in
    true*)              echo "--rebase"; return 0 ;;
    "false true"*)      echo "--merge"; return 0 ;;
    "false false true") echo "--squash"; return 0 ;;
    *) return 1 ;;
  esac
}

# ---------------------------------------------------------------------------
# Cut strategies. Each owns the full sequence from CHANGELOG rewrite
# through tag push. The caller dispatches based on requires_pr.

cut_direct() {
  update_changelog
  run git add CHANGELOG.md
  run git commit -m "release: $NEXT_VERSION"
  run git tag "$NEXT_VERSION"
  run git push origin main "$NEXT_VERSION"
}

cut_via_pr() {
  release_branch="release/$NEXT_VERSION"
  if [ -z "$DRY_RUN" ] \
     && git show-ref --verify --quiet "refs/heads/$release_branch"; then
    echo "tag-release: branch $release_branch already exists locally; aborting." >&2
    exit 1
  fi

  run git checkout -b "$release_branch"
  update_changelog
  run git add CHANGELOG.md
  run git commit -m "release: $NEXT_VERSION"
  run git push -u origin "$release_branch"

  pr_body="Cuts $NEXT_VERSION. Generated by scripts/tag-release.sh.

When this PR merges, the script tags the resulting commit on main and
pushes the tag, which triggers the release workflow."
  run gh pr create --base main --head "$release_branch" \
    --title "release: $NEXT_VERSION" --body "$pr_body"

  # `gh pr checks --watch` exits non-zero with "no checks reported" if
  # it runs before GitHub has registered any check runs for the PR.
  # That's a race we hit on the first cut: push → PR create → watch
  # happens in seconds, but the checks API takes ~5-15s longer to
  # populate. Poll briefly until at least one check appears, then
  # hand off to --watch for the rest.
  wait_for_checks_to_register
  run gh pr checks "$release_branch" --watch

  if ! merge_flag="$(pick_merge_flag)"; then
    echo "tag-release: no merge method enabled on $REPO." >&2
    exit 1
  fi
  run gh pr merge "$release_branch" "$merge_flag" --delete-branch

  # Read the post-merge SHA on main and tag *that*. Rebase-merge
  # rewrites SHAs even on linear PRs, so the local pre-merge commit
  # is not what landed.
  if [ -z "$DRY_RUN" ]; then
    merge_sha="$(gh pr view "$release_branch" \
      --json mergeCommit --jq '.mergeCommit.oid')"
    if [ -z "$merge_sha" ] || [ "$merge_sha" = "null" ]; then
      echo "tag-release: failed to read merge commit sha from PR." >&2
      exit 1
    fi
  else
    merge_sha="<post-merge-sha>"
  fi
  run git fetch origin main
  run git checkout main
  run git reset --hard origin/main
  run git tag "$NEXT_VERSION" "$merge_sha"
  run git push origin "$NEXT_VERSION"
}

# ---------------------------------------------------------------------------
# Dispatch.

if requires_pr; then
  echo "tag-release: main is protected — cutting via PR."
  STRATEGY="pr"
else
  echo "tag-release: main is unprotected — cutting directly."
  STRATEGY="direct"
fi

if [ -n "$DRY_RUN" ]; then
  echo "[dry-run] update CHANGELOG.md: rename [Unreleased] -> [$NEXT_NO_V] — $today"
fi

if [ "$STRATEGY" = "pr" ]; then
  cut_via_pr
else
  cut_direct
fi

if [ -n "$DRY_RUN" ]; then
  echo "[dry-run] gh run watch --workflow=release.yml"
  echo "[dry-run] verify checksum + attestation for each release artifact"
  echo "[dry-run] would print: ready: https://github.com/$REPO/releases/tag/$NEXT_VERSION"
  exit 0
fi

# ---------------------------------------------------------------------------
# Workflow watch + artifact verification (both strategies).

sleep 2
run_id="$(gh run list --workflow=release.yml --limit=1 \
  --json databaseId --jq '.[0].databaseId')"
if [ -z "$run_id" ]; then
  echo "tag-release: failed to capture release run id." >&2
  exit 1
fi
run gh run watch --exit-status "$run_id"

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
