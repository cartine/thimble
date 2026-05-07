# K-15 — PR CI: staticcheck + gosec

- Wave / Step: 3.3
- Effort: S
- Risk: low
- Deps: K-13
- Files: .github/workflows/ci.yml, .staticcheck.conf, .gosec.yaml (optional)

## Goal

Layer static analysis on top of `go vet`. `staticcheck` catches Go-idiomatic
bugs; `gosec` catches security smells (path traversal, weak RNG, unsafe
shell, hardcoded creds).

## Acceptance

- CI runs `staticcheck ./...` and `gosec ./...` and fails on any finding.
- Initial pass may need an allowlist (e.g. `gosec` may flag the `exec.Command`
  for `age` shelling — document and exclude with a comment justifying).
- Each suppression has a comment naming the reason and a knot ID if a fix is
  planned.

## Notes

Resist the urge to broadly suppress; prefer fixing the underlying smell.
