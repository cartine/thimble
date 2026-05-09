# K-14 — PR CI: govulncheck

- Wave / Step: 3.2
- Effort: S
- Risk: low
- Deps: K-13
- Files: .github/workflows/ci.yml

## Goal

Catch vulnerable stdlib/dep combinations on every PR. Cheap, low-noise.

## Acceptance

- CI workflow installs `govulncheck` and runs `govulncheck ./...`.
- Job fails on any reported vulnerability.
- README badges (K-10) include a vuln-status badge if a hosted reporter is
  used; otherwise just rely on the CI gate.

## Notes

`govulncheck` is conservative — only reports actually-reachable
vulnerabilities, so failures are signal not noise.
