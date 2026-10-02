# The checks CI runs, and the two tools: `make try A=disk`, `make propose A=disk`.
GO ?= go
A ?=
P ?=

.PHONY: all build test vet fmt fmt-check lint shellcheck try propose check clean

all: check

## build: the binaries a command action runs (bin/), here bin/go-stats
build:
	$(GO) build -o bin/go-stats ./cmd/go-stats

test:
	$(GO) test -race -count=1 ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

lint:
	golangci-lint run ./...

shellcheck:
	shellcheck actions/*/*.sh

## try: run actions/$(A) as harnsy would; P="-p name=value ..." sets parameters
try: build
	@test -n "$(A)" || { echo "usage: make try A=<action> [P='-p name=value']"; exit 2; }
	$(GO) run ./cmd/try $(P) actions/$(A)

## propose: send actions/$(A) to harnsy (FROM=<agent> PROJECT=<project>)
propose:
	@test -n "$(A)" || { echo "usage: make propose A=<action> FROM=<agent> PROJECT=<project>"; exit 2; }
	$(GO) run ./cmd/propose -from "$(FROM)" -project "$(PROJECT)" actions/$(A)

## check: what CI runs
check: fmt-check vet test build
	@for a in actions/*/; do $(GO) run ./cmd/try $$a >/dev/null || { echo "try failed: $$a"; exit 1; }; done
	@echo "all actions run and report by the contract"

clean:
	rm -rf bin
