---
name: ship-release
description: >-
  Ship a new Thimble release by syncing main, previewing changes since the
  last tag, running quality gates, cutting the tag, watching the release
  workflow to green, and verifying published artifacts and attestation.
---

# /ship-release

Ship a new Thimble release.

## Use this skill when

- The user asks to ship, cut, publish, or tag a new Thimble release.
- The user wants help recovering when a tag was pushed but the release
  workflow failed or assets did not publish.

## Principles

- Release from `main`, not from a feature branch or worktree.
- Keep the working tree clean before starting.
- Sync `main` before previewing or running gates.
- Do not use a PR workflow for the version bump — Thimble ships from
  `main` via `scripts/tag-release.sh`.
- Do not delete or rewrite a failing tag silently. If the release path
  fails, fix forward and re-tag.
- Do not manually create the GitHub Release or upload assets — that is
  the release workflow's job.

## Steps

1. **Determine bump type** — Ask whether this is a `patch`, `minor`, or
   `major` release unless already specified (e.g. `/ship-release patch`)
   or an explicit `vX.Y.Z` was given. Show the latest tag and what each
   bump would produce:

   ```bash
   git describe --tags --abbrev=0 2>/dev/null || echo "(no prior tag)"
   ```

2. **Preflight and sync** — Confirm the repo is on `main`, clean, and
   current:

   ```bash
   git status --porcelain
   git rev-parse --abbrev-ref HEAD
   git fetch origin main --tags
   git pull --ff-only
   gh auth status
   gh run list --workflow=release.yml --limit=1 \
     --json status --jq '.[0].status'
   ```

   Refuse if the working tree is dirty, the branch is not `main`,
   `gh` is unauthenticated, or a release workflow is already running.
   If `git pull --ff-only` updates `main`, continue from the new HEAD.
   If it fails because the branch diverged, stop and surface the exact
   remediation.

3. **Preview changes** — After syncing, show what is shipping:

   ```bash
   git log "$(git describe --tags --abbrev=0)..HEAD" --oneline
   ```

   Present a brief summary so the user can confirm scope before the cut.

4. **Confirm CHANGELOG coverage** — Open `CHANGELOG.md` and verify the
   `## [Unreleased]` block is non-empty between that heading and the
   next `## [` heading. Refuse to ship if it is empty — an empty
   `[Unreleased]` means there is nothing to release.

   Walk the commits from step 3 and confirm each user-facing change is
   reflected somewhere in the `[Unreleased]` block. If a user-facing
   commit is missing from the changelog, stop and surface the gap.
   Offer to author the missing entry before continuing. Internal-only
   changes (refactors, test-only, CI, doc tweaks, dep bumps with no
   behavior change) do not need a changelog entry.

   `scripts/tag-release.sh` will rewrite this block to
   `## [X.Y.Z] — YYYY-MM-DD` and refresh the link references at the
   bottom — do not edit those manually here.

5. **Run quality gates** — Execute in parallel and stop on any failure:

   ```bash
   make lint
   go test -race ./...
   ```

   If integration coverage matters for this release, also run
   `make integration` (requires `age` + `age-keygen` on PATH).

6. **Cut the release** — Run `scripts/tag-release.sh` via the Makefile.
   Prefer a dry-run first if the operator is unsure:

   ```bash
   make tag-release VERSION=<bump_or_version> DRY_RUN=1   # optional preview
   make tag-release VERSION=<bump_or_version>
   ```

   The script bumps the version, rewrites `CHANGELOG.md`, commits, tags
   `vX.Y.Z`, pushes `main` and the tag atomically, then watches the
   `release.yml` workflow to completion and verifies checksums plus the
   SLSA build-provenance attestation for each tarball.

   If the workflow fails, the script exits non-zero. Surface the failing
   job URL and stop — do not retry blindly.

7. **Verify published artifacts** — After the workflow goes green,
   confirm the GitHub Release for `vX.Y.Z` exists and that the runtime
   tarballs, `checksums.txt`, and attestations are attached:

   ```bash
   gh release view vX.Y.Z --repo cartine/thimble
   ```

   If the script already ran `sha256sum -c` and `gh attestation verify`
   against the published assets, this is a final sanity check rather
   than a re-run.

8. **Report** — Print one line on success:

   ```
   ready: https://github.com/cartine/thimble/releases/tag/vX.Y.Z
   ```

   Include a one- or two-bullet summary lifted from the `[X.Y.Z]`
   section of `CHANGELOG.md` so the user can see the release story
   without opening GitHub.

## Recovery

- **Tag pushed, workflow failed** — Investigate the failing run, fix
  forward on `main`, then cut a new patch tag. Do not delete the
  failing tag silently; operators expect it in `git log --tags` for
  post-mortem.
- **Tag pushed, workflow green, assets missing** — Re-run the workflow
  via `gh workflow run release.yml -f tag=vX.Y.Z` (or via the GitHub
  UI). The `verify-release` workflow can re-check checksums.
- **Dirty working tree after CHANGELOG rewrite** — `git restore
  CHANGELOG.md` to discard the in-progress rewrite, then start over
  from step 4.
- **Tag collision** — A `vX.Y.Z` tag already exists. Either pick a
  different bump or coordinate with maintainers; never force-push an
  existing release tag.

## How to test without cutting a real release

`scripts/tag-release.sh` supports `--dry-run`, which prints every
git/gh side effect with a `[dry-run]` prefix and exits without touching
the repo or GitHub:

```bash
make tag-release VERSION=patch DRY_RUN=1
# or directly:
bash scripts/tag-release.sh patch --dry-run
```

The bump algorithm has its own test:

```bash
bash scripts/test_tag_release_bump.sh
```

## Don't

- Don't ship from a branch other than `main` — the release workflow
  only fires on tag pushes that descend from `main`.
- Don't push tags that don't match `^v[0-9]+\.[0-9]+\.[0-9]+$`. The
  verify and attestation steps assume strict semver.
- Don't ship with an empty `[Unreleased]` block in `CHANGELOG.md`.
- Don't manually create the GitHub Release or upload `tar.gz` /
  `checksums.txt` — the release workflow owns those artifacts and the
  build-provenance attestation only matches the workflow-built copies.
