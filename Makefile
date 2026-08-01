# Makefile for Ingress Shift Analyzer

# Variables
BINARY_NAME = ingress-shift-analyzer
VERSION ?= $(shell git describe --tags --always --dirty="-dev" 2>/dev/null || echo dev)
BUILD_DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

PKG = ./src/analyzer

# Build targets
build:
	go build -ldflags "-X main.Version=$(VERSION)" -o $(BINARY_NAME) $(PKG)

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o $(BINARY_NAME)-linux-amd64 $(PKG)

build-macos:
	GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o $(BINARY_NAME)-darwin-amd64 $(PKG)

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o $(BINARY_NAME)-windows-amd64.exe $(PKG)

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME)-linux-amd64 $(BINARY_NAME)-darwin-amd64 $(BINARY_NAME)-windows-amd64.exe

install:
	go install -ldflags "-X main.Version=$(VERSION)" $(PKG)

test:
	go test ./...

.PHONY: build build-linux build-macos build-windows clean install test