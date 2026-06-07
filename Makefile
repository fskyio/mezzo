.DEFAULT_GOAL := build

APP := mezzo
CMD := ./cmd/$(APP)

GO ?= go
GOFLAGS ?=
ARGS ?=

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X main.version=$(VERSION)
BUILD_FLAGS ?= -trimpath -ldflags "$(LDFLAGS)"

GORELEASER_VERSION ?= latest
GORELEASER ?= $(GO) run github.com/goreleaser/goreleaser/v2@$(GORELEASER_VERSION)

.PHONY: help all build run test test-race lint vet fmt fmt-check tidy generate check clean snapshot release

help:
	@printf '%s\n' \
		'Targets:' \
		'  make build      Build ./mezzo' \
		'  make run        Run the app with go run (override ARGS=...)' \
		'  make test       Run unit tests' \
		'  make test-race  Run tests with the race detector' \
		'  make lint       Run formatting check and go vet' \
		'  make fmt        Format Go files' \
		'  make tidy       Tidy go.mod/go.sum' \
		'  make generate   Run go generate' \
		'  make snapshot   Build a local GoReleaser snapshot' \
		'  make release    Publish a GoReleaser release' \
		'  make clean      Remove build output' \
		'  make check      Run lint, tests, and build' \
		'' \
		'Examples:' \
		'  make run ARGS="-verbose"' \
		'  make build VERSION=canary'

all: check

build:
	$(GO) build $(GOFLAGS) $(BUILD_FLAGS) -o $(APP) $(CMD)

run:
	$(GO) run $(GOFLAGS) $(CMD) $(ARGS)

test:
	$(GO) test $(GOFLAGS) ./...

test-race:
	$(GO) test $(GOFLAGS) -race ./...

lint: fmt-check vet

vet:
	$(GO) vet $(GOFLAGS) ./...

fmt:
	$(GO) fmt $(GOFLAGS) ./...

fmt-check:
	@test -z "$$($(GO)fmt -l .)" || { \
		echo 'Go files need formatting:'; \
		$(GO)fmt -l .; \
		exit 1; \
	}

tidy:
	$(GO) mod tidy

generate:
	$(GO) generate $(GOFLAGS) ./...

check: lint test build

snapshot:
	$(GORELEASER) release --snapshot --clean

release:
	$(GORELEASER) release --clean

clean:
	rm -rf $(APP) dist coverage.out
