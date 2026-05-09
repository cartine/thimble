# K-42 — Distribution: Homebrew tap

- Wave / Step: 7.5
- Effort: M
- Risk: low
- Deps: K-40
- Files: separate tap repo (`homebrew-thimble`), .github/workflows/release.yml

## Goal

`brew install cartine/thimble/thimble`. Most macOS operators install
everything via brew; not having it here means most people fall back to
`curl|sh`, which is the path we want them off.

## Acceptance

- A `homebrew-thimble` repo exists with a `Formula/thimble.rb`.
- Release workflow updates the formula on each release with the new version
  and SHA-256s for darwin/amd64 and darwin/arm64.
- `brew install cartine/thimble/thimble` works on a clean macOS image and
  produces a binary that passes `thimble --version`.
- README "Install" section adds the brew one-liner above the curl one.

## Notes

Doable as a tap from day one. Homebrew core requires a track record we
don't have yet — revisit in 6 months.
