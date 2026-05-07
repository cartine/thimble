# K-44 — Distribution: Debian (.deb) repo

- Wave / Step: 7.7
- Effort: L
- Risk: low
- Deps: K-40
- Files: .github/workflows/release.yml, scripts/build-deb.sh

## Goal

Many deploy hosts are Debian/Ubuntu. `apt-get install thimble` from a
hosted apt repo (Cloudsmith, GitHub Pages with `apt-ftparchive`, or
PackageCloud) is the cleanest install path for those hosts.

## Acceptance

- Release workflow produces signed `.deb` packages for amd64 and arm64.
- An apt-compatible index is hosted (recommend GitHub Pages +
  `apt-ftparchive` for free).
- README "Install" gains a Debian section: `curl … | sudo apt-key add -`
  (or signed-by trusted.gpg.d), `echo "deb …" | sudo tee
  /etc/apt/sources.list.d/thimble.list`, `sudo apt-get install thimble`.
- The `.deb` post-install script does *not* generate identities; it only
  installs the binary.

## Notes

This is the heaviest distribution channel; consider it L not M because of
the GPG signing key management story. Land it last in Wave 7 if at all.
