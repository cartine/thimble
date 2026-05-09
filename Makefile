# Thimble Makefile
#
# Targets are kept POSIX-portable so they work under BSD make on macOS as
# well as GNU make on Linux. The `## ` comments after the colon are parsed
# by `make help` to print target descriptions.
#
# Run `make help` for the full list.

GO        ?= go
GOOS      ?=
GOARCH    ?=
LDFLAGS   ?= -s -w
BUILDARGS ?= -trimpath -ldflags="$(LDFLAGS)"

.PHONY: help build test integration lint vuln verify-release demo tag-release

help: ## List targets and short descriptions.
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / { \
	  printf "  \033[1m%-18s\033[0m %s\n", $$1, $$2 \
	}' Makefile

build: ## Build thimble for the host (override GOOS/GOARCH to cross-compile).
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(BUILDARGS) ./cmd/thimble

test: ## Run unit tests with the race detector.
	$(GO) test -race ./...

integration: ## Run integration tests against the real `age` binary.
	@command -v age >/dev/null 2>&1 || { \
	  echo "age not found; install from https://github.com/FiloSottile/age"; \
	  exit 1; \
	}
	@command -v age-keygen >/dev/null 2>&1 || { \
	  echo "age-keygen not found; install from https://github.com/FiloSottile/age"; \
	  exit 1; \
	}
	$(GO) test -tags integration -timeout 60s ./...

lint: ## Run golangci-lint and the source-size checker.
	@# Install golangci-lint v2 with the active toolchain so it can lint a
	@# matching-version Go target. v1 binaries built against older Go can
	@# refuse to run when the project's toolchain is newer.
	@command -v golangci-lint >/dev/null 2>&1 || \
	  $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	@version=$$(golangci-lint version --short 2>/dev/null || echo 0.0.0); \
	  major=$${version%%.*}; \
	  if [ "$$major" -lt 2 ] 2>/dev/null; then \
	    $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; \
	  fi
	golangci-lint run
	bash scripts/check_file_sizes.sh

vuln: ## Install govulncheck (if missing) and scan for known CVEs.
	@command -v govulncheck >/dev/null 2>&1 || \
	  $(GO) install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

verify-release: ## Reproduce a published release and diff SHA-256s. Usage: make verify-release VERSION=vX.Y.Z
	@if [ -z "$(VERSION)" ]; then \
	  echo "usage: make verify-release VERSION=vX.Y.Z"; \
	  exit 2; \
	fi
	bash scripts/verify-release.sh "$(VERSION)"

demo: ## Print instructions for recording assets/demo.cast.
	@echo 'Run: asciinema rec -c "bash scripts/demo.sh" assets/demo.cast'
	@echo "Then commit assets/demo.cast to capture the recording."

tag-release: ## Bump version, tag, push, watch release. Usage: make tag-release VERSION=patch|minor|major|vX.Y.Z [DRY_RUN=1]
	@if [ -z "$(VERSION)" ]; then \
	  echo "usage: make tag-release VERSION=patch|minor|major|vX.Y.Z [DRY_RUN=1]"; \
	  exit 2; \
	fi
	@if [ -n "$(DRY_RUN)" ]; then \
	  bash scripts/tag-release.sh "$(VERSION)" --dry-run; \
	else \
	  bash scripts/tag-release.sh "$(VERSION)"; \
	fi
