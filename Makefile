# Build variables
BINARY_NAME=istio-usage-collector
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev-build")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X github.com/solo-io/istio-usage-collector/cmd/version.version=${VERSION} -X github.com/solo-io/istio-usage-collector/cmd/version.gitCommit=${GIT_COMMIT} -s -w"
GOFLAGS=CGO_ENABLED=0
OUTPUT_DIR=_output
VERSION_DIR=$(OUTPUT_DIR)/$(VERSION)

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod

# Define extension based on OS (.exe for Windows)
EXT=
ifeq ($(GOOS),windows)
	EXT=.exe
endif

.PHONY: all build clean deps tidy help cross-build cross-build-and-pack install-test-tools run-tests add-test-dependencies

# builds the binary for the current platform
build: ensure_output_dir ## Build the binary
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(VERSION_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) .
	(cd $(VERSION_DIR) && shasum -a 256 $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) > $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT).sha256)

# builds the binary for the current platform and packs it using upx
build-and-pack: ensure_output_dir ## Build the binary and pack it using upx
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(VERSION_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) . && \
	upx $(VERSION_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) && \
	(cd $(VERSION_DIR) && shasum -a 256 $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) > $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT).sha256)

clean: ## Clean up build artifacts
	$(GOCLEAN)
	rm -rf $(OUTPUT_DIR)

deps: ## Install dependencies
	$(GOMOD) download

tidy: ## Tidy Go modules
	$(GOMOD) tidy

ensure_output_dir: ## Create output directory if it doesn't exist
	mkdir -p $(VERSION_DIR)

# Platforms to build for (GOOS-GOARCH)
PLATFORMS=linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64

# builds the binary for all supported platforms
cross-build: ensure_output_dir deps tidy ## Build for multiple platforms
	$(foreach platform,$(PLATFORMS),\
		$(eval GOOS=$(word 1,$(subst -, ,$(platform)))) \
		$(eval GOARCH=$(word 2,$(subst -, ,$(platform)))) \
		$(eval EXT=$(if $(filter windows,$(GOOS)),.exe,)) \
		GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(VERSION_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) . && \
		(cd $(VERSION_DIR) && shasum -a 256 $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) > $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT).sha256);)

# similar to the cross-build target, but also packs the binaries using upx
# for supported platforms (all but darwin, as of UPX 4.2.4), it reduces binary size by ~2/3.
cross-build-and-pack: ensure_output_dir deps tidy ## Build for multiple platforms and pack using upx
	$(foreach platform,$(PLATFORMS),\
		$(eval GOOS=$(word 1,$(subst -, ,$(platform)))) \
		$(eval GOARCH=$(word 2,$(subst -, ,$(platform)))) \
		$(eval EXT=$(if $(filter windows,$(GOOS)),.exe,)) \
		GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOFLAGS) $(GOBUILD) $(LDFLAGS) -o $(VERSION_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) . && \
		upx $(VERSION_DIR)/$(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) && \
		(cd $(VERSION_DIR) && shasum -a 256 $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT) > $(BINARY_NAME)-$(GOOS)-$(GOARCH)$(EXT).sha256);)

help: ## Show this help
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

install-test-tools:
	@go install gotest.tools/gotestsum@v1.12.0

run-unit-tests:
	@gotestsum --junitfile junit-unit-test.xml -- -tags=unit ./...

run-e2e-tests:
	@gotestsum --junitfile junit-e2e-test.xml -- -tags=e2e ./...

add-test-dependencies:
	@helm repo add metrics-server "https://kubernetes-sigs.github.io/metrics-server/"
	@helm repo add istio "https://istio-release.storage.googleapis.com/charts"

# Default target
.DEFAULT_GOAL := help 