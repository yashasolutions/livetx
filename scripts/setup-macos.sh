#!/usr/bin/env bash
#
# setup-macos.sh — bootstrap a fresh (non-developer) Mac to build and run LiveTX.
#
# Installs, if missing: Xcode Command Line Tools, Homebrew, Go, ffmpeg, and
# BlackHole (for capturing system audio — "what you hear"). Then builds the
# GUI and CLI binaries.
#
# Usage:
#   ./scripts/setup-macos.sh            # full setup + build
#   ./scripts/setup-macos.sh --no-blackhole   # skip the system-audio loopback device
#   ./scripts/setup-macos.sh --no-build       # install deps only, don't build
#
# Safe to re-run: every step checks whether the tool is already present.

set -euo pipefail

# ---- pretty output ---------------------------------------------------------
BOLD=$'\033[1m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; RED=$'\033[31m'; RESET=$'\033[0m'
info()  { printf "%s==>%s %s\n" "$BOLD$GREEN" "$RESET" "$*"; }
warn()  { printf "%s!!%s %s\n"  "$BOLD$YELLOW" "$RESET" "$*"; }
err()   { printf "%sxx%s %s\n"  "$BOLD$RED" "$RESET" "$*" >&2; }
step()  { printf "\n%s%s%s\n" "$BOLD" "$*" "$RESET"; }

# ---- args ------------------------------------------------------------------
INSTALL_BLACKHOLE=1
DO_BUILD=1
for arg in "$@"; do
	case "$arg" in
		--no-blackhole) INSTALL_BLACKHOLE=0 ;;
		--no-build)     DO_BUILD=0 ;;
		-h|--help)
			grep '^#' "$0" | sed 's/^# \{0,1\}//'
			exit 0 ;;
		*) err "unknown option: $arg"; exit 1 ;;
	esac
done

# ---- sanity ----------------------------------------------------------------
if [[ "$(uname -s)" != "Darwin" ]]; then
	err "This script is for macOS only. Detected: $(uname -s)"
	exit 1
fi

# Resolve the repo root (script lives in <repo>/scripts).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

ARCH="$(uname -m)"
info "macOS $(sw_vers -productVersion) on $ARCH"

# ---- 1. Xcode Command Line Tools ------------------------------------------
# Required for git and for the C toolchain that Fyne (the GUI) needs to build.
step "1/6  Xcode Command Line Tools"
if xcode-select -p >/dev/null 2>&1; then
	info "Already installed."
else
	warn "Installing Command Line Tools — a system dialog will open."
	xcode-select --install || true
	echo "Waiting for the Command Line Tools installation to finish..."
	echo "Complete the dialog, then this script will continue automatically."
	until xcode-select -p >/dev/null 2>&1; do
		sleep 5
	done
	info "Command Line Tools installed."
fi

# ---- 2. Homebrew -----------------------------------------------------------
step "2/6  Homebrew"
if ! command -v brew >/dev/null 2>&1; then
	warn "Installing Homebrew (you may be prompted for your password)."
	/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

# Make brew available in this shell (paths differ on Apple Silicon vs Intel).
if [[ -x /opt/homebrew/bin/brew ]]; then
	eval "$(/opt/homebrew/bin/brew shellenv)"
elif [[ -x /usr/local/bin/brew ]]; then
	eval "$(/usr/local/bin/brew shellenv)"
fi

if ! command -v brew >/dev/null 2>&1; then
	err "Homebrew install did not complete. Open a new terminal and re-run this script."
	exit 1
fi
info "Homebrew ready: $(brew --version | head -1)"

# ---- 3. Go -----------------------------------------------------------------
step "3/6  Go"
if command -v go >/dev/null 2>&1; then
	info "Already installed: $(go version)"
else
	info "Installing Go..."
	brew install go
	info "$(go version)"
fi

# ---- 4. ffmpeg (audio capture) --------------------------------------------
step "4/6  ffmpeg (audio capture backend)"
if command -v ffmpeg >/dev/null 2>&1; then
	info "Already installed: $(ffmpeg -version | head -1)"
else
	info "Installing ffmpeg..."
	brew install ffmpeg
fi

# ---- 5. BlackHole (system-audio loopback) ---------------------------------
step "5/6  BlackHole (capture system audio — \"what you hear\")"
if [[ "$INSTALL_BLACKHOLE" -eq 0 ]]; then
	warn "Skipped (--no-blackhole). You'll only be able to transcribe the microphone."
elif brew list --cask blackhole-2ch >/dev/null 2>&1; then
	info "Already installed."
else
	warn "Installing BlackHole (a cask install may prompt for your password)."
	brew install --cask blackhole-2ch
	cat <<-'EOF'

	  BlackHole is a virtual audio device. To transcribe system audio:
	    1. Open "Audio MIDI Setup" (in /Applications/Utilities).
	    2. Create a "Multi-Output Device" containing both your real
	       speakers/headphones AND "BlackHole 2ch" so you can still hear audio.
	    3. Set that Multi-Output Device as the macOS output.
	    4. In LiveTX, pick "BlackHole 2ch" as the capture device.
	  See README-macos.md for the full walkthrough.

	EOF
fi

# ---- 6. Build --------------------------------------------------------------
step "6/6  Build LiveTX"
if [[ "$DO_BUILD" -eq 0 ]]; then
	warn "Skipped (--no-build)."
else
	info "Downloading Go modules..."
	go mod download
	info "Building GUI (livetx-gui)..."
	go build -o livetx-gui ./cmd/livetx-gui
	info "Building CLI (livetx)..."
	go build -o livetx ./cmd/livetx
	info "Built ./livetx-gui and ./livetx"
fi

# ---- API key reminder ------------------------------------------------------
step "Configuration"
if [[ -f .env ]] && grep -q "SONIOX_API_KEY" .env 2>/dev/null; then
	info ".env with SONIOX_API_KEY found."
else
	warn "No Soniox API key configured yet. Create a .env file in this folder:"
	echo "    echo 'SONIOX_API_KEY=your_api_key_here' > .env"
fi

step "Done"
cat <<-EOF
  Run the app:
    ${BOLD}./livetx-gui${RESET}                          # graphical interface
    ${BOLD}./livetx -list-devices${RESET}                # list audio devices (CLI)
    ${BOLD}./livetx -device "BlackHole 2ch"${RESET}      # transcribe from the CLI

  First run: macOS will ask for Microphone permission — click Allow.
EOF
