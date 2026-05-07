# K-45 — Distribution: Dockerfile + GHCR image

- Wave / Step: 7.8
- Effort: M
- Risk: low
- Deps: K-40
- Files: Dockerfile, .github/workflows/release.yml

## Goal

A minimal multi-arch image so Thimble can run inside CI/CD pipelines and
ephemeral container environments without an install step.

## Acceptance

- `Dockerfile` is multi-stage: build from `golang:1.25` with `-trimpath`,
  copy a static binary and a pinned `age` binary into a `scratch` (or
  `gcr.io/distroless/static`) final stage.
- Release workflow builds `linux/amd64` and `linux/arm64`, pushes to
  `ghcr.io/cartine/thimble:vX.Y.Z` and `:latest`.
- Image is signed via cosign keyless (K-40 toolchain).
- README "Install" gains: `docker run --rm -v $PWD:/work
  ghcr.io/cartine/thimble:latest list <app> <env>`.

## Notes

Bake the `age` binary into the image, with a verified SHA. This is the one
distribution channel where K-18's pinning story can be made hard guarantee.
