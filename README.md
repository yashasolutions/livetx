# LiveTX - Live Transcription

A real-time audio transcription application with GUI and CLI interfaces, powered by Soniox speech-to-text API.

## Prerequisites

- Go 1.19 or later
- Audio capture dependencies (platform-specific):
  - **Linux**: PulseAudio or PipeWire with `pactl` and/or `pw-record`
  - **macOS**: `ffmpeg` (audio capture) + optional BlackHole for system audio — see [README-macos.md](README-macos.md) for a one-command setup
  - **Windows**: WASAPI (built-in)

## Installation

1. Clone the repository:
```bash
git clone <repository-url>
cd livetx
```

2. Install Go dependencies:
```bash
go mod download
```

3. Set up your Soniox API key:
   - Create a `.env` file in the project root
   - Add your API key: `SONIOX_API_KEY=your_api_key_here`
   - Or set it as an environment variable

## Building

### GUI Application
```bash
go build -o livetx-gui ./cmd/livetx-gui
```

### CLI Application
```bash
go build -o livetx ./cmd/livetx
```

## Usage

### GUI
```bash
./livetx-gui
```

### CLI
```bash
# List available audio devices
./livetx -list-devices

# Start transcription with specific device
./livetx -device "device_name" -lang-in auto -lang-out en
```

## Features

- Real-time audio transcription
- Language detection and translation
- Multiple audio device support
- File output with timestamps
- Cross-platform support
