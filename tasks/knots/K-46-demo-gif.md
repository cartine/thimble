# K-46 — Demo GIF / asciinema in README

- Wave / Step: 8.1
- Effort: S
- Risk: low
- Deps: K-04, K-05, K-09
- Files: assets/demo.cast or assets/demo.gif, README.md

## Goal

A 30-second demo showing init → recipient add → set → render moves adoption
more than docs do. Demos answer "what is this?" in seconds.

## Acceptance

- Either an `asciinema` `.cast` file embedded via the asciinema-player
  shortcode, or a GIF (≤2 MB) under `assets/`.
- README's first section after the logo embeds the demo.
- The recording uses sample names that match the README's "Real-Life Flow"
  so a viewer can connect demo to docs.
- No real recipients or identities visible in the recording; use throwaway
  ones generated for the recording.

## Notes

Keep it short. A 30-second demo gets watched; a 3-minute one does not.
