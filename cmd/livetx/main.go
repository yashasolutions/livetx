package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/livetx/livetx/internal/app"
)

func main() {
	// Define flags
	listDevices := flag.Bool("list-devices", false, "List available audio capture devices")
	device := flag.String("device", "", "Device ID or name for audio capture")
	file := flag.String("file", "", "Transcribe an existing mp3/mp4 file to WebVTT (.vtt) instead of live audio")
	outFile := flag.String("out", "", "Output file path (default: transcript-YYYYMMDD-HHMMSS.txt, or <input>.vtt in -file mode)")
	langIn := flag.String("lang-in", "auto", "Input language code (e.g., 'he', 'en') or 'auto'")
	langOut := flag.String("lang-out", "en", "Output/translation language")
	sampleRate := flag.Int("samplerate", 48000, "Audio sample rate in Hz")
	channels := flag.Int("channels", 1, "Number of audio channels")
	cueMaxChars := flag.Int("cue-max-chars", 0, "VTT: max characters per caption cue (0 = default)")
	cueGapMs := flag.Int("cue-gap-ms", 0, "VTT: silence gap in ms that starts a new cue (0 = default)")

	flag.Parse()

	// Load .env file if present
	godotenv.Load()

	// Handle list-devices
	if *listDevices {
		devices, err := app.ListDevices()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing devices: %v\n", err)
			os.Exit(1)
		}

		if len(devices) == 0 {
			fmt.Println("No audio devices found.")
			fmt.Println("On Linux, ensure PulseAudio or PipeWire is running.")
			fmt.Println("Try: pactl list short sources")
			os.Exit(0)
		}

		fmt.Println("Available audio devices:")
		for _, d := range devices {
			fmt.Printf("  %s\n", d.ID)
		}
		os.Exit(0)
	}

	// Get API key
	apiKey := os.Getenv("SONIOX_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "Error: SONIOX_API_KEY not set. Set it in .env or environment.\n")
		os.Exit(1)
	}

	// File mode: transcribe an existing media file to WebVTT and exit.
	if *file != "" {
		runFileMode(apiKey, *file, *outFile, *langIn, *cueMaxChars, *cueGapMs)
		return
	}

	// Generate default output filename if not specified
	outputFile := *outFile
	if outputFile == "" {
		outputFile = fmt.Sprintf("transcript-%s.txt", time.Now().Format("20060102-150405"))
	}

	// Create app config
	cfg := app.Config{
		APIKey:     apiKey,
		DeviceID:   *device,
		OutputFile: outputFile,
		LangIn:     *langIn,
		LangOut:    *langOut,
		SampleRate: *sampleRate,
		Channels:   *channels,
	}

	// Create app
	application, err := app.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing app: %v\n", err)
		os.Exit(1)
	}
	defer application.Close()

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	// Print startup info
	fmt.Printf("Starting transcription...\n")
	fmt.Printf("Output file: %s\n", outputFile)
	if *device != "" {
		fmt.Printf("Device: %s\n", *device)
	}
	fmt.Printf("Sample rate: %d Hz, Channels: %d\n", *sampleRate, *channels)
	fmt.Printf("Language: %s -> %s\n", *langIn, *langOut)
	fmt.Println("Press Ctrl+C to stop.")
	fmt.Println()

	// Run the app
	if err := application.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Transcription complete.")
}

// runFileMode transcribes an existing media file to a WebVTT subtitle file.
func runFileMode(apiKey, inputPath, outFile, langIn string, maxChars, gapMs int) {
	outputFile := outFile
	if outputFile == "" {
		base := filepath.Base(inputPath)
		outputFile = strings.TrimSuffix(base, filepath.Ext(base)) + ".vtt"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	fmt.Printf("Generating VTT: %s -> %s\n", inputPath, outputFile)
	fmt.Printf("Language: %s\n", langIn)

	cfg := app.FileConfig{
		APIKey:     apiKey,
		InputPath:  inputPath,
		OutputFile: outputFile,
		LangIn:     langIn,
		MaxChars:   maxChars,
		GapMs:      int64(gapMs),
	}

	if err := app.RunFileVTT(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}
