# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Stripped git as a transport from documentation and examples. Thimble's
  data model is file-first; movement of bundles and manifests between
  operators and hosts uses any file transport (rsync over ssh, object
  storage, etc.). Pattern A (store host with sshd) is the recommended
  default. See README "Storing and Syncing" (added in K-54). Mentions of
  git for Thimble's own development workflow (cloning the source repo,
  release pipeline) are unchanged.

### Added

- Documented the file-first replication model: Pattern A (store host) as
  the recommended default, Pattern B (object storage — S3/MinIO/GCS) and
  Pattern C (direct host-to-host) as alternatives. Concurrency safety is
  provided by manifest versions (K-21) and append-only audit log merge
  (K-27). thimble.md "Peer-to-peer shape" expanded with design framing;
  CONTRIBUTING.md gains a "Running multi-leader" pointer to the README
  section. A future trio of knots (K-55..K-57) will add `thimble peer`
  subcommands as sugar over the same primitives.
- 48-knot hardening rollout tracked in [tasks/knot-plan.md](tasks/knot-plan.md)
  via the [`kno`](https://github.com/cartine/knots) execution-plan tooling.
- LICENSE (Apache-2.0) at repo root.
- Real disclosure policy in SECURITY.md (GitHub Private Vulnerability Reporting
  + `its@thecartine.me`); previous internal review notes preserved at
  `docs/security-review.md`.
- CONTRIBUTING.md with repo layout, coding standards, PR workflow.
- Source-code size standard enforced by `make lint`: <500 lines/file,
  <100 lines/function, <100 columns/line. Tooling: `.golangci.yml` (`funlen`,
  `lll`) + `scripts/check_file_sizes.sh`.
- TAXONOMY.md defining the canonical vocabulary; CLAUDE.md and AGENTS.md at
  repo root.
- Threat model section in README.
- K-36: quorum-signed recipient list. Optional
  `secrets/recipients.signed.toml` declares M-of-N operators; when
  present, `thimble recipient add` enforces a three-phase prepare /
  sign-add / commit protocol so a single compromised maintainer can
  no longer escrow plaintext access via a one-line manifest diff. New
  CLI surface: `recipient sign-add`, `recipient add --bootstrap`,
  `recipient list`. Protocol spec at `docs/recipient-quorum.md`.
- K-37: `recipient remove --rotate` flow. Each value in a namespace
  now has an `origin` label (`provision`, `and-set`, or `set`)
  recorded in a sibling `<env>.origins.json` plaintext file. Running
  `thimble recipient remove --rotate <app> <env> age1...` regenerates
  every `provision`-origin value atomically alongside the recipient
  drop, and surfaces every other key as "manual rotate needed" so the
  operator knows what still needs out-of-band attention.
  `--rotate-randoms-only` is the silent variant for scripts. Hidden
  `--origin <source>` flag on `set`/`create`/`update` lets the
  upstream pipeline tag the source of a value (default `set`);
  `and-set` always tags `and-set` automatically. Recommended
  pipeline: `thimble provision | thimble set --origin=provision
  <app> <env> KEY` so the value is recorded as auto-rotatable. The
  rotation is atomic: the manifest, bundle, and `.origins.json`
  either all advance or all roll back to the pre-rotation state.

## [0.1.0] — pending

Initial public-ready slice. The runtime hardening from Waves 4–6 (age binary
pinning, identity-mode checks, manifest version + flock, web cookie auth,
host-header allowlist, …) is included before this tag is cut.

### Added

- File-first secrets manager for `<application>/<environment>` namespaces.
- `age`-backed encryption with recipient-list metadata.
- CLI: `init`, `set`, `create`, `update`, `delete`, `list`, `render`,
  `provision`, `and-set`, `and-get`, `recipient add/remove`, `web`.
- Web UI on loopback with token authentication.
- Cross-platform release tarballs via GitHub Actions.

### Security

- Encryption delegated to `age`; no custom cryptography.
- Atomic writes for manifest and bundles.
- Restrictive file modes (0600 files, 0700 dirs).
- Secret values rejected as command arguments; masked-prompt or pipe entry only.

[Unreleased]: https://github.com/cartine/thimble/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/cartine/thimble/releases/tag/v0.1.0
