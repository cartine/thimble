---
name: ship-release
description: >-
  Ship a new Thimble release by opening a VERSION-bump PR, getting it
  merged, and letting the release workflow detect the change on main
  and publish the GitHub Release.
---

# /ship-release

Ship a new Thimble release.

## Use this skill when

- The user asks to ship, cut, publish, or tag a new Thimble release.
- The user wants to verify or recover a release that didn't fully
  publish.

## How the flow is split

Thimble's release is workflow-driven, following the same pattern as
`~/knots`. There are two halves:

1. **Local "propose"** — `make bump-version VERSION=…` edits the
   top-level `VERSION` file, rewrites `[Unreleased]` →
   `[X.Y.Z] — YYYY-MM-DD` in `CHANGELOG.md`, commits on a
   `release/vX.Y.Z` branch, pushes, and opens a PR. That is all
   the local script does — no `git tag`, no `git push <tag>`, no
   waiting on workflows.
2. **Workflow "publish"** — when the PR merges,
   `.github/workflows/release.yml` triggers on `push: main`,
   detects the `VERSION` change vs. the previous commit, builds
   the per-platform tarballs, generates `checksums.txt`, tags
   `vX.Y.Z`, and creates the GitHub Release. It is idempotent —
   if the release already exists with every expected asset, it
   skips; if partial, it completes it.

This split is intentional. The previous tag-release-locally model
hit three separate races on a protected `main` (atomic tag+main
push, `gh pr checks --watch`, `gh run list --limit=1`); all three
disappear when the workflow owns the tag and the release.

## Steps

1. **Preflight** — Confirm the repo is on `main`, clean, and current.

   ```bash
   git status --porcelain
   git rev-parse --abbrev-ref HEAD
   git fetch origin --tags
   git pull --ff-only
   ```

   Refuse if dirty, not on `main`, or if `git pull --ff-only` fails.

2. **Determine bump type** — Ask whether this is a `patch`, `minor`,
   or `major` release unless already specified (e.g.
   `/ship-release patch`) or an explicit `vX.Y.Z` was given. Show
   the current `VERSION` and what each bump would produce.

3. **Confirm CHANGELOG coverage** — Open `CHANGELOG.md` and verify
   the `## [Unreleased]` block is non-empty. Refuse to ship if it
   is. Walk the commits since the last release tag and confirm
   each user-facing change is reflected in `[Unreleased]`. If a
   user-facing commit is missing, stop and surface the gap. Offer
   to author the missing entry before continuing.

4. **Run local quality gates** — Optional but recommended:

   ```bash
   make lint
   go test -race ./...
   ```

   These are also run by required PR checks, so failing locally
   means the PR will fail too.

5. **Open the release PR** — Run:

   ```bash
   make bump-version VERSION=<bump_or_version>           # opens the PR
   make bump-version VERSION=<bump_or_version> DRY_RUN=1 # rehearsal
   ```

   The script edits `VERSION` and `CHANGELOG.md`, commits on
   `release/vX.Y.Z`, pushes, and opens a PR titled
   `release: vX.Y.Z`.

6. **Wait for required checks** — On the PR, wait for the
   `build-test (*)`, `lint`, `govulncheck`, `integration (real
   age)` required checks to pass. `gh pr checks <pr> --watch`
   works once they register (~10s after PR creation).

7. **Merge the PR** — `gh pr merge <pr> --rebase --delete-branch`
   (or via the GitHub UI). The merge triggers the release workflow.

8. **Watch the release workflow** — The workflow detects the
   `VERSION` change, builds, and publishes. Watch it with:

   ```bash
   sleep 5  # let GitHub register the run
   run_id=$(gh run list --workflow=release.yml --branch=main \
     --limit=1 --json databaseId --jq '.[0].databaseId')
   gh run watch --exit-status "$run_id"
   ```

   Don't use `--limit=1` without `--branch=main` — that can return
   an older tag-triggered run from before this refactor.

9. **Verify published outputs** — Confirm the release exists with
   the expected assets:

   ```bash
   gh release view vX.Y.Z --repo cartine/thimble
   ```

   Expect: four tarballs (`thimble_X.Y.Z_{linux,darwin}_{amd64,arm64}.tar.gz`),
   `checksums.txt`, and `install.sh`.

10. **Report** — One line:

    ```
    ready: https://github.com/cartine/thimble/releases/tag/vX.Y.Z
    ```

    Include a one- or two-bullet summary lifted from the
    `[X.Y.Z]` section of `CHANGELOG.md`.

## Recovery

The workflow is idempotent. If publish fails partway:

- **Release exists but assets missing** — Re-run the workflow
  with `gh workflow run release.yml -f version=X.Y.Z`. The
  detect step sees the partial release and re-enters publish to
  complete it.
- **VERSION on main matches an unreleased tag** — Workflow will
  detect and publish. No manual intervention.
- **Wrong version landed on main** — Open a new bump PR with the
  intended version (don't try to "fix" the existing tag). The
  workflow only fires when `VERSION` *changes*, so a same-version
  re-push is a no-op.

## Don't

- Don't `git tag` or `git push origin <tag>` locally. The
  workflow owns tags.
- Don't manually create the GitHub Release or upload assets —
  the workflow's idempotent recovery will fight you.
- Don't bypass the release PR by editing `VERSION` directly on
  `main` (would skip code review and CI gates on the bump).
- Don't ship with an empty `[Unreleased]` block.
