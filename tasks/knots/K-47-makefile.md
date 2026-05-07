# K-47 — Makefile targets

- Wave / Step: 8.2
- Effort: S
- Risk: low
- Deps: K-12, K-13, K-14, K-15, K-16
- Files: Makefile (new)

## Goal

A `Makefile` with the standard set of targets means "every contributor runs
the same checks I run." Reproducibility is a trust signal.

## Acceptance

Targets:
- `make build` — local cross-platform builds.
- `make test` — `go test ./...` race + coverage.
- `make integration` — runs `//go:build integration` jobs (K-16) against
  real `age`.
- `make lint` — `go vet`, `staticcheck`, `gosec`.
- `make vuln` — `govulncheck`.
- `make verify-release VERSION=…` — calls K-41's verify script.
- `make help` — prints target descriptions.

Acceptance:
- All targets work on a clean checkout.
- CI workflows from Wave 3 invoke `make` targets directly so local and CI
  stay in sync.

## Notes

CLAUDE.md prefers build targets over ad-hoc commands. This is the knot
that operationalizes that.
