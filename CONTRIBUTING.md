# Contributing to Thimble

Thanks for the interest. Thimble is intentionally small; PRs should keep it
that way.

## Quick start

```sh
git clone https://github.com/cartine/thimble
cd thimble
go build ./cmd/thimble
go test ./...
```

You also need `age` on `PATH` to run anything beyond unit tests.

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
go test ./...
```

Integration tests against a real `age` binary land with
[K-16](tasks/knots/K-16-real-age-integration-test.md). Once they exist,
run `make integration` for the full surface.

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

Cut by maintainers via [K-48](tasks/knots/K-48-tag-release-automation.md)'s
`make tag-release VERSION=…` once that target lands. Until then, releases
are tagged manually and the existing GitHub Actions workflow at
[.github/workflows/release.yml](.github/workflows/release.yml) builds the
artifacts.

## Reporting bugs

Public bugs: open a GitHub issue.
Security issues: see [SECURITY.md](SECURITY.md) — please don't file public
issues for security findings.

## Code of conduct

Be kind. Disagree about technical decisions, not about people.
