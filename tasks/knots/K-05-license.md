# K-05 — Add LICENSE

- Wave / Step: 1.1
- Effort: S
- Risk: low
- Deps: —
- Files: LICENSE (new)

## Goal

Add an OSI-approved LICENSE at the repo root. Without one, any policy-aware
org will reject Thimble at procurement-review stage regardless of code
quality. Highest-leverage trust signal in the entire plan.

## Acceptance

- `LICENSE` exists at repo root containing full Apache-2.0 or MIT text.
- README "Install" section gains a one-line "Licensed under …" footer.
- `go.mod` / `cmd/thimble/main.go` retain no license-conflicting dependencies
  (current deps are stdlib + `golang.org/x/term` — BSD; compatible with both).

## Notes

Default recommendation: Apache-2.0. It includes a patent grant, which matters
for anything that touches secrets handling or crypto wrapping.
