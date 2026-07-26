# LiveTX on macOS

Real-time audio transcription on macOS. This guide assumes a **plain Mac with
nothing developer-related installed** — the setup script installs everything for
you.

---

## Quick start (recommended)

Open **Terminal** (press `⌘ Space`, type "Terminal", hit Return), then run:

```bash
cd path/to/livetx        # the folder containing this file
./scripts/setup-macos.sh
```

The script installs any missing dependencies, builds the app, and prints how to
run it. It's safe to re-run — it skips anything already installed.

Then add your Soniox API key and launch:

```bash
echo 'SONIOX_API_KEY=your_api_key_here' > .env
./livetx-gui
```

On the first run macOS asks for **Microphone** permission — click **Allow**.

---

## What gets installed

The Mac likely has none of these. The script installs whatever is missing:

| Dependency | Why it's needed |
|------------|-----------------|
| **Xcode Command Line Tools** | Provides `git` and the C compiler the GUI (Fyne) builds against. |
| **Homebrew** | The package manager used to install everything below. |
| **Go** | Compiles LiveTX (it's a Go program). |
| **ffmpeg** | The audio-capture backend on macOS. **Required.** |
| **BlackHole** | Virtual audio device for capturing **system audio** ("what you hear"). Optional — only needed if you're not transcribing the microphone. |

Skip BlackHole with `./scripts/setup-macos.sh --no-blackhole`.

---

## Capturing microphone vs. system audio

- **Microphone** — works immediately. Just pick your mic in the app.
- **System audio ("what you hear")** — macOS has **no built-in way** to capture
  its own output, so you route it through the free **BlackHole** virtual device:

  1. Open **Audio MIDI Setup** (in `/Applications/Utilities`, or search via
     Spotlight).
  2. Click **+** (bottom-left) → **Create Multi-Output Device**.
  3. Tick **both** your real output (e.g. "MacBook Pro Speakers" or your
     headphones) **and** "BlackHole 2ch". This lets you *hear* the audio while
     it's also sent to BlackHole.
  4. Click the macOS volume/sound control → set output to that **Multi-Output
     Device**.
  5. In LiveTX, choose **🔊 BlackHole 2ch** as the capture device.

  Now anything playing on the Mac (a call, a video, a stream) is transcribed,
  and you still hear it normally.

---

## Running it

```bash
./livetx-gui                          # graphical interface
./livetx -list-devices                # list audio devices (CLI)
./livetx -device "BlackHole 2ch" -lang-in auto -lang-out en   # CLI transcription
```

Device names in `-list-devices` are tagged:
`🎙 ... (microphone)` and `🔊 ... (output loopback — what you hear)`.

---

## Manual install (if you prefer not to run the script)

```bash
# 1. Command Line Tools
xcode-select --install

# 2. Homebrew — see https://brew.sh
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# 3. Dependencies
brew install go ffmpeg
brew install --cask blackhole-2ch      # optional, for system audio

# 4. Build
go mod download
go build -o livetx-gui ./cmd/livetx-gui
go build -o livetx ./cmd/livetx
```

---

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| **"ffmpeg not found"** | `brew install ffmpeg`, or re-run `./scripts/setup-macos.sh`. |
| **No audio / empty transcript from the mic** | System Settings → Privacy & Security → **Microphone** → enable Terminal (or LiveTX). Restart the app. |
| **No system audio captured** | Confirm the macOS output is set to the **Multi-Output Device**, and BlackHole is selected in LiveTX. |
| **`brew: command not found` after install** | Close and reopen Terminal, then re-run the script. |
| **GUI won't build** | Ensure Xcode Command Line Tools are installed (`xcode-select -p`); the Fyne GUI needs a C compiler. |
| **"cannot be opened because the developer cannot be verified"** | The BlackHole installer is signed; if macOS blocks it, allow it in System Settings → Privacy & Security. |

---

## Notes for the person building this

- The macOS audio backend lives in `internal/audio/audio_darwin.go` and shells
  out to `ffmpeg`'s `avfoundation` input, mirroring the Linux capturer.
- The **CLI** (`./cmd/livetx`) cross-compiles from any OS with
  `GOOS=darwin go build ./cmd/livetx`. The **GUI** uses cgo (Fyne/OpenGL) and
  must be built **on a Mac** (or with a macOS cross-toolchain).
