# K-21 — Manifest version + flock (TOCTOU fix)

- Wave / Step: 4.4
- Effort: M
- Risk: med
- Deps: K-12
- Files: internal/store/

## Goal

`rewriteEnv` does load → decrypt → mutate → encrypt → save with no flock or
version-check on save. Two operators editing different keys in the same
namespace concurrently silently lose one set of edits.

## Acceptance

- Each `envManifest` carries a monotonic `Version` (or per-namespace ETag).
- `saveManifest` reads the on-disk manifest under an exclusive `flock` of
  `secrets/thimble.json`, checks the version matches the one this command
  loaded, and refuses with a clear "another writer changed
  <app>/<env>; rerun" if not.
- Reads (`list`, `render`) take a shared lock.
- Tests cover: two goroutines mutating different keys → second one fails
  cleanly; same key → second one fails cleanly; lock released on panic.

## Notes

`flock` semantics on macOS/Linux are slightly different but `golang.org/x/sys`
covers both. Don't bring in a third-party lock library — keep deps minimal.
