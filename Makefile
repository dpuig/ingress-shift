# Makefile for Ingress Shift (analyzer, harness, orchestrator)

VERSION ?= $(shell git describe --tags --always --dirty="-dev" 2>/dev/null || echo dev)
BUILD_DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

LDFLAGS = -ldflags "-X main.Version=$(VERSION)"

# Build targets — analyzer (open source)
build: build-analyzer

build-analyzer:
	go build $(LDFLAGS) -o ingress-shift-analyzer ./src/analyzer

build-analyzer-linux:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o ingress-shift-analyzer-linux-amd64 ./src/analyzer

build-analyzer-macos:
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o ingress-shift-analyzer-darwin-amd64 ./src/analyzer

build-analyzer-windows:
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o ingress-shift-analyzer-windows-amd64.exe ./src/analyzer

# Build targets — harness (commercial)
build-harness:
	go build $(LDFLAGS) -o ingress-shift-harness ./src/harness

# Build targets — orchestrator (commercial)
build-orchestrator:
	go build $(LDFLAGS) -o ingress-shift-orchestrator ./src/orchestrator

build-all: build-analyzer build-harness build-orchestrator

clean:
	rm -f ingress-shift-analyzer ingress-shift-analyzer-linux-amd64 ingress-shift-analyzer-darwin-amd64 ingress-shift-analyzer-windows-amd64.exe \
		ingress-shift-harness ingress-shift-orchestrator

install:
	go install $(LDFLAGS) ./src/analyzer

test:
	go test ./...

lint:
	golangci-lint run ./...

.PHONY: build build-analyzer build-analyzer-linux build-analyzer-macos build-analyzer-windows \
	build-harness build-orchestrator build-all clean install test lint
