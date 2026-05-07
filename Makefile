# Thimble Makefile
#
# Minimal targets land in K-01. Wider target set lands in K-47
# (build, test, integration, vuln, release verification, demo).

.PHONY: lint

lint: ## Run Go linter and source-size checker.
	@command -v golangci-lint >/dev/null 2>&1 || { \
	  echo "golangci-lint not found; install from https://golangci-lint.run"; \
	  exit 1; \
	}
	golangci-lint run
	bash scripts/check_file_sizes.sh
