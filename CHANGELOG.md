# Changelog

All notable changes to this project are documented here. The format is based
on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- 48-knot hardening rollout tracked in [tasks/knot-plan.md](tasks/knot-plan.md)
  via the [`kno`](https://github.com/cartine/knots) execution-plan tooling.
- LICENSE (Apache-2.0) at repo root.
- Real disclosure policy in SECURITY.md (GitHub Private Vulnerability Reporting
  + `security@cartine.me`); previous internal review notes preserved at
  `docs/security-review.md`.
- CONTRIBUTING.md with repo layout, coding standards, PR workflow.
- Source-code size standard enforced by `make lint`: <500 lines/file,
  <100 lines/function, <100 columns/line. Tooling: `.golangci.yml` (`funlen`,
  `lll`) + `scripts/check_file_sizes.sh`.
- TAXONOMY.md defining the canonical vocabulary; CLAUDE.md and AGENTS.md at
  repo root.
- Threat model section in README.

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
