export GOFLAGS ?= -buildvcs=false

.PHONY: help build test test-e2e test-integration test-irc test-ubuntu-docker verify-no-ui-leak fmt setup-aps-wsm \
  12fcc-record 12fcc-grade 12fcc-badge

help:
	@echo "Targets:"
	@echo "  build               Build default CLI to bin/wsm and compile all packages"
	@echo "  test                Run unit + default E2E tests"
	@echo "  test-e2e            Run default E2E tests only"
	@echo "  test-integration    Run integration_wsm-tagged tests (cross-tool e2e: rux/kit)"
	@echo "  test-irc            Run integration_irc-tagged tests against a real ergo (ERGO_BIN)"
	@echo "  test-ubuntu-docker  Run full test matrix inside Ubuntu Docker image"
	@echo "  verify-no-ui-leak   Ensure pkg/* does not depend on charmbracelet"
	@echo "  fmt                 Run gofmt on Go files"
	@echo "  setup-aps-wsm       Set up APS + WSM integration (builds both)"

build:
	@mkdir -p .tmp/gocache bin
	GOCACHE=$$(pwd)/.tmp/gocache go build -o bin/wsm ./cmd/wsm
	GOCACHE=$$(pwd)/.tmp/gocache go build ./...

test:
	@mkdir -p .tmp/gocache
	GOCACHE=$$(pwd)/.tmp/gocache go test ./...

test-e2e:
	@mkdir -p .tmp/gocache
	GOCACHE=$$(pwd)/.tmp/gocache go test ./tests/e2e -count=1

test-integration:
	@mkdir -p .tmp/gocache
	GOCACHE=$$(pwd)/.tmp/gocache go test -tags integration_wsm ./... -count=1

test-irc:
	@mkdir -p .tmp/gocache
	GOCACHE=$$(pwd)/.tmp/gocache go test -tags integration_irc ./... -count=1

test-ubuntu-docker:
	./scripts/run-tests-ubuntu-docker.sh

verify-no-ui-leak:
	@mkdir -p .tmp/gocache
	@if [ -d pkg ] && find pkg -name '*.go' | grep -q .; then \
		deps="$$(GOCACHE=$$(pwd)/.tmp/gocache go list -deps ./pkg/...)" || exit 1; \
		leaked="$$(echo "$$deps" | grep -E 'charmbracelet|charm\.land' || true)"; \
		if [ -n "$$leaked" ]; then \
			echo 'error: terminal-UI dependency leaked into pkg/*'; \
			echo "$$leaked" | sed 's/^/  /'; \
			exit 1; \
		fi; \
		echo 'ok: pkg/* is free of terminal-UI dependencies'; \
	else \
		echo 'pkg/* has no Go files yet; dependency boundary check skipped'; \
	fi

fmt:
	@if find . -name '*.go' | grep -q .; then gofmt -w $$(find . -name '*.go'); else echo 'no Go files to format'; fi

setup-aps-wsm: ## Set up APS + WSM integration
	@echo "Setting up APS + WSM integration..."
	@bash scripts/setup-aps-wsm.sh

# Conformance grading (12fcc service tier). KIT_BIN must point at a
# kit binary built from hop.top/kit cmd/kit.
12fcc-record: ## Re-record conformance cassettes from real binary runs
	@./scripts/12fcc-record.sh

12fcc-grade: ## Grade recorded cassettes (STRICT=1 to fail on fail verdicts)
	@./scripts/12fcc-grade.sh

12fcc-badge: ## Regenerate .12fc.json from verify leaves + graded verdicts
	@./scripts/12fcc-badge.sh
