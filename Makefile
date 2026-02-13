export GOFLAGS ?= -buildvcs=false

.PHONY: help build build-charm test test-e2e test-e2e-charm test-ubuntu-docker verify-no-ui-leak fmt

help:
	@echo "Targets:"
	@echo "  build               Build default CLI to bin/wsm and compile all packages"
	@echo "  build-charm         Build charmui CLI to bin/wsm-charm"
	@echo "  test                Run unit + default E2E tests"
	@echo "  test-e2e            Run default E2E tests only"
	@echo "  test-e2e-charm      Run charm-tagged E2E tests (TUI + charm UI path)"
	@echo "  test-ubuntu-docker  Run full test matrix inside Ubuntu Docker image"
	@echo "  verify-no-ui-leak   Ensure pkg/* does not depend on charmbracelet"
	@echo "  fmt                 Run gofmt on Go files"

build:
	@mkdir -p .tmp/gocache bin
	GOCACHE=$$(pwd)/.tmp/gocache go build -o bin/wsm ./cmd/wsm
	GOCACHE=$$(pwd)/.tmp/gocache go build ./...

build-charm:
	@mkdir -p .tmp/gocache bin
	GOCACHE=$$(pwd)/.tmp/gocache go build -tags charmui -modfile=go.charm.mod -o bin/wsm-charm ./cmd/wsm

test:
	@mkdir -p .tmp/gocache
	GOCACHE=$$(pwd)/.tmp/gocache go test ./...

test-e2e:
	@mkdir -p .tmp/gocache
	GOCACHE=$$(pwd)/.tmp/gocache go test ./tests/e2e -count=1

test-e2e-charm:
	@mkdir -p .tmp/gocache
	GOCACHE=$$(pwd)/.tmp/gocache go test -tags charmui -modfile=go.charm.mod ./tests/e2e -run 'TestTUI|TestUIModeSelection_CharmBuild' -count=1

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
