# syntax=docker/dockerfile:1.7

# Multi-arch image for Thimble. Two stages:
#   1. builder: compiles thimble for $TARGETARCH and downloads the pinned
#      `age` binary, verifying its SHA-256 against the value below.
#   2. runtime: distroless static + thimble + age + age-keygen.
#
# Build flags (-trimpath, -ldflags="-s -w") match the GitHub release
# workflow so that a `docker build` and a release tarball produce a
# byte-for-byte identical thimble binary.

FROM --platform=$BUILDPLATFORM golang:1.25-alpine@sha256:5caaf1cca9dc351e13deafbc3879fd4754801acba8653fa9540cea125d01a71f AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_DATE=unknown
ARG COMMIT=unknown

RUN apk add --no-cache ca-certificates curl tar

WORKDIR /src

# Cache deps before copying the rest of the source tree.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH

RUN set -eux; \
    ldflags="-s -w"; \
    ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.version=${VERSION}"; \
    ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.commit=${COMMIT}"; \
    ldflags="$ldflags -X github.com/cartine/thimble/internal/cli.buildDate=${BUILD_DATE}"; \
    go build -trimpath -ldflags="$ldflags" -o /out/thimble ./cmd/thimble

# ----------------------------------------------------------------------
# Pin the `age` binary. Trust anchor: AGE_VERSION + AGE_SHA256_*. Update
# both together when bumping. URL is the canonical FiloSottile/age
# release tarball; SHAs are copied verbatim from the release page.
#
# Source: https://github.com/FiloSottile/age/releases/tag/v1.3.1
#   age-v1.3.1-linux-amd64.tar.gz  bdc69c09cbdd6cf8b1f333d372a1f58247b3a33146406333e30c0f26e8f51377
#   age-v1.3.1-linux-arm64.tar.gz  c6878a324421b69e3e20b00ba17c04bc5c6dab0030cfe55bf8f68fa8d9e9093a
# ----------------------------------------------------------------------
ARG AGE_VERSION=v1.3.1
ARG AGE_SHA256_AMD64=bdc69c09cbdd6cf8b1f333d372a1f58247b3a33146406333e30c0f26e8f51377
ARG AGE_SHA256_ARM64=c6878a324421b69e3e20b00ba17c04bc5c6dab0030cfe55bf8f68fa8d9e9093a

RUN set -eux; \
    case "$TARGETARCH" in \
      amd64) age_sha="$AGE_SHA256_AMD64" ;; \
      arm64) age_sha="$AGE_SHA256_ARM64" ;; \
      *) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    name="age-${AGE_VERSION}-linux-${TARGETARCH}.tar.gz"; \
    url="https://github.com/FiloSottile/age/releases/download/${AGE_VERSION}/${name}"; \
    curl -fsSL -o "/tmp/${name}" "$url"; \
    echo "${age_sha}  /tmp/${name}" | sha256sum -c -; \
    mkdir -p /tmp/age-extract; \
    tar -xzf "/tmp/${name}" -C /tmp/age-extract; \
    install -m 0755 /tmp/age-extract/age/age /out/age; \
    install -m 0755 /tmp/age-extract/age/age-keygen /out/age-keygen; \
    rm -rf /tmp/age-extract "/tmp/${name}"

# ----------------------------------------------------------------------
# Runtime stage: distroless static-nonroot, pinned by digest.
# Source: gcr.io/distroless/static:nonroot
# Index digest captured at the time this Dockerfile was authored. Bump
# the digest with the same care as a third-party dep.
# ----------------------------------------------------------------------
FROM gcr.io/distroless/static:nonroot@sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39

COPY --from=builder /out/thimble /usr/local/bin/thimble
COPY --from=builder /out/age /usr/local/bin/age
COPY --from=builder /out/age-keygen /usr/local/bin/age-keygen

ENTRYPOINT ["/usr/local/bin/thimble"]
