# K-32 — `Cache-Control: no-store` on web responses

- Wave / Step: 5.3
- Effort: S
- Risk: low
- Deps: K-12
- Files: internal/web/

## Goal

Stop browsers from caching pages that contain key listings, recipient
addresses, or token-bearing redirect URLs. Cheap defense against the
"someone walks up to my unlocked laptop" case.

## Acceptance

- Every response from `internal/web` carries
  `Cache-Control: no-store, no-cache, must-revalidate` and `Pragma: no-cache`.
- Verified by integration test (existing
  `TestWebUIRequiresTokenAndRedactsValues` extended).

## Notes

Trivial middleware. Land it the same PR as K-31 if convenient.
