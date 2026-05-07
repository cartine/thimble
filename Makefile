# Thimble Makefile
#
# Minimal targets land in K-01. Wider target set lands in K-47
# (build, test, integration, vuln, release verification, demo).

.PHONY: lint integration verify-release demo

lint: ## Run Go linter and source-size checker.
	@command -v golangci-lint >/dev/null 2>&1 || { \
	  echo "golangci-lint not found; install from https://golangci-lint.run"; \
	  exit 1; \
	}
	golangci-lint run
	bash scripts/check_file_sizes.sh

integration: ## Run integration tests against the real `age` binary.
	@command -v age >/dev/null 2>&1 || { \
	  echo "age not found; install from https://github.com/FiloSottile/age"; \
	  exit 1; \
	}
	@command -v age-keygen >/dev/null 2>&1 || { \
	  echo "age-keygen not found; install from https://github.com/FiloSottile/age"; \
	  exit 1; \
	}
	go test -tags integration -timeout 60s ./...

verify-release: ## Reproduce a published release and diff SHA-256s. Usage: make verify-release VERSION=vX.Y.Z
	@if [ -z "$(VERSION)" ]; then \
	  echo "usage: make verify-release VERSION=vX.Y.Z"; \
	  exit 2; \
	fi
	bash scripts/verify-release.sh "$(VERSION)"

demo: ## Print instructions for recording assets/demo.cast.
	@echo "Run: asciinema rec assets/demo.cast bash scripts/demo.sh"
	@echo "Then commit assets/demo.cast to capture the recording."
