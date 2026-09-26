package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
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
	file := flag.String("file", "", "Transcribe an existing mp3/mp4 file (or every media file in a folder) to WebVTT (.vtt) instead of live audio")
	outFile := flag.String("out", "", "Output file path (default: transcript-YYYYMMDD-HHMMSS.txt, or <input>.vtt in -file mode; in folder mode, an output directory)")
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

	// File mode: transcribe an existing media file (or every media file in a
	// folder) to WebVTT and exit.
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

// mediaExts is the set of file extensions treated as transcribable media when
// scanning a folder. Anything else (e.g. .vtt, .txt) is skipped.
var mediaExts = map[string]bool{
	".mp3": true, ".mp4": true, ".m4a": true, ".wav": true,
	".flac": true, ".aac": true, ".ogg": true, ".oga": true,
	".opus": true, ".mkv": true, ".mov": true, ".webm": true,
	".wma": true, ".aiff": true, ".aif": true, ".mpeg": true,
	".mpg": true, ".avi": true, ".ts": true, ".3gp": true, ".flv": true,
}

// runFileMode transcribes a single media file, or every media file in a folder,
// to WebVTT subtitle files. When inputPath is a directory each file is processed
// one after another.
func runFileMode(apiKey, inputPath, outFile, langIn string, maxChars, gapMs int) {
	info, err := os.Stat(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if info.IsDir() {
		runFolderMode(ctx, apiKey, inputPath, outFile, langIn, maxChars, gapMs)
		return
	}

	outputFile := outFile
	if outputFile == "" {
		outputFile = vttPathFor(inputPath, "")
	}

	if err := transcribeFile(ctx, apiKey, inputPath, outputFile, langIn, maxChars, gapMs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done.")
}

// runFolderMode transcribes every media file in dir, one after another. The
// output .vtt for each file is written next to the source, or into outDir when
// -out names a directory. Failures are logged and the batch continues; the
// process exits non-zero if any file failed.
func runFolderMode(ctx context.Context, apiKey, dir, outDir, langIn string, maxChars, gapMs int) {
	files, err := collectMediaFiles(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Printf("No media files found in %s\n", dir)
		return
	}

	fmt.Printf("Found %d media file(s) in %s\n", len(files), dir)

	var done, skipped, failed int
	for i, inputPath := range files {
		if ctx.Err() != nil {
			break
		}

		outputFile := vttPathFor(inputPath, outDir)
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(files), filepath.Base(inputPath))

		// Skip files that already have a transcript so the batch is resumable
		// and re-runs don't repeat paid API calls.
		if _, err := os.Stat(outputFile); err == nil {
			fmt.Printf("  Skipping, %s already exists\n", outputFile)
			skipped++
			continue
		}

		if err := transcribeFile(ctx, apiKey, inputPath, outputFile, langIn, maxChars, gapMs); err != nil {
			if ctx.Err() != nil {
				break
			}
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			failed++
			continue
		}
		done++
	}

	fmt.Printf("\nDone. %d transcribed, %d skipped, %d failed\n", done, skipped, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// transcribeFile generates a single WebVTT file from one media file.
func transcribeFile(ctx context.Context, apiKey, inputPath, outputFile, langIn string, maxChars, gapMs int) error {
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
	return app.RunFileVTT(ctx, cfg)
}

// collectMediaFiles returns the media files directly inside dir, sorted by name.
// Subdirectories are not descended into.
func collectMediaFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if mediaExts[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// vttPathFor returns the .vtt output path for an input media file. When outDir
// is empty the .vtt is placed next to the source; otherwise it goes into outDir.
func vttPathFor(inputPath, outDir string) string {
	base := filepath.Base(inputPath)
	name := strings.TrimSuffix(base, filepath.Ext(base)) + ".vtt"
	if outDir != "" {
		return filepath.Join(outDir, name)
	}
	return filepath.Join(filepath.Dir(inputPath), name)
}
