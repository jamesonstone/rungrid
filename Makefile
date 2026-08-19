.DEFAULT_GOAL := help

BIN_DIR := ./bin
BINARY := $(BIN_DIR)/rungrid
CMD := .
PREFIX ?= /usr/local
GLOBAL_BIN_DIR := $(PREFIX)/bin
GLOBAL_BINARY := $(GLOBAL_BIN_DIR)/rungrid
SUDO ?= sudo
ARGS ?=
GO_FILES := $(shell find . -type f -name '*.go' -not -path './vendor/*')

.PHONY: help build compile link run install clean fmt fmt-check vet test test-race lint vuln license build-cross sanitize check release-snapshot

help:
	@printf '%s\n' 'Rungrid developer workflow'
	@printf '%s\n' ''
	@printf '%s\n' '  make build             compile the rungrid binary into bin/'
	@printf '%s\n' '  make link              one-time symlink of bin/rungrid into PREFIX/bin'
	@printf '%s\n' '  make run ARGS="..."    build and run the repository binary'
	@printf '%s\n' '  make install           install with the active Go toolchain'
	@printf '%s\n' '  make clean             remove generated build and release output'
	@printf '%s\n' '  make check             check format, vet, test, race, licenses, and builds'
	@printf '%s\n' '  make lint              run golangci-lint'
	@printf '%s\n' '  make vuln              run govulncheck'
	@printf '%s\n' '  make license           verify dependency license material'
	@printf '%s\n' '  make release-snapshot  validate a local GoReleaser snapshot'

build: compile

compile:
	mkdir -p $(BIN_DIR)
	go build -o $(BINARY) $(CMD)

link: compile
	@set -eu; target="$(abspath $(BINARY))"; destination="$(GLOBAL_BINARY)"; \
	if [ -L "$$destination" ] && [ "$$(readlink "$$destination")" = "$$target" ]; then \
		printf 'linked %s -> %s\n' "$$destination" "$$target"; \
		exit 0; \
	fi; \
	if [ -e "$$destination" ] && [ ! -L "$$destination" ]; then \
		printf 'refusing to replace non-symlink %s\n' "$$destination" >&2; \
		exit 1; \
	fi; \
	if [ ! -d "$(GLOBAL_BIN_DIR)" ] && \
		! mkdir -p "$(GLOBAL_BIN_DIR)" 2>/dev/null; then \
		$(SUDO) mkdir -p "$(GLOBAL_BIN_DIR)"; \
	fi; \
	if [ -w "$(GLOBAL_BIN_DIR)" ]; then \
		ln -sfn "$$target" "$$destination"; \
	else \
		$(SUDO) ln -sfn "$$target" "$$destination"; \
	fi; \
	[ -L "$$destination" ] && [ "$$(readlink "$$destination")" = "$$target" ] || { \
		printf 'failed to link %s -> %s\n' "$$destination" "$$target" >&2; \
		exit 1; \
	}; \
	printf 'linked %s -> %s\n' "$$destination" "$$target"

run: compile
	$(BINARY) $(ARGS)

install:
	go install $(CMD)

clean:
	rm -rf $(BIN_DIR) dist

fmt:
	gofmt -w $(GO_FILES)

fmt-check:
	@test -z "$$(gofmt -l $(GO_FILES))"

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

license:
	tests/licenses/check.sh

build-cross:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o /tmp/rungrid-darwin-amd64 .
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /tmp/rungrid-darwin-arm64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /tmp/rungrid-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/rungrid-linux-arm64 .

sanitize:
	go test -run 'TestCLISpec(IsNeutral|DefinesV1Contract)' .

check: fmt-check vet test test-race sanitize license compile build-cross

release-snapshot:
	goreleaser check
	@if command -v syft >/dev/null 2>&1; then \
		goreleaser release --snapshot --clean --skip=sign; \
	else \
		goreleaser release --snapshot --clean --skip=sign,sbom; \
	fi
