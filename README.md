# livetx

A CLI tool that captures system audio output (loopback/monitor), streams it to Soniox for real-time transcription and translation to English, prints partial transcripts to the console, and appends final transcripts to a text file with ISO-8601 timestamps.

## Features

- Real-time speech-to-text transcription via Soniox
- One-way translation to English
- System audio capture (loopback/monitor source)
- Partial transcripts shown in console
- Final transcripts saved to file with timestamps
- Graceful shutdown on Ctrl+C

## Requirements

- Go 1.21+
- Linux: PipeWire (pw-record) or PulseAudio (parec)
- Soniox API key

## Installation

