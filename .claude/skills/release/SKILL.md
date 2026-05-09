---
name: release
description: >-
  Cut a Thimble release. Bumps the version (patch/minor/major), updates
  CHANGELOG, tags, pushes, and watches the release workflow to green.
---

# Release

Drives a full Thimble release from one entry point. The same flow is
available as `make tag-release VERSION=…`; this skill is the agent-driven
counterpart so an operator can say `/release patch` and watch.

The script that does the work is
[`scripts/tag-release.sh`](../../../scripts/tag-release.sh). The skill
exists to gate the call — verify preconditions, classify failures
clearly, and surface the result.

## Preconditions

Refuse if any of the following are not true:

- `git status --porcelain` is empty (clean working tree).
- `git rev-parse --abbrev-ref HEAD` reports `main`.
- `gh auth status` exits 0 (so the watch + attestation steps can run).
- No release workflow is currently in progress (`gh run list
  --workflow=release.yml --limit=1 --json status --jq '.[0].status'`
  reports `completed`).
- `make lint` exits 0.
- `go test ./...` exits 0.

If a precondition fails, print one line per failure and stop. Do not
proceed with the cut.

## Bump algorithm

Read the latest tag with:

```sh
git describe --tags --abbrev=0 2>/dev/null || true
```

Parse it as `vMAJOR.MINOR.PATCH`. Compute the next version from the
operator's input:

| Input    | Behaviour                                                      |
|----------|----------------------------------------------------------------|
| `patch`  | latest with `PATCH+=1`. No prior tag → `v0.1.0`.               |
| `minor`  | latest with `MINOR+=1`, `PATCH=0`. No prior tag → `v0.1.0`.    |
| `major`  | latest with `MAJOR+=1`, `MINOR=0`, `PATCH=0`. No prior → `v1.0.0`. |
| `vX.Y.Z` | use as-is. Must match `^v[0-9]+\.[0-9]+\.[0-9]+$`.             |

This matches `compute_next_version` in `scripts/tag-release.sh`. The
companion test
[`scripts/test_tag_release_bump.sh`](../../../scripts/test_tag_release_bump.sh)
exercises the matrix.

## Update CHANGELOG.md

Open `CHANGELOG.md`, find the `## [Unreleased]` heading, and:

1. Confirm the block is non-empty between `[Unreleased]` and the next
   `## [` heading. Refuse to cut if it is empty (no changelog =
   nothing to release).
2. Rename the heading to `## [X.Y.Z] — YYYY-MM-DD` (UTC date).
3. Insert a fresh empty `## [Unreleased]` block above it with an
   `### Added` subheading.
4. Refresh the link references at the bottom:
   - `[Unreleased]: https://github.com/cartine/thimble/compare/vX.Y.Z...HEAD`
   - `[X.Y.Z]: https://github.com/cartine/thimble/releases/tag/vX.Y.Z`

## Commit, tag, push

```sh
git add CHANGELOG.md
git commit -m "release: vX.Y.Z"
git tag vX.Y.Z
git push origin main vX.Y.Z
```

## Watch the release workflow

Capture the run id and watch it to completion:

```sh
sleep 2  # let the tag-push event register a workflow run
RUN_ID=$(gh run list --workflow=release.yml --limit=1 \
  --json databaseId --jq '.[0].databaseId')
gh run watch --exit-status "$RUN_ID"
```

If `gh run watch` returns non-zero, surface the failing job's URL and
stop. Do not attempt artifact verification on a failed run.

## Verify artifacts

After the workflow goes green, download the published assets and run
both verifications:

```sh
verify_dir=$(mktemp -d)
cd "$verify_dir"
gh release download vX.Y.Z --repo cartine/thimble
sha256sum -c checksums.txt
for f in thimble_*.tar.gz; do
  gh attestation verify "$f" --repo cartine/thimble
done
cd - >/dev/null
rm -rf "$verify_dir"
```

`gh attestation verify` is the SLSA build-provenance check from K-40 —
it ties the artifact to the exact workflow run and source commit.

## Report

On success, print exactly one line:

```
ready: https://github.com/cartine/thimble/releases/tag/vX.Y.Z
```

On failure at any step, print:

- which step failed,
- the upstream URL (workflow run, release page) when relevant,
- a one-line rollback hint:
  - tag pushed but workflow failed: investigate the run, fix forward
    on `main`, then re-tag (do **not** delete the failing tag silently).
  - working tree dirty after CHANGELOG rewrite: `git restore CHANGELOG.md`.
  - already-pushed tag conflict: pick a new bump or coordinate with
    other maintainers.

## How to test

There is no way to safely cut a real release from a test session. The
script supports `--dry-run` which prints every shell side effect with
a `[dry-run]` prefix and exits 0 without touching git or GitHub:

```sh
make tag-release VERSION=patch DRY_RUN=1
# or directly:
bash scripts/tag-release.sh patch --dry-run
```

The bump algorithm is independently tested:

```sh
bash scripts/test_tag_release_bump.sh
```

## Don't

- Do not skip CHANGELOG. An empty `[Unreleased]` block means no
  release. Refuse.
- Do not push tags that don't match `^v[0-9]+\.[0-9]+\.[0-9]+$`. The
  release workflow's tag filter is `v*` but the verify and attestation
  steps assume strict semver.
- Do not delete a failing tag silently — operators expect the failed
  tag in `git log --tags` so they can post-mortem the run.
- Do not run on anything other than `main`. The release workflow only
  fires on tag pushes that descend from `main`.
