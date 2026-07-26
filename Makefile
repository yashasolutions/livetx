# LiveTX — build & dev tasks
#
# Run `make` or `make help` to see available targets.

GUI_BIN  := livetx-gui
CLI_BIN  := livetx
GUI_PKG  := ./cmd/livetx-gui
CLI_PKG  := ./cmd/livetx

GO       ?= go
DIST     := dist

.DEFAULT_GOAL := help

## ---- Build ----------------------------------------------------------------

.PHONY: build
build: gui cli ## Build both the GUI and CLI

.PHONY: gui
gui: ## Build the GUI app (needs a C toolchain: Xcode CLT on macOS)
	$(GO) build -o $(GUI_BIN) $(GUI_PKG)

.PHONY: cli
cli: ## Build the CLI app
	$(GO) build -o $(CLI_BIN) $(CLI_PKG)

## ---- Run ------------------------------------------------------------------

.PHONY: run
run: gui ## Build and launch the GUI
	./$(GUI_BIN)

.PHONY: run-cli
run-cli: cli ## Build the CLI and list audio devices
	./$(CLI_BIN) -list-devices

## ---- Cross-compile --------------------------------------------------------
# The CLI is pure Go and cross-compiles anywhere. The GUI uses cgo (Fyne/OpenGL)
# and must be built on the target OS, so it is intentionally not cross-compiled.

.PHONY: cross-cli
cross-cli: ## Cross-compile the CLI for macOS (arm64/amd64) and Linux into dist/
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -o $(DIST)/$(CLI_BIN)-darwin-arm64 $(CLI_PKG)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(DIST)/$(CLI_BIN)-darwin-amd64 $(CLI_PKG)
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(DIST)/$(CLI_BIN)-linux-amd64  $(CLI_PKG)
	@echo "Built CLI binaries in $(DIST)/"

## ---- Dependencies & quality ----------------------------------------------

.PHONY: deps
deps: ## Download Go module dependencies
	$(GO) mod download

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	$(GO) mod tidy

.PHONY: fmt
fmt: ## Format all Go code
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: test
test: ## Run tests
	$(GO) test ./...

.PHONY: check
check: fmt vet test ## Format, vet, and test

## ---- Setup ----------------------------------------------------------------

.PHONY: setup-macos
setup-macos: ## Install all macOS dependencies and build (see README-macos.md)
	./scripts/setup-macos.sh

## ---- Housekeeping ---------------------------------------------------------

.PHONY: clean
clean: ## Remove built binaries and dist/
	rm -f $(GUI_BIN) $(CLI_BIN)
	rm -rf $(DIST)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
