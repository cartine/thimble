# K-13 — PR CI: go vet + go test

- Wave / Step: 3.1
- Effort: S
- Risk: low
- Deps: K-12
- Files: .github/workflows/ci.yml (new)

## Goal

Today there is *no* CI on PRs — only the release workflow. Add a baseline
gate that runs on every push and pull request.

## Acceptance

- `.github/workflows/ci.yml` runs on `push` and `pull_request`.
- Steps: `actions/checkout@v4`, `actions/setup-go@v5` with `go-version-file`,
  `go vet ./...`, `go test ./...` (race + coverage flags).
- Workflow runs on Ubuntu and macOS at least.
- Branch protection on `main` requires this workflow to pass.

## Notes

Coverage output can stay simple (text in job log) until K-15 adds richer
reporting.
