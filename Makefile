# LiveTX — build & dev tasks
#
# Run `make` or `make help` to see available targets.

GUI_BIN  := livetx-gui
CLI_BIN  := livetx
WEB_BIN  := livetx-web
GUI_PKG  := ./cmd/livetx-gui
CLI_PKG  := ./cmd/livetx
WEB_PKG  := ./cmd/livetx-web

GO       ?= go
DIST     := dist

# Build-time tooling for the web UI (not needed to `go build` — generated .x.go
# and the compiled app.css are committed; only regenerating them needs these).
TOOLS     := .tools
TAILWIND  := $(TOOLS)/tailwindcss
GSX       := github.com/gsxhq/gsx/cmd/gsx@latest
# Map uname to the Tailwind standalone release asset name.
UNAME_S   := $(shell uname -s)
UNAME_M   := $(shell uname -m)
TW_OS     := $(if $(filter Darwin,$(UNAME_S)),macos,linux)
TW_ARCH   := $(if $(filter arm64 aarch64,$(UNAME_M)),arm64,x64)
TW_ASSET  := tailwindcss-$(TW_OS)-$(TW_ARCH)

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

.PHONY: web
web: ## Build the web UI app (pure Go, no cgo — opens in Chrome app mode)
	CGO_ENABLED=0 $(GO) build -o $(WEB_BIN) $(WEB_PKG)

## ---- Web UI assets (gsx + Tailwind) --------------------------------------
# Regenerating assets is a dev step; the outputs (ui/*.x.go, cmd/livetx-web/web/
# app.css) are committed so plain `go build`/`make web` needs none of this.

$(TAILWIND):
	@mkdir -p $(TOOLS)
	@echo "Downloading Tailwind CLI ($(TW_ASSET))..."
	curl -sSL -o $(TAILWIND) https://github.com/tailwindlabs/tailwindcss/releases/latest/download/$(TW_ASSET)
	chmod +x $(TAILWIND)

.PHONY: gen
gen: ## Regenerate gsx components (.gsx -> .x.go)
	$(GO) run $(GSX) generate

.PHONY: web-assets
web-assets: gen $(TAILWIND) ## Regenerate the web UI CSS (gsx generate + Tailwind build)
	$(TAILWIND) -i web/gsxui.css -o cmd/livetx-web/web/app.css --minify
	@echo "Rebuilt cmd/livetx-web/web/app.css"

## ---- Run ------------------------------------------------------------------

.PHONY: run
run: gui ## Build and launch the GUI
	./$(GUI_BIN)

.PHONY: run-cli
run-cli: cli ## Build the CLI and list audio devices
	./$(CLI_BIN) -list-devices

.PHONY: run-web
run-web: web ## Build and launch the web UI (Chrome app-mode window)
	./$(WEB_BIN)

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

.PHONY: cross-web
cross-web: ## Cross-compile the web UI for macOS (arm64/amd64) and Linux into dist/ (no cgo, no toolchain)
	@mkdir -p $(DIST)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 $(GO) build -o $(DIST)/$(WEB_BIN)-darwin-arm64 $(WEB_PKG)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(DIST)/$(WEB_BIN)-darwin-amd64 $(WEB_PKG)
	GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 $(GO) build -o $(DIST)/$(WEB_BIN)-linux-amd64  $(WEB_PKG)
	@echo "Built web UI binaries in $(DIST)/"

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
	rm -f $(GUI_BIN) $(CLI_BIN) $(WEB_BIN)
	rm -rf $(DIST)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
