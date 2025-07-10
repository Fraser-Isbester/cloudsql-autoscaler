.PHONY: build clean test fmt vet install help
.DEFAULT_GOAL := help

BINARY_NAME := cloudsql-autoscaler
BUILD_DIR := .
GO_FILES := $(shell find . -name "*.go" -type f)

## Build the binary
build: $(BUILD_DIR)/$(BINARY_NAME)

$(BUILD_DIR)/$(BINARY_NAME): $(GO_FILES)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

## Install the binary to $GOPATH/bin
install:
	go install ./cmd/$(BINARY_NAME)

## Run tests
test:
	go test -v ./...

## Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

## Format code
fmt:
	go fmt ./...

## Run go vet
vet:
	go vet ./...

## Run all checks (fmt, vet, build, test)
check: fmt vet build test

## Clean build artifacts
clean:
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	rm -f coverage.out coverage.html

## Show help
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'