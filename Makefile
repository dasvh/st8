GOLANGCI_LINT_VERSION ?= v2.9.0
GOVULNCHECK_VERSION ?= v1.1.4
GOLANGCI_LINT_CMD := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
GOVULNCHECK_CMD := go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	go fmt ./...
	go mod tidy -v

## fmt: run formatters
.PHONY: fmt
fmt:
	$(GOLANGCI_LINT_CMD) fmt

## lint: run linters
.PHONY: lint
lint:
	$(GOLANGCI_LINT_CMD) run

## audit: run quality control checks
.PHONY: audit
audit:
	go mod verify
	$(GOLANGCI_LINT_CMD) run
	$(GOVULNCHECK_CMD) ./...
	go test -race -buildvcs -vet=off ./...

## test: run all tests
.PHONY: test
test:
	go test -v -race -buildvcs ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	go test -v -race -buildvcs -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

## build: compile all packages
.PHONY: build
build:
	go build ./...
