# K-43 — Distribution: Scoop bucket

- Wave / Step: 7.6
- Effort: M
- Risk: low
- Deps: K-40
- Files: separate bucket repo (`scoop-thimble`), .github/workflows/release.yml

## Goal

`scoop install thimble` for Windows operators. Required to support Windows
without falling back to a hand-edited `install.ps1`.

## Acceptance

- A `scoop-thimble` repo exists with a `bucket/thimble.json` manifest.
- Release workflow now also builds Windows binaries (`windows/amd64`,
  `windows/arm64`) and updates the manifest.
- `scoop bucket add thimble https://github.com/cartine/scoop-thimble &&
  scoop install thimble` works on a clean Windows image.

## Notes

This requires the release matrix to add Windows targets — which is fine,
the code has no Unix-only assumptions except possibly the file mode checks
(K-19 already needs to handle Windows). Validate during the K-19 work.
