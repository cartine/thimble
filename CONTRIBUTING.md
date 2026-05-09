# Contributing to Thimble

Thanks for the interest. Thimble is intentionally small; PRs should keep it
that way.

## Quick start

```sh
git clone https://github.com/cartine/thimble
cd thimble
make build
make test
```

`make help` lists every target. You also need `age` on `PATH` to run
anything beyond unit tests.

### Run the binary you just built

`make build` lands the binary at `./thimble` (relative to the repo
root). For pre-release dogfooding — running `thimble` from any
directory, recording the demo cast, exercising real namespaces — use
`make install-local`:

```sh
make install-local        # builds + installs to ~/.local/bin/thimble
thimble --version         # confirm $PATH picks it up
make uninstall-local      # remove the local install when done
```

This mirrors the same pattern knots and foolery use; `~/.local/bin` is
already on most operators' `PATH`. Override the destination with
`make install-local INSTALL_DIR=/some/other/dir`.

## Setup verification

Run `thimble doctor` after install to confirm your environment is sane. It
checks the resolved `age` binary path/version/SHA-256, the optional
`THIMBLE_AGE_SHA256` pin, the identity file (presence and 0600 mode), the
secrets store directory (presence, 0700 mode, writeability), the manifest
(parseable, all bundles present), per-namespace bundle SHA-256 (matches the
manifest's `bundle_sha256`, K-22), the default web port `127.0.0.1:8787`, and
the recipient list per namespace (count, type prefix, opaque thumbprint).
Non-zero exit if anything fails. `--json` emits machine-readable output for
scripts.

## Repository layout

```
cmd/thimble/        # CLI entrypoint (split into internal/* in K-12)
scripts/            # install/update scripts, lint helpers
docs/               # internal review notes, design history
tasks/              # the kno-managed execution plan and per-knot drafts
.github/workflows/  # release pipeline (CI lands in Wave 4)
```

After [K-12](tasks/knots/K-12-split-main-go.md) lands, code moves into
`internal/store/`, `internal/age/`, `internal/web/`, and friends.

## Vocabulary

[TAXONOMY.md](TAXONOMY.md) defines the canonical terms (`application`,
`environment`, `namespace`, `recipient`, `identity`, `bundle`, …). Read it
before introducing or renaming a domain concept. Refresh it via
`/taxonomize` after large changes.

## Coding standards

| Metric | Limit |
|--------|-------|
| File length | < 500 lines |
| Function/method body | < 100 lines |
| Line width | < 100 columns |

Run `make lint` before committing. The standard is enforced by
`.golangci.yml` (`funlen`, `lll`) and `scripts/check_file_sizes.sh`.

## Tests

```sh
make test          # unit tests with the race detector
make integration   # tests against a real `age` binary
make lint          # golangci-lint + source-size checker
make vuln          # govulncheck against known CVEs
```

`make help` prints every target with a one-line description.

## Running multi-leader

When you contribute to a feature that touches replication, concurrency,
or transport assumptions, the model to keep in mind is the one
documented in the README's
[Storing and Syncing](README.md#storing-and-syncing) section. The short
version: bundles are encrypted files, transport is interchangeable
(rsync over ssh against a store host is the recommended default), and
concurrent writes are made safe by the K-21 manifest version + flock and
the K-27 append-only audit log. New behaviour should respect those
invariants — Thimble has no always-on server and is not gaining one.

## Pull requests

- One logical change per PR. Refactors and behavior changes go in
  separate commits.
- Reference the relevant `K-NN` knot in the commit subject if the change
  is part of the active rollout (`kno ls` shows the live list).
- All required CI checks must pass — these come online with Wave 4
  (`go vet`, `go test`, `govulncheck`, `staticcheck`, `gosec`).

## Security-sensitive changes

If your PR touches encryption, recipient handling, the web token, the
install script, or the release pipeline, please:

- Read [docs/security-review.md](docs/security-review.md) and
  [SECURITY.md](SECURITY.md).
- Add a "Security impact" line to the PR description naming what changes
  in the threat model.
- Consider whether your change deserves a `risk-high` tag on the
  associated knot.

## Releases

Cut a release with `make tag-release VERSION=patch` (or `minor`,
`major`, or an explicit `vX.Y.Z`). The target wraps
[`scripts/tag-release.sh`](scripts/tag-release.sh): it bumps the
version, rewrites the `[Unreleased]` block in `CHANGELOG.md`, tags
the commit, pushes, watches the release workflow with
`gh run watch`, then verifies each artifact's SHA-256 against the
published `checksums.txt` and runs `gh attestation verify` (K-40).
Pass `DRY_RUN=1` to see the full plan without side effects.

The same flow is also available as the `/release` agent skill — see
[.claude/skills/release/SKILL.md](.claude/skills/release/SKILL.md).

## Reporting bugs

Public bugs: open a GitHub issue.
Security issues: see [SECURITY.md](SECURITY.md) — please don't file public
issues for security findings.

## Code of conduct

Be kind. Disagree about technical decisions, not about people.
