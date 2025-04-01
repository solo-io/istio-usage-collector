# Build variables
BINARY_NAME=ambient-migration-estimator-snapshot
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev-build")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GO_VERSION=$(shell go version | awk '{print $$3}')
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT} -X main.goVersion=${GO_VERSION} -X main.binaryName=${BINARY_NAME} -s -w"
GOFLAGS=CGO_ENABLED=0
OUTPUT_DIR=_output

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Source files
SOURCES=$(shell find . -name "*.go" -type f)

.PHONY: all build clean test lint deps tidy help cross-build cross-build-and-pack

all: clean deps tidy test build

# builds the binary for the current platform
build: ensure_output_dir ## Build the binary
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)-$(VERSION) .

# builds the binary for the current platform and packs it using upx
build-and-pack: ensure_output_dir ## Build the binary and pack it using upx
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)-$(VERSION) . && \
	upx $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)-$(VERSION)

clean: ## Clean up build artifacts
	$(GOCLEAN)
	rm -rf $(OUTPUT_DIR)

test: ## Run unit tests
	$(GOTEST) -v ./...

lint: ## Run linters
	golangci-lint run ./...

deps: ## Install dependencies
	$(GOMOD) download

tidy: ## Tidy Go modules
	$(GOMOD) tidy

ensure_output_dir: ## Create output directory if it doesn't exist
	mkdir -p $(OUTPUT_DIR)

# Platforms to build for (GOOS-GOARCH)
PLATFORMS=linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64

# builds the binary for all supported platforms
cross-build: ensure_output_dir deps tidy ## Build for multiple platforms
	$(foreach platform,$(PLATFORMS),\
		$(eval GOOS=$(word 1,$(subst -, ,$(platform)))) \
		$(eval GOARCH=$(word 2,$(subst -, ,$(platform)))) \
		GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)-$(VERSION) .;)

# similar to the cross-build target, but also packs the binaries using upx
# for supported platforms (all but darwin, as of UPX 4.2.4), it reduces binary size by ~2/3.
cross-build-and-pack: ensure_output_dir deps tidy ## Build for multiple platforms and pack using upx
	$(foreach platform,$(PLATFORMS),\
		$(eval GOOS=$(word 1,$(subst -, ,$(platform)))) \
		$(eval GOARCH=$(word 2,$(subst -, ,$(platform)))) \
		GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)-$(VERSION) . && \
		upx $(OUTPUT_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)-$(VERSION);)

help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# Default target
.DEFAULT_GOAL := help 