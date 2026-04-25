export GOFLAGS ?= -buildvcs=false

.PHONY: help build test test-e2e test-ubuntu-docker verify-no-ui-leak fmt setup-aps-wsm

help:
	@echo "Targets:"
	@echo "  build               Build default CLI to bin/wsm and compile all packages"
	@echo "  test                Run unit + default E2E tests"
	@echo "  test-e2e            Run default E2E tests only"
	@echo "  test-ubuntu-docker  Run full test matrix inside Ubuntu Docker image"
	@echo "  verify-no-ui-leak   Ensure pkg/* does not depend on charmbracelet (legacy)"
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

test-ubuntu-docker:
	./scripts/run-tests-ubuntu-docker.sh

verify-no-ui-leak:
	@mkdir -p .tmp/gocache
	@if [ -d pkg ] && find pkg -name '*.go' | grep -q .; then \
		deps="$$(GOCACHE=$$(pwd)/.tmp/gocache go list -deps ./pkg/...)" || exit 1; \
		echo "$$deps" | grep -q 'charmbracelet' && \
		(echo 'error: charmbracelet dependency leaked into pkg/*' && exit 1) || \
		echo 'ok: pkg/* is free of charmbracelet dependencies'; \
	else \
		echo 'pkg/* has no Go files yet; dependency boundary check skipped'; \
	fi

fmt:
	@if find . -name '*.go' | grep -q .; then gofmt -w $$(find . -name '*.go'); else echo 'no Go files to format'; fi

setup-aps-wsm: ## Set up APS + WSM integration
	@echo "Setting up APS + WSM integration..."
	@bash scripts/setup-aps-wsm.sh
