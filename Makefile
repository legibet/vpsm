.PHONY: help fmt lint vet test test-no-cache build build-bin run smoke check

BINARY := vpsm

help:
	@printf '%s\n' \
		'make fmt           Run gofmt on all Go files' \
		'make lint          Run golangci-lint' \
		'make vet           Run go vet' \
		'make test          Run go test ./...' \
		'make test-no-cache Run go test ./... -count=1' \
		'make build         Run go build ./...' \
		'make build-bin     Build ./$(BINARY)' \
		'make run           Run the app' \
		'make smoke         Run help smoke test' \
		'make check         Run lint, vet, test, and build'

fmt:
	@gofmt -w $$(find . -name '*.go' -type f)

lint:
	golangci-lint run

vet:
	go vet ./...

test:
	go test ./...

test-no-cache:
	go test ./... -count=1

build:
	go build ./...

build-bin:
	go build -o $(BINARY) .

run:
	go run .

smoke:
	go run . help

check: lint vet test build
