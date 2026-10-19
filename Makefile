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

# Install locations, following the GNU conventions. Packagers set DESTDIR and
# PREFIX (for example DESTDIR=pkg PREFIX=/usr).
DESTDIR ?=
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share
MANDIR ?= $(DATADIR)/man
DOCDIR ?= $(DATADIR)/doc/$(APP)
UNITDIR ?= $(PREFIX)/lib/systemd/system
INSTALL ?= install

# The man page files in man/. The suffix is the manual section.
MAN_PAGES := mezzo.1 mezzo-config.5

.PHONY: help all build run test test-race lint vet fmt fmt-check tidy generate check clean snapshot release
.PHONY: install install-bin install-man install-doc install-systemd uninstall

help:
	@printf '%s\n' \
		'Targets:' \
		'  make build      Build ./mezzo' \
		'  make install    Install the binary, man pages and docs (run make build first)' \
		'  make uninstall  Remove what make install and make install-systemd installed' \
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
		'  make build VERSION=canary' \
		'  make build && sudo make install' \
		'  make install PREFIX=/usr DESTDIR=/tmp/pkg' \
		'  make install-bin install-man PREFIX=$$HOME/.local' \
		'  sudo make install install-systemd   (also install the systemd unit)'

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

# make install does not build. A rebuild under sudo or fakeroot can miss the Go
# toolchain, and Git may refuse to describe the checkout, so the binary would
# be built with the wrong version. Run make build first, as a normal user.
install: install-bin install-man install-doc

install-bin:
	@test -f $(APP) || { echo "$(APP) has not been built; run 'make build' first" >&2; exit 1; }
	$(INSTALL) -d "$(DESTDIR)$(BINDIR)"
	$(INSTALL) -m 755 $(APP) "$(DESTDIR)$(BINDIR)/$(APP)"

install-man:
	@for page in $(MAN_PAGES); do \
		dir="$(DESTDIR)$(MANDIR)/man$${page##*.}"; \
		$(INSTALL) -d "$$dir" && $(INSTALL) -m 644 "man/$$page" "$$dir/$$page" || exit 1; \
	done

install-doc:
	$(INSTALL) -d "$(DESTDIR)$(DOCDIR)"
	$(INSTALL) -m 644 LICENSE README.md "$(DESTDIR)$(DOCDIR)"

# Not part of make install, because not every system runs systemd.
#
# The unit in packaging/ starts /usr/bin/mezzo, so point it at BINDIR. It reads
# its settings from /etc/default/mezzo, /etc/sysconfig/mezzo or
# /etc/conf.d/mezzo, whichever exists. See mezzo-config(5).
install-systemd:
	$(INSTALL) -d "$(DESTDIR)$(UNITDIR)"
	sed 's|^ExecStart=.*|ExecStart=$(BINDIR)/$(APP)|' packaging/systemd/$(APP).service > "$(DESTDIR)$(UNITDIR)/$(APP).service"
	chmod 644 "$(DESTDIR)$(UNITDIR)/$(APP).service"

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/$(APP)"
	@for page in $(MAN_PAGES); do \
		rm -f "$(DESTDIR)$(MANDIR)/man$${page##*.}/$$page"; \
	done
	rm -f "$(DESTDIR)$(DOCDIR)/LICENSE" "$(DESTDIR)$(DOCDIR)/README.md"
	@rmdir "$(DESTDIR)$(DOCDIR)" 2>/dev/null || true
	rm -f "$(DESTDIR)$(UNITDIR)/$(APP).service"

snapshot:
	$(GORELEASER) release --snapshot --clean

release:
	$(GORELEASER) release --clean

clean:
	rm -rf $(APP) dist coverage.out man/*.gz
