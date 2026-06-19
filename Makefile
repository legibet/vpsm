.PHONY: help fmt lint vet test test-no-cache build build-bin run smoke check release

BINARY ?= vpsm
GO ?= go
GOLANGCI_LINT ?= golangci-lint
PKGS := ./...

help:
	@printf '%s\n' \
		'Quality:' \
		'  make fmt           Format Go files using golangci-lint formatters' \
		'  make lint          Run golangci-lint' \
		'  make vet           Run go vet' \
		'  make check         Run lint, vet, test, and build' \
		'' \
		'Tests:' \
		'  make test          Run go test ./...' \
		'  make test-no-cache Run go test ./... -count=1' \
		'  make smoke         Run help smoke test' \
		'' \
		'Build and run:' \
		'  make build         Run go build ./...' \
		'  make build-bin     Build ./$(BINARY)' \
		'  make run           Run the app' \
		'  make release VERSION=v0.1.0  Check, tag, and push a release'

fmt:
	$(GOLANGCI_LINT) fmt

lint:
	$(GOLANGCI_LINT) run

vet:
	$(GO) vet $(PKGS)

test:
	$(GO) test $(PKGS)

test-no-cache:
	$(GO) test $(PKGS) -count=1

build:
	$(GO) build $(PKGS)

build-bin:
	$(GO) build -o $(BINARY) .

run:
	$(GO) run .

smoke:
	$(GO) run . help

check: lint vet test build

release:
	@test -n "$(VERSION)" || (printf '%s\n' 'VERSION is required, for example: make release VERSION=v0.1.0' >&2; exit 2)
	scripts/release.sh $(VERSION)
